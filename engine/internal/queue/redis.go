package queue

import (
 "context"
 "errors"
 "fmt"
 "strings"
 "time"

 "github.com/redis/go-redis/v9"
)
const(DefaultStream="hosting:inspection";DefaultGroup="inspection-workers")
type Message struct{ID string;JobID string}
type Queue struct{client *redis.Client;stream string;group string}
func New(addr,password string,db int)(*Queue,error){
 if strings.TrimSpace(addr)==""{return nil,errors.New("REDIS_ADDR is required")}
 client:=redis.NewClient(&redis.Options{Addr:addr,Password:password,DB:db,DialTimeout:5*time.Second,ReadTimeout:5*time.Second,WriteTimeout:5*time.Second})
 return &Queue{client:client,stream:DefaultStream,group:DefaultGroup},nil
}
func(q *Queue)Close()error{if q==nil||q.client==nil{return nil};return q.client.Close()}
func(q *Queue)Ping(ctx context.Context)error{if q==nil||q.client==nil{return errors.New("Redis queue is not configured")};return q.client.Ping(ctx).Err()}
func(q *Queue)EnsureGroup(ctx context.Context)error{
 err:=q.client.XGroupCreateMkStream(ctx,q.stream,q.group,"0").Err()
 if err!=nil&&!strings.Contains(err.Error(),"BUSYGROUP"){return fmt.Errorf("create inspection consumer group: %w",err)}
 return nil
}
func(q *Queue)Publish(ctx context.Context,jobID string)error{
 if jobID==""{return errors.New("inspection job id is required")}
 _,err:=q.client.XAdd(ctx,&redis.XAddArgs{Stream:q.stream,Values:map[string]interface{}{"job_id":jobID}}).Result()
 if err!=nil{return fmt.Errorf("publish inspection job: %w",err)};return nil
}
func(q *Queue)Read(ctx context.Context,consumer string,block time.Duration)([]Message,error){
 streams,err:=q.client.XReadGroup(ctx,&redis.XReadGroupArgs{Group:q.group,Consumer:consumer,Streams:[]string{q.stream,">"},Count:10,Block:block}).Result()
 if errors.Is(err,redis.Nil){return nil,nil};if err!=nil{return nil,fmt.Errorf("read inspection stream: %w",err)}
 var out []Message;for _,stream:=range streams{for _,entry:=range stream.Messages{if value,ok:=entry.Values["job_id"].(string);ok{out=append(out,Message{ID:entry.ID,JobID:value})}}};return out,nil
}
func(q *Queue)ClaimStale(ctx context.Context,consumer string,minIdle time.Duration)([]Message,error){
 items,_,err:=q.client.XAutoClaim(ctx,&redis.XAutoClaimArgs{Stream:q.stream,Group:q.group,Consumer:consumer,MinIdle:minIdle,Start:"0-0",Count:10}).Result()
 if err!=nil{return nil,fmt.Errorf("claim abandoned inspection messages: %w",err)}
 out:=make([]Message,0,len(items));for _,entry:=range items{if value,ok:=entry.Values["job_id"].(string);ok{out=append(out,Message{ID:entry.ID,JobID:value})}};return out,nil
}
func(q *Queue)Ack(ctx context.Context,id string)error{if id==""{return errors.New("Redis stream message id is required")};return q.client.XAck(ctx,q.stream,q.group,id).Err()}
