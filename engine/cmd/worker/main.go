package main

import (
 "context"
 "log"
 "os"
 "os/signal"
 "strconv"
 "strings"
 "syscall"
 "time"

 "github.com/lani772/all-in-one/engine/internal/deployment"
 "github.com/lani772/all-in-one/engine/internal/objectstore"
 "github.com/lani772/all-in-one/engine/internal/queue"
 "github.com/lani772/all-in-one/engine/internal/worker"
)
func main(){
 ctx,cancel:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer cancel()
 db,err:=deployment.OpenPostgres(ctx,strings.TrimSpace(os.Getenv("DATABASE_URL")));if err!=nil{log.Fatal(err)};defer db.Close()
 objects,err:=objectstore.NewMinIOStore(ctx,objectstore.MinIOConfig{
  Endpoint:strings.TrimSpace(os.Getenv("S3_ENDPOINT")),Bucket:strings.TrimSpace(os.Getenv("S3_BUCKET")),
  AccessKey:os.Getenv("S3_ACCESS_KEY"),SecretKey:os.Getenv("S3_SECRET_KEY"),
  Region:envOr("S3_REGION","us-east-1"),UseTLS:strings.EqualFold(os.Getenv("S3_USE_TLS"),"true"),
 });if err!=nil{log.Fatal(err)}
 dbIndex:=0;if raw:=os.Getenv("REDIS_DB");raw!=""{if parsed,e:=strconv.Atoi(raw);e==nil&&parsed>=0{dbIndex=parsed}}
 jobs,err:=queue.New(envOr("REDIS_ADDR","localhost:6379"),os.Getenv("REDIS_PASSWORD"),dbIndex);if err!=nil{log.Fatal(err)};defer jobs.Close()
 if err:=jobs.Ping(ctx);err!=nil{log.Fatal(err)}
 host,_:=os.Hostname();consumer:=host+"-"+strconv.Itoa(os.Getpid())
 instance,err:=worker.New(db,objects,jobs,consumer,worker.Config{
  MaxArchiveBytes:worker.ParseInt64Env("MAX_SOURCE_BYTES",20<<20),
  MaxExpandedBytes:worker.ParseInt64Env("MAX_EXPANDED_BYTES",512<<20),
  MaxEntries:worker.ParseIntEnv("MAX_ARCHIVE_ENTRIES",10000),
  MaxCompressionRatio:worker.ParseFloatEnv("MAX_COMPRESSION_RATIO",200),
  Timeout:time.Duration(worker.ParseIntEnv("INSPECTION_TIMEOUT_SECONDS",300))*time.Second,
 });if err!=nil{log.Fatal(err)}
 if err:=instance.Run(ctx);err!=nil{log.Fatal(err)}
}
func envOr(key,fallback string)string{if value:=strings.TrimSpace(os.Getenv(key));value!=""{return value};return fallback}
