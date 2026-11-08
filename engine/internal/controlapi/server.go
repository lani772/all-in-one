package controlapi

import (
 "context"
 "crypto/rand"
 "database/sql"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "path"
 "regexp"
 "strings"
 "time"

 "github.com/lani772/all-in-one/engine/internal/auth"
)

const (
 defaultMaxSourceBytes int64 = 20 << 20
 maxJSONBytes = 64 << 10
)
var (
 idPattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")
 controlChars = regexp.MustCompile("[\\x00-\\x1f\\x7f]")
)
type SourceStore interface {
 PresignSourceUpload(context.Context,string,time.Duration,int64)(string,map[string]string,error)
 StatSource(context.Context,string)(int64,error)
 Ready(context.Context) error
}
type Config struct { MaxSourceBytes int64; UploadTTL time.Duration }
type Server struct { db *sql.DB; objects SourceStore; cfg Config }

func New(db *sql.DB, objects SourceStore, cfg Config)(*Server,error){
 if db==nil || objects==nil { return nil,errors.New("database and source object store are required") }
 if cfg.MaxSourceBytes<=0 { cfg.MaxSourceBytes=defaultMaxSourceBytes }
 if cfg.UploadTTL<=0 || cfg.UploadTTL>30*time.Minute { cfg.UploadTTL=15*time.Minute }
 return &Server{db:db,objects:objects,cfg:cfg},nil
}
func (s *Server) Health(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet { write(w,405,map[string]string{"error":"method not allowed"});return }
 write(w,200,map[string]string{"status":"ok","service":"all-in-one-hosting-engine"})
}
func (s *Server) Ready(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet { write(w,405,map[string]string{"error":"method not allowed"});return }
 ctx,cancel:=context.WithTimeout(r.Context(),3*time.Second);defer cancel()
 checks:=map[string]string{"buildExecution":"not-connected","worker":"not-connected"}
 if err:=s.db.PingContext(ctx);err!=nil {checks["database"]="error";write(w,503,map[string]any{"status":"not-ready","checks":checks});return}
 checks["database"]="ok"
 if err:=s.objects.Ready(ctx);err!=nil {checks["objectStorage"]="error";write(w,503,map[string]any{"status":"not-ready","checks":checks});return}
 checks["objectStorage"]="ok"
 var workerReady bool
 if err:=s.db.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM engine_workers WHERE mode='inspection-only' AND last_seen_at > now() - interval '45 seconds')").Scan(&workerReady);err==nil && workerReady {
  checks["worker"]="ok"
  checks["buildExecution"]="inspection-only"
 }
 write(w,200,map[string]any{"status":"ready","service":"all-in-one-hosting-engine","checks":checks})
}
func (s *Server) ServeHTTP(w http.ResponseWriter,r *http.Request){
 if r.URL.RawQuery!="" {write(w,400,map[string]string{"error":"query parameters are not supported"});return}
 p:=strings.Split(strings.Trim(r.URL.Path,"/"),"/")
 if len(p)==3 && p[0]=="api" && p[1]=="v1" && p[2]=="projects" && r.Method==http.MethodPost {s.createProject(w,r);return}
 if len(p)==5 && p[0]=="api" && p[1]=="v1" && p[2]=="projects" && idPattern.MatchString(p[3]){
  switch {
  case p[4]=="source-upload" && r.Method==http.MethodPost: s.createSourceUpload(w,r,p[3]);return
  case p[4]=="source-finalize" && r.Method==http.MethodPost: s.finalizeSourceUpload(w,r,p[3]);return
  case p[4]=="builds" && r.Method==http.MethodPost: s.createBuild(w,r,p[3]);return
  case p[4]=="deployments" && r.Method==http.MethodGet: s.listDeployments(w,r,p[3]);return
  }
 }
 if len(p)==4 && p[0]=="api" && p[1]=="v1" && p[2]=="builds" && idPattern.MatchString(p[3]) && r.Method==http.MethodGet {
  s.getBuild(w,r,p[3]);return
 }
 if len(p)==5 && p[0]=="api" && p[1]=="v1" && p[2]=="builds" && idPattern.MatchString(p[3]) && p[4]=="cancel" && r.Method==http.MethodPost {
  s.cancelBuild(w,r,p[3]);return
 }
 write(w,404,map[string]string{"error":"route not found"})
}
func (s *Server) createProject(w http.ResponseWriter,r *http.Request){
 owner:=auth.UserID(r.Context()); if owner=="" {write(w,401,map[string]string{"error":"authenticated owner is missing"});return}
 var in struct{Name string "json:\"name\""}
 if err:=decode(r,&in);err!=nil {write(w,400,map[string]string{"error":err.Error()});return}
 name:=strings.TrimSpace(in.Name); if len([]rune(name))<2 || len([]rune(name))>48 || controlChars.MatchString(name) {write(w,400,map[string]string{"error":"project name must contain 2-48 printable characters"});return}
 id,err:=newID();if err!=nil {write(w,500,map[string]string{"error":"could not generate project identifier"});return}
 base:=slugify(name);ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
 for attempt:=0;attempt<6;attempt++ {
  slug:=base;if attempt>0 {slug=fmt.Sprintf("%s-%s",base,id[:6+attempt])}
  result,err:=s.db.ExecContext(ctx,"INSERT INTO projects (id,owner_id,name,slug) VALUES ($1,$2,$3,$4) ON CONFLICT (slug) DO NOTHING",id,owner,name,slug)
  if err!=nil {write(w,503,map[string]string{"error":"could not create project"});return}
  count,err:=result.RowsAffected();if err!=nil {write(w,503,map[string]string{"error":"could not confirm project creation"});return}
  if count==1 {write(w,201,map[string]any{"projectId":id,"slug":slug,"name":name,"status":"created"});return}
 }
 write(w,409,map[string]string{"error":"could not allocate a unique project URL; retry"})
}
func (s *Server) createSourceUpload(w http.ResponseWriter,r *http.Request,projectID string){
 owner:=auth.UserID(r.Context())
 var in struct{Name string "json:\"name\"";Size int64 "json:\"size\""}
 if err:=decode(r,&in);err!=nil {write(w,400,map[string]string{"error":err.Error()});return}
 if !validArchiveName(in.Name)||in.Size<1||in.Size>s.cfg.MaxSourceBytes {write(w,400,map[string]string{"error":fmt.Sprintf("source must be a .zip file between 1 and %d bytes",s.cfg.MaxSourceBytes)});return}
 ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
 var exists bool
 if err:=s.db.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1 AND owner_id=$2)",projectID,owner).Scan(&exists);err!=nil {write(w,503,map[string]string{"error":"could not verify project ownership"});return}
 if !exists {write(w,404,map[string]string{"error":"project not found"});return}
 uploadID,err:=newID();if err!=nil {write(w,500,map[string]string{"error":"could not generate upload identifier"});return}
 key:="sources/"+projectID+"/"+uploadID+"/source.zip"
 url,fields,err:=s.objects.PresignSourceUpload(ctx,key,s.cfg.UploadTTL,in.Size);if err!=nil {write(w,503,map[string]string{"error":"could not create a size-limited private upload form"});return}
 expires:=time.Now().UTC().Add(s.cfg.UploadTTL)
 _,err=s.db.ExecContext(ctx,"INSERT INTO source_uploads (id,project_id,owner_id,object_key,original_name,size_bytes,status,expires_at) VALUES ($1,$2,$3,$4,$5,$6,'pending',$7)",uploadID,projectID,owner,key,in.Name,in.Size,expires)
 if err!=nil {write(w,503,map[string]string{"error":"could not save source upload metadata"});return}
 write(w,201,map[string]any{"uploadUrl":url,"objectKey":key,"expiresAt":expires.Format(time.RFC3339),"fields":fields})
}
func (s *Server) finalizeSourceUpload(w http.ResponseWriter,r *http.Request,projectID string){
 owner:=auth.UserID(r.Context());var in struct{ObjectKey string "json:\"objectKey\""}
 if err:=decode(r,&in);err!=nil {write(w,400,map[string]string{"error":err.Error()});return}
 if !validSourceKey(in.ObjectKey,projectID) {write(w,400,map[string]string{"error":"invalid source object key"});return}
 ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
 var uploadID string;var expected int64
 err:=s.db.QueryRowContext(ctx,"SELECT id,size_bytes FROM source_uploads WHERE project_id=$1 AND owner_id=$2 AND object_key=$3 AND status='pending' AND expires_at>now()",projectID,owner,in.ObjectKey).Scan(&uploadID,&expected)
 if errors.Is(err,sql.ErrNoRows) {write(w,404,map[string]string{"error":"pending source upload not found or expired"});return}
 if err!=nil {write(w,503,map[string]string{"error":"could not load source upload metadata"});return}
 size,err:=s.objects.StatSource(ctx,in.ObjectKey);if err!=nil {write(w,409,map[string]string{"error":"uploaded source object is not present yet"});return}
 if size!=expected {_,_=s.db.ExecContext(ctx,"UPDATE source_uploads SET status='failed' WHERE id=$1 AND owner_id=$2",uploadID,owner);write(w,409,map[string]string{"error":"uploaded object size does not match the declared source size"});return}
 result,err:=s.db.ExecContext(ctx,"UPDATE source_uploads SET status='uploaded',uploaded_at=now() WHERE id=$1 AND owner_id=$2 AND status='pending'",uploadID,owner)
 if err!=nil {write(w,503,map[string]string{"error":"could not finalize source upload"});return}
 count,err:=result.RowsAffected();if err!=nil||count!=1 {write(w,409,map[string]string{"error":"source upload was already finalized or changed"});return}
 write(w,200,map[string]any{"uploaded":true,"objectKey":in.ObjectKey,"size":size})
}
func (s *Server) createBuild(w http.ResponseWriter,r *http.Request,projectID string){
 owner:=auth.UserID(r.Context())
 ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel()
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil {write(w,503,map[string]string{"error":"could not begin build job transaction"});return}
 defer tx.Rollback()
 var uploadID,sourceKey string
 err=tx.QueryRowContext(ctx,"SELECT id,object_key FROM source_uploads WHERE project_id=$1 AND owner_id=$2 AND status='uploaded' ORDER BY uploaded_at DESC LIMIT 1 FOR UPDATE",projectID,owner).Scan(&uploadID,&sourceKey)
 if errors.Is(err,sql.ErrNoRows) {write(w,409,map[string]string{"error":"upload and finalize a source ZIP before creating a build"});return}
 if err!=nil {write(w,503,map[string]string{"error":"could not locate a finalized source upload"});return}
 jobID,err:=newID();if err!=nil {write(w,500,map[string]string{"error":"could not generate build job identifier"});return}
 _,err=tx.ExecContext(ctx,"INSERT INTO build_jobs (id,project_id,source_upload_id,owner_id,source_object_key,status) VALUES ($1,$2,$3,$4,$5,'queued')",jobID,projectID,uploadID,owner,sourceKey)
 if err!=nil {write(w,503,map[string]string{"error":"could not create build job"});return}
 _,err=tx.ExecContext(ctx,"INSERT INTO queue_outbox (job_id) VALUES ($1)",jobID)
 if err!=nil {write(w,503,map[string]string{"error":"could not record build queue event"});return}
 if err:=tx.Commit();err!=nil {write(w,503,map[string]string{"error":"could not commit build job"});return}
 write(w,202,map[string]any{"jobId":jobID,"status":"queued","execution":"inspection-only","message":"The source ZIP is queued for secure extraction and framework inspection. Project build commands are not executed."})
}

func (s *Server) getBuild(w http.ResponseWriter,r *http.Request,jobID string){
 owner:=auth.UserID(r.Context())
 ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
 var status string
 var inspection []byte
 var message sql.NullString
 var createdAt,updatedAt time.Time
 err:=s.db.QueryRowContext(ctx,"SELECT status,inspection_result,error_message,created_at,updated_at FROM build_jobs WHERE id=$1 AND owner_id=$2",jobID,owner).Scan(&status,&inspection,&message,&createdAt,&updatedAt)
 if errors.Is(err,sql.ErrNoRows) {write(w,404,map[string]string{"error":"build job not found"});return}
 if err!=nil {write(w,503,map[string]string{"error":"could not read build job"});return}
 var result any
 if len(inspection)>0 { result=json.RawMessage(inspection) }
 var errorText any
 if message.Valid { errorText=message.String }
 write(w,200,map[string]any{"jobId":jobID,"status":status,"execution":"inspection-only","inspection":result,"error":errorText,"createdAt":createdAt,"updatedAt":updatedAt})
}

func (s *Server) cancelBuild(w http.ResponseWriter,r *http.Request,jobID string){
 owner:=auth.UserID(r.Context())
 ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
 result,err:=s.db.ExecContext(ctx,"UPDATE build_jobs SET status='cancelled',updated_at=now(),finished_at=now() WHERE id=$1 AND owner_id=$2 AND status='queued'",jobID,owner)
 if err!=nil {write(w,503,map[string]string{"error":"could not cancel build job"});return}
 count,err:=result.RowsAffected();if err!=nil {write(w,503,map[string]string{"error":"could not confirm build cancellation"});return}
 if count==1 {write(w,200,map[string]any{"jobId":jobID,"status":"cancelled"});return}
 var status string
 err=s.db.QueryRowContext(ctx,"SELECT status FROM build_jobs WHERE id=$1 AND owner_id=$2",jobID,owner).Scan(&status)
 if errors.Is(err,sql.ErrNoRows) {write(w,404,map[string]string{"error":"build job not found"});return}
 if err!=nil {write(w,503,map[string]string{"error":"could not read build job state"});return}
 write(w,409,map[string]any{"error":"only queued jobs can be cancelled","status":status})
}
func (s *Server) listDeployments(w http.ResponseWriter,r *http.Request,projectID string){
 owner:=auth.UserID(r.Context());ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
 var current sql.NullString
 err:=s.db.QueryRowContext(ctx,"SELECT current_deployment_id FROM projects WHERE id=$1 AND owner_id=$2",projectID,owner).Scan(&current)
 if errors.Is(err,sql.ErrNoRows) {write(w,404,map[string]string{"error":"project not found"});return}
 if err!=nil {write(w,503,map[string]string{"error":"could not read project deployments"});return}
 rows,err:=s.db.QueryContext(ctx,"SELECT id,build_id,status,manifest_key,total_bytes,file_count,created_at FROM deployments WHERE project_id=$1 AND owner_id=$2 ORDER BY created_at DESC LIMIT 50",projectID,owner)
 if err!=nil {write(w,503,map[string]string{"error":"could not list deployments"});return};defer rows.Close()
 type item struct{ID string "json:\"id\"";BuildID string "json:\"buildId\"";Status string "json:\"status\"";ManifestKey string "json:\"manifestKey\"";TotalBytes int64 "json:\"totalBytes\"";FileCount int "json:\"fileCount\"";CreatedAt time.Time "json:\"createdAt\""}
 items:=make([]item,0)
 for rows.Next(){var v item;if err:=rows.Scan(&v.ID,&v.BuildID,&v.Status,&v.ManifestKey,&v.TotalBytes,&v.FileCount,&v.CreatedAt);err!=nil {write(w,503,map[string]string{"error":"could not read deployment records"});return};items=append(items,v)}
 if err:=rows.Err();err!=nil {write(w,503,map[string]string{"error":"could not finish listing deployments"});return}
 write(w,200,map[string]any{"deployments":items,"currentDeploymentId":current.String})
}
func decode(r *http.Request,target any)error{
 if r.Body==nil{return errors.New("JSON request body is required")}
 data,err:=io.ReadAll(io.LimitReader(r.Body,maxJSONBytes+1));if err!=nil{return errors.New("could not read JSON request body")}
 if len(data)==0||len(data)>maxJSONBytes{return errors.New("JSON request body is empty or too large")}
 d:=json.NewDecoder(strings.NewReader(string(data)));d.DisallowUnknownFields()
 if err:=d.Decode(target);err!=nil{return errors.New("invalid JSON request body")}
 if d.Decode(&struct{}{})!=io.EOF{return errors.New("request body must contain one JSON value")}
 return nil
}
func slugify(v string)string{
 var b strings.Builder;dash:=false
 for _,r:=range strings.ToLower(v){if r>='a'&&r<='z'||r>='0'&&r<='9'{b.WriteRune(r);dash=false;continue};if !dash&&b.Len()>0{b.WriteByte('-');dash=true}}
 slug:=strings.Trim(b.String(),"-");if slug==""{slug="project"};if len(slug)>48{slug=strings.TrimRight(slug[:48],"-")};return slug
}
func validArchiveName(v string)bool{return v!=""&&len(v)<=180&&!controlChars.MatchString(v)&&!strings.ContainsAny(v,"/\\")&&path.Base(v)==v&&strings.EqualFold(path.Ext(v),".zip")}
func validSourceKey(key,projectID string)bool{p:=strings.Split(key,"/");return len(p)==4&&p[0]=="sources"&&p[1]==projectID&&idPattern.MatchString(p[2])&&p[3]=="source.zip"&&!strings.Contains(key,"..")}
func newID()(string,error){var b [16]byte;if _,err:=rand.Read(b[:]);err!=nil{return "",err};return hex.EncodeToString(b[:]),nil}
func write(w http.ResponseWriter,status int,value any){w.Header().Set("Content-Type","application/json; charset=utf-8");w.Header().Set("Cache-Control","no-store");w.WriteHeader(status);_ = json.NewEncoder(w).Encode(value)}
