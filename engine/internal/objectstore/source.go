package objectstore

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "io"
 "os"
 "regexp"
 "strings"
 "time"

 "github.com/minio/minio-go/v7"
)

var sourceIDPattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")

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
 u,fields,err:=s.client.PresignedPostPolicy(ctx,policy)
 if err!=nil{return "",nil,fmt.Errorf("presign bounded source upload: %w",err)}
 return u.String(),fields,nil
}

func (s *MinIOStore) StatSource(ctx context.Context,key string)(int64,error){
 if s==nil||s.client==nil||!validSourceObjectKey(key){return 0,errors.New("invalid source object key")}
 info,err:=s.client.StatObject(ctx,s.bucket,key,minio.StatObjectOptions{})
 if err!=nil{return 0,fmt.Errorf("stat source upload: %w",err)}
 return info.Size,nil
}

// SealSource copies the mutable upload into a service-owned key and hashes the copy.
// The browser is never given a write policy for the sealed-sources namespace.
func (s *MinIOStore) SealSource(ctx context.Context,sourceKey,sealedKey string,expectedSize int64)(string,error){
 if s==nil||s.client==nil||!validSourceObjectKey(sourceKey)||!validSealedSourceKey(sealedKey){return "",errors.New("invalid source sealing keys")}
 if expectedSize<1||expectedSize>100<<20{return "",errors.New("source size is outside the sealing limit")}
 _,err:=s.client.CopyObject(ctx,minio.CopyDestOptions{Bucket:s.bucket,Object:sealedKey},minio.CopySrcOptions{Bucket:s.bucket,Object:sourceKey})
 if err!=nil{return "",fmt.Errorf("copy source into sealed namespace: %w",err)}
 cleanup:=func(){_ = s.client.RemoveObject(context.WithoutCancel(ctx),s.bucket,sealedKey,minio.RemoveObjectOptions{})}
 object,err:=s.client.GetObject(ctx,s.bucket,sealedKey,minio.GetObjectOptions{})
 if err!=nil{cleanup();return "",fmt.Errorf("open sealed source: %w",err)}
 defer object.Close()
 info,err:=object.Stat();if err!=nil{cleanup();return "",fmt.Errorf("stat sealed source: %w",err)}
 if info.Size!=expectedSize{cleanup();return "",fmt.Errorf("sealed source size mismatch: got %d, expected %d",info.Size,expectedSize)}
 hash:=sha256.New();n,err:=io.Copy(hash,io.LimitReader(object,expectedSize+1))
 if err!=nil{cleanup();return "",fmt.Errorf("hash sealed source: %w",err)}
 if n!=expectedSize{cleanup();return "",fmt.Errorf("sealed source read %d bytes, expected %d",n,expectedSize)}
 return hex.EncodeToString(hash.Sum(nil)),nil
}

func (s *MinIOStore) DownloadSealedSource(ctx context.Context,key,destination string,maxBytes int64)(int64,string,error){
 if s==nil||s.client==nil||!validSealedSourceKey(key)||destination==""||maxBytes<1{return 0,"",errors.New("invalid sealed source download request")}
 object,err:=s.client.GetObject(ctx,s.bucket,key,minio.GetObjectOptions{});if err!=nil{return 0,"",fmt.Errorf("open sealed source: %w",err)}
 defer object.Close()
 info,err:=object.Stat();if err!=nil{return 0,"",fmt.Errorf("stat sealed source: %w",err)}
 if info.Size<1||info.Size>maxBytes{return 0,"",errors.New("sealed source exceeds configured archive size limit")}
 file,err:=os.OpenFile(destination,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if err!=nil{return 0,"",err}
 defer file.Close()
 hash:=sha256.New();n,copyErr:=io.Copy(io.MultiWriter(file,hash),io.LimitReader(object,maxBytes+1))
 if copyErr!=nil{_ = os.Remove(destination);return n,"",fmt.Errorf("download sealed source: %w",copyErr)}
 if n!=info.Size||n>maxBytes{_ = os.Remove(destination);return n,"",fmt.Errorf("sealed source length changed: got %d, expected %d",n,info.Size)}
 return n,hex.EncodeToString(hash.Sum(nil)),nil
}

func (s *MinIOStore) DeleteSource(ctx context.Context,key string)error{
 if s==nil||s.client==nil||(!validSourceObjectKey(key)&&!validSealedSourceKey(key)){return errors.New("invalid source deletion key")}
 if err:=s.client.RemoveObject(ctx,s.bucket,key,minio.RemoveObjectOptions{});err!=nil{return fmt.Errorf("delete source object: %w",err)}
 return nil
}
func (s *MinIOStore) Ready(ctx context.Context)error{
 if s==nil||s.client==nil||s.bucket==""{return errors.New("object storage is not configured")}
 exists,err:=s.client.BucketExists(ctx,s.bucket);if err!=nil{return fmt.Errorf("check object storage: %w",err)}
 if !exists{return errors.New("artifact bucket is missing")};return nil
}
func validSourceObjectKey(key string)bool{
 parts:=strings.Split(key,"/")
 return len(parts)==4&&parts[0]=="sources"&&sourceIDPattern.MatchString(parts[1])&&sourceIDPattern.MatchString(parts[2])&&parts[3]=="source.zip"
}
func validSealedSourceKey(key string)bool{
 parts:=strings.Split(key,"/")
 return len(parts)==4&&parts[0]=="sealed-sources"&&sourceIDPattern.MatchString(parts[1])&&sourceIDPattern.MatchString(parts[2])&&parts[3]=="source.zip"
}
