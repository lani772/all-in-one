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

func (s *MinIOStore) PresignSourceUpload(ctx context.Context,key string,expires time.Duration)(string,error){
 if s==nil||s.client==nil||!validSourceObjectKey(key){return "",errors.New("invalid source object key")}
 if expires<=0||expires>30*time.Minute{return "",errors.New("source upload URL expiry must be between 1 second and 30 minutes")}
 u,err:=s.client.PresignedPutObject(ctx,s.bucket,key,expires);if err!=nil{return "",fmt.Errorf("presign source upload: %w",err)}
 return u.String(),nil
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
