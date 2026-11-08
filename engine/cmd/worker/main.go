package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/lani772/all-in-one/engine/internal/buildworker"
	"github.com/lani772/all-in-one/engine/internal/deployment"
	"github.com/lani772/all-in-one/engine/internal/objectstore"
	"github.com/lani772/all-in-one/engine/internal/queue"
)

func main() {
	ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM); defer stop()
	db,err:=deployment.OpenPostgres(ctx,strings.TrimSpace(os.Getenv("DATABASE_URL"))); if err!=nil { log.Fatal(err) }; defer db.Close()
	q,err:=queue.New(envOr("REDIS_ADDR","localhost:6379"),os.Getenv("REDIS_PASSWORD"),envInt("REDIS_DB",0)); if err!=nil { log.Fatal(err) }; defer q.Close()
	if err:=q.Ping(ctx); err!=nil { log.Fatal(err) }
	objects,err:=objectstore.NewMinIOStore(ctx,objectstore.MinIOConfig{
		Endpoint:strings.TrimSpace(os.Getenv("S3_ENDPOINT")),Bucket:strings.TrimSpace(os.Getenv("S3_BUCKET")),
		AccessKey:os.Getenv("S3_ACCESS_KEY"),SecretKey:os.Getenv("S3_SECRET_KEY"),
		Region:envOr("S3_REGION","us-east-1"),UseTLS:envBool("S3_USE_TLS",false),
	}); if err!=nil { log.Fatal(err) }
	workerID:=envOr("WORKER_ID",hostname())
	worker,err:=buildworker.New(db,q,objects,buildworker.Config{
		WorkerID:workerID,MaxArchiveBytes:envInt64("MAX_SOURCE_BYTES",20<<20),
		MaxArchiveEntries:envInt("MAX_ARCHIVE_ENTRIES",10000),MaxFileBytes:envInt64("MAX_ARCHIVE_FILE_BYTES",100<<20),
		MaxExpandedBytes:envInt64("MAX_EXPANDED_BYTES",500<<20),MaxCompressionRatio:envFloat("MAX_COMPRESSION_RATIO",100),
	}); if err!=nil { log.Fatal(err) }
	if err:=worker.Run(ctx); err!=nil { log.Fatal(err) }
}

func hostname() string { h,err:=os.Hostname(); if err!=nil||h=="" { return "inspection-worker" }; return h }
func envOr(key,fallback string)string { if v:=strings.TrimSpace(os.Getenv(key));v!="" { return v }; return fallback }
func envInt(key string,fallback int)int { v:=strings.TrimSpace(os.Getenv(key));if v=="" {return fallback};n,e:=strconv.Atoi(v);if e!=nil||n<0 {log.Fatalf("%s must be a non-negative integer",key)};return n }
func envInt64(key string,fallback int64)int64 { v:=strings.TrimSpace(os.Getenv(key));if v=="" {return fallback};n,e:=strconv.ParseInt(v,10,64);if e!=nil||n<1 {log.Fatalf("%s must be a positive integer",key)};return n }
func envFloat(key string,fallback float64)float64 { v:=strings.TrimSpace(os.Getenv(key));if v=="" {return fallback};n,e:=strconv.ParseFloat(v,64);if e!=nil||n<=0 {log.Fatalf("%s must be positive",key)};return n }
func envBool(key string,fallback bool)bool { v:=strings.ToLower(strings.TrimSpace(os.Getenv(key)));switch v {case "true","1","yes":return true;case "false","0","no":return false;default:return fallback} }
