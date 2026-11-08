package worker

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "log"
 "os"
 "path/filepath"
 "strconv"
 "time"

 "github.com/lani772/all-in-one/engine/internal/archive"
 "github.com/lani772/all-in-one/engine/internal/inspector"
 "github.com/lani772/all-in-one/engine/internal/objectstore"
 "github.com/lani772/all-in-one/engine/internal/queue"
)

type Config struct {
 MaxArchiveBytes int64
 MaxExpandedBytes int64
 MaxEntries int
 MaxCompressionRatio float64
 Timeout time.Duration
}
type Worker struct {
 db *sql.DB
 objects *objectstore.MinIOStore
 queue *queue.Queue
 consumer string
 cfg Config
}
func New(db *sql.DB,objects *objectstore.MinIOStore,jobs *queue.Queue,consumer string,cfg Config)(*Worker,error){
 if db==nil||objects==nil||jobs==nil{return nil,errors.New("database, object storage and Redis queue are required")}
 if consumer==""{consumer="inspection-worker"}
 if cfg.MaxArchiveBytes<=0{cfg.MaxArchiveBytes=20<<20}
 if cfg.MaxExpandedBytes<=0{cfg.MaxExpandedBytes=512<<20}
 if cfg.MaxEntries<=0{cfg.MaxEntries=10000}
 if cfg.MaxCompressionRatio<=0{cfg.MaxCompressionRatio=200}
 if cfg.Timeout<=0{cfg.Timeout=5*time.Minute}
 return &Worker{db:db,objects:objects,queue:jobs,consumer:consumer,cfg:cfg},nil
}
func(w *Worker)Run(ctx context.Context)error{
 if err:=w.queue.Ping(ctx);err!=nil{return err}
 if err:=w.queue.EnsureGroup(ctx);err!=nil{return err}
 log.Printf("inspection worker %q started; customer code execution is disabled",w.consumer)
 for{
  if ctx.Err()!=nil{return nil}
  if err:=w.publishOutbox(ctx);err!=nil&&ctx.Err()==nil{log.Printf("outbox dispatch: %v",err)}
  messages,err:=w.queue.ClaimStale(ctx,w.consumer,5*time.Minute)
  if err!=nil&&ctx.Err()==nil{log.Printf("claim stale messages: %v",err)}
  if len(messages)==0{
   messages,err=w.queue.Read(ctx,w.consumer,2*time.Second)
   if err!=nil{if ctx.Err()!=nil{return nil};log.Printf("read inspection queue: %v",err);time.Sleep(time.Second);continue}
  }
  for _,message:=range messages{
   if message.JobID==""{_ = w.queue.Ack(ctx,message.ID);continue}
   if err:=w.process(ctx,message.JobID);err!=nil{log.Printf("inspection job %s deferred: %v",message.JobID,err);continue}
   if err:=w.queue.Ack(ctx,message.ID);err!=nil{log.Printf("ack inspection message %s: %v",message.ID,err)}
  }
 }
}

// PostgreSQL is the source of truth. A crash after XADD but before published_at
// can create duplicate stream entries; database claiming makes them idempotent.
func(w *Worker)publishOutbox(ctx context.Context)error{
 tx,err:=w.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 rows,err:=tx.QueryContext(ctx,`SELECT id,job_id FROM job_outbox WHERE published_at IS NULL ORDER BY id LIMIT 25 FOR UPDATE SKIP LOCKED`)
 if err!=nil{return err}
 type item struct{id int64;jobID string};items:=make([]item,0,25)
 for rows.Next(){var v item;if err:=rows.Scan(&v.id,&v.jobID);err!=nil{rows.Close();return err};items=append(items,v)}
 if err:=rows.Err();err!=nil{rows.Close();return err};if err:=rows.Close();err!=nil{return err}
 for _,v:=range items{
  if err:=w.queue.Publish(ctx,v.jobID);err!=nil{return err}
  if _,err:=tx.ExecContext(ctx,`UPDATE job_outbox SET published_at=now(),attempts=attempts+1 WHERE id=$1`,v.id);err!=nil{return err}
 }
 return tx.Commit()
}

type jobRecord struct{ID string;ProjectID string;UploadID string;SourceObjectKey string;SourceSHA256 string;SourceSize int64}
func(w *Worker)process(parent context.Context,jobID string)error{
 ctx,cancel:=context.WithTimeout(parent,w.cfg.Timeout);defer cancel()
 var job jobRecord
 err:=w.db.QueryRowContext(ctx,`UPDATE inspection_jobs SET status='inspecting',attempts=attempts+1,updated_at=now(),started_at=COALESCE(started_at,now())
 WHERE id=$1 AND (status='queued' OR (status='inspecting' AND updated_at<now()-interval '5 minutes'))
 RETURNING id,project_id,upload_id,source_object_key,source_sha256,source_size_bytes`,jobID).
 Scan(&job.ID,&job.ProjectID,&job.UploadID,&job.SourceObjectKey,&job.SourceSHA256,&job.SourceSize)
 if errors.Is(err,sql.ErrNoRows){return nil}
 if err!=nil{return fmt.Errorf("claim inspection job: %w",err)}
 if _,err:=w.db.ExecContext(ctx,`UPDATE source_uploads SET status='inspecting' WHERE id=$1 AND status IN ('sealed','inspecting')`,job.UploadID);err!=nil{return err}
 tmp,err:=os.MkdirTemp("","hosting-inspection-");if err!=nil{return w.fail(job,err)}
 defer os.RemoveAll(tmp)
 archivePath:=filepath.Join(tmp,"source.zip")
 size,digest,err:=w.objects.DownloadSealedSource(ctx,job.SourceObjectKey,archivePath,w.cfg.MaxArchiveBytes)
 if err!=nil{return w.fail(job,err)}
 if size!=job.SourceSize||digest!=job.SourceSHA256{return w.fail(job,errors.New("sealed source checksum or size does not match PostgreSQL metadata"))}
 sourceDir:=filepath.Join(tmp,"source")
 if err:=archive.Extract(archivePath,sourceDir,archive.Limits{MaxEntries:w.cfg.MaxEntries,MaxFileBytes:100<<20,MaxExpandedBytes:w.cfg.MaxExpandedBytes,MaxCompressionRatio:w.cfg.MaxCompressionRatio});err!=nil{return w.fail(job,err)}
 result,err:=inspector.Inspect(sourceDir);if err!=nil{return w.fail(job,err)}
 encoded,err:=json.Marshal(result);if err!=nil{return w.fail(job,err)}
 logText:=fmt.Sprintf("Source archive verified (%d bytes, SHA-256 %s), extracted safely. Framework=%s; package manager=%s; files=%d. No project code was executed.",size,digest,result.Framework,result.PackageManager,result.FilesSeen)
 persistCtx,persistCancel:=context.WithTimeout(context.Background(),10*time.Second);defer persistCancel()
 tx,err:=w.db.BeginTx(persistCtx,nil);if err!=nil{return err};defer tx.Rollback()
 if _,err:=tx.ExecContext(persistCtx,`UPDATE inspection_jobs SET status='succeeded',result=$2::jsonb,log_text=$3,updated_at=now(),completed_at=now() WHERE id=$1 AND status='inspecting'`,job.ID,string(encoded),logText);err!=nil{return err}
 if _,err:=tx.ExecContext(persistCtx,`UPDATE source_uploads SET status='inspected' WHERE id=$1 AND status='inspecting'`,job.UploadID);err!=nil{return err}
 return tx.Commit()
}
func(w *Worker)fail(job jobRecord,cause error)error{
 message:=cause.Error();if len(message)>4000{message=message[:4000]}
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
 tx,err:=w.db.BeginTx(ctx,nil);if err!=nil{return fmt.Errorf("%v (could not persist failure: %w)",cause,err)};defer tx.Rollback()
 if _,err:=tx.ExecContext(ctx,`UPDATE inspection_jobs SET status='failed',log_text=$2,updated_at=now(),completed_at=now() WHERE id=$1`,job.ID,message);err!=nil{return fmt.Errorf("%v (could not persist failure: %w)",cause,err)}
 if _,err:=tx.ExecContext(ctx,`UPDATE source_uploads SET status='failed' WHERE id=$1 AND status='inspecting'`,job.UploadID);err!=nil{return fmt.Errorf("%v (could not update source status: %w)",cause,err)}
 if err:=tx.Commit();err!=nil{return fmt.Errorf("%v (could not commit failure: %w)",cause,err)}
 return nil
}
func ParseIntEnv(name string,fallback int)int{raw:=os.Getenv(name);if raw==""{return fallback};v,err:=strconv.Atoi(raw);if err!=nil||v<1{return fallback};return v}
func ParseInt64Env(name string,fallback int64)int64{raw:=os.Getenv(name);if raw==""{return fallback};v,err:=strconv.ParseInt(raw,10,64);if err!=nil||v<1{return fallback};return v}
func ParseFloatEnv(name string,fallback float64)float64{raw:=os.Getenv(name);if raw==""{return fallback};v,err:=strconv.ParseFloat(raw,64);if err!=nil||v<=0{return fallback};return v}
