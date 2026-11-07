package objectstore

import (
 "context"
 "errors"
 "fmt"
 "regexp"
 "strings"
 "time"

 "github.com/minio/minio-go/v7"
)

var sourceIDPattern=regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")

func (s *MinIOStore) PresignSourceUpload(ctx context.Context,key string,expires time.Duration,size int64)(string,map[string]string,error){
 if s==nil||s.client==nil||!validSourceObjectKey(key){return "",nil,errors.New("invalid source object key")}
 if expires<=0||expires>30*time.Minute{return "",nil,errors.New("source upload policy expiry must be between 1 second and 30 minutes")}
 if size<1||size>100<<20{return "",nil,errors.New("source upload size is outside the allowed range")}
 policy:=minio.NewPostPolicy()
 if err:=policy.SetBucket(s.bucket);err!=nil{return "",nil,err}
 if err:=policy.SetKey(key);err!=nil{return "",nil,err}
 if err:=policy.SetExpires(time.Now().UTC().Add(expires));err!=nil{return "",nil,err}
 if err:=policy.SetContentType("application/zip");err!=nil{return "",nil,err}
 if err:=policy.SetContentLengthRange(size,size);err!=nil{return "",nil,err}
 u,fields,err:=s.client.PresignedPostPolicy(ctx,*policy)
 if err!=nil{return "",nil,fmt.Errorf("presign bounded source upload: %w",err)}
 return u.String(),fields,nil
}
func (s *MinIOStore) StatSource(ctx context.Context,key string)(int64,error){
 if s==nil||s.client==nil||!validSourceObjectKey(key){return 0,errors.New("invalid source object key")}
 info,err:=s.client.StatObject(ctx,s.bucket,key,minio.StatObjectOptions{});if err!=nil{return 0,fmt.Errorf("stat source upload: %w",err)}
 return info.Size,nil
}
func (s *MinIOStore) Ready(ctx context.Context)error{
 if s==nil||s.client==nil||s.bucket==""{return errors.New("object storage is not configured")}
 exists,err:=s.client.BucketExists(ctx,s.bucket);if err!=nil{return fmt.Errorf("check object storage: %w",err)}
 if !exists{return errors.New("artifact bucket is missing")};return nil
}
func validSourceObjectKey(key string)bool{
 if key==""||strings.Contains(key,"\\")||strings.Contains(key,".."){return false}
 p:=strings.Split(key,"/")
 return len(p)==4&&p[0]=="sources"&&sourceIDPattern.MatchString(p[1])&&sourceIDPattern.MatchString(p[2])&&p[3]=="source.zip"
}
