package buildworker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/lani772/all-in-one/engine/internal/archive"
	"github.com/lani772/all-in-one/engine/internal/inspector"
	"github.com/lani772/all-in-one/engine/internal/objectstore"
	"github.com/lani772/all-in-one/engine/internal/queue"
)

type Config struct {
	WorkerID string
	MaxArchiveBytes int64
	MaxArchiveEntries int
	MaxFileBytes int64
	MaxExpandedBytes int64
	MaxCompressionRatio float64
}

type Worker struct {
	db *sql.DB
	queue *queue.Queue
	objects *objectstore.MinIOStore
	cfg Config
}

func New(db *sql.DB, q *queue.Queue, objects *objectstore.MinIOStore, cfg Config) (*Worker, error) {
	if db == nil || q == nil || objects == nil { return nil, errors.New("worker requires PostgreSQL, Redis and object storage") }
	if cfg.WorkerID == "" { return nil, errors.New("worker ID is required") }
	if cfg.MaxArchiveBytes <= 0 { cfg.MaxArchiveBytes = 20 << 20 }
	return &Worker{db:db, queue:q, objects:objects, cfg:cfg}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	if err := w.queue.EnsureGroup(ctx); err != nil { return err }
	go w.heartbeat(ctx)
	log.Printf("inspection worker %s started; arbitrary project code execution is disabled", w.cfg.WorkerID)
	consumer := w.cfg.WorkerID
	reclaimTicker := time.NewTicker(30*time.Second)
	defer reclaimTicker.Stop()
	for {
		if err := w.dispatchOutbox(ctx); err != nil && ctx.Err() == nil { log.Printf("outbox dispatch: %v",err) }
		jobs, err := w.queue.Read(ctx, consumer, 2*time.Second, 10)
		if err != nil {
			if ctx.Err() != nil { return nil }
			log.Printf("read build stream: %v",err)
			continue
		}
		for _, job := range jobs {
			if err := w.process(ctx, job.ID); err != nil {
				if ctx.Err() != nil { return nil }
				log.Printf("job %s will remain pending for retry: %v",job.ID,err)
				continue
			}
			if err := w.queue.Ack(ctx,job.StreamID); err != nil { log.Printf("ack job %s: %v",job.ID,err) }
		}
		select {
		case <-reclaimTicker.C:
			stale, claimErr := w.queue.ClaimStale(ctx,consumer,4*time.Minute,10)
			if claimErr != nil { log.Printf("claim stale jobs: %v",claimErr); continue }
			for _, job := range stale {
				if err := w.process(ctx,job.ID); err != nil { log.Printf("reclaimed job %s failed: %v",job.ID,err); continue }
				if err := w.queue.Ack(ctx,job.StreamID); err != nil { log.Printf("ack reclaimed job %s: %v",job.ID,err) }
			}
		default:
		}
	}
}

func (w *Worker) dispatchOutbox(ctx context.Context) error {
	tx, err := w.db.BeginTx(ctx,nil); if err != nil { return err }
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx,`SELECT id, job_id FROM queue_outbox WHERE dispatched_at IS NULL ORDER BY id LIMIT 50 FOR UPDATE SKIP LOCKED`)
	if err != nil { return err }
	type item struct { id int64; jobID string }
	var items []item
	for rows.Next() { var v item; if err := rows.Scan(&v.id,&v.jobID); err != nil { rows.Close(); return err }; items=append(items,v) }
	if err := rows.Err(); err != nil { rows.Close(); return err }
	if err := rows.Close(); err != nil { return err }
	for _, item := range items {
		if err := w.queue.Enqueue(ctx,item.jobID); err != nil { return err }
		if _, err := tx.ExecContext(ctx,`UPDATE queue_outbox SET dispatched_at=now() WHERE id=$1`,item.id); err != nil { return err }
	}
	return tx.Commit()
}

func (w *Worker) process(parent context.Context, jobID string) error {
	ctx,cancel := context.WithTimeout(parent,3*time.Minute); defer cancel()
	var projectID, sourceKey string
	err := w.db.QueryRowContext(ctx,`UPDATE build_jobs SET status='inspecting', started_at=COALESCE(started_at,now()), updated_at=now() WHERE id=$1 AND (status='queued' OR (status='inspecting' AND updated_at < now()-interval '4 minutes')) RETURNING project_id, source_object_key`,jobID).Scan(&projectID,&sourceKey)
	if errors.Is(err,sql.ErrNoRows) { return nil }
	if err != nil { return err }

	temp, err := os.MkdirTemp("","allinone-inspect-*")
	if err != nil { return w.fail(ctx,jobID,err) }
	defer os.RemoveAll(temp)
	zipPath := filepath.Join(temp,"source.zip")
	if _,err := w.objects.DownloadSource(ctx,sourceKey,zipPath,w.cfg.MaxArchiveBytes); err != nil { return w.fail(ctx,jobID,err) }
	sourceDir := filepath.Join(temp,"source")
	limits := archive.Limits{MaxArchiveBytes:w.cfg.MaxArchiveBytes,MaxEntries:w.cfg.MaxArchiveEntries,MaxFileBytes:w.cfg.MaxFileBytes,MaxExpandedBytes:w.cfg.MaxExpandedBytes,MaxCompressionRatio:w.cfg.MaxCompressionRatio}
	if err := archive.SafeExtractContext(ctx,zipPath,sourceDir,limits); err != nil { return w.fail(ctx,jobID,err) }
	result, err := inspector.Inspect(sourceDir)
	if err != nil { return w.fail(ctx,jobID,err) }
	encoded, err := json.Marshal(result); if err != nil { return w.fail(ctx,jobID,err) }
	_, err = w.db.ExecContext(ctx,`UPDATE build_jobs SET status='inspected', inspection_result=$2::jsonb, error_message=NULL, updated_at=now(), finished_at=now() WHERE id=$1 AND status='inspecting'`,jobID,string(encoded))
	return err
}

func (w *Worker) fail(ctx context.Context, jobID string, cause error) error {
	if ctx.Err() != nil { return ctx.Err() }
	message := cause.Error(); if len(message)>1000 { message=message[:1000] }
	_, err := w.db.ExecContext(ctx,`UPDATE build_jobs SET status='failed', error_message=$2, updated_at=now(), finished_at=now() WHERE id=$1 AND status='inspecting'`,jobID,message)
	if err != nil { return fmt.Errorf("%v (persist failure: %w)",cause,err) }
	return nil
}

func (w *Worker) heartbeat(ctx context.Context) {
	ticker:=time.NewTicker(10*time.Second); defer ticker.Stop()
	for {
		_,err:=w.db.ExecContext(ctx,`INSERT INTO engine_workers(worker_id,mode,last_seen_at) VALUES($1,'inspection-only',now()) ON CONFLICT(worker_id) DO UPDATE SET mode=EXCLUDED.mode,last_seen_at=now()`,w.cfg.WorkerID)
		if err!=nil && ctx.Err()==nil { log.Printf("worker heartbeat: %v",err) }
		select { case <-ctx.Done(): return; case <-ticker.C: }
	}
}
