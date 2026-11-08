package archive

import (
 "archive/zip"
 "errors"
 "fmt"
 "io"
 "io/fs"
 "os"
 "path"
 "path/filepath"
 "strings"
)

var (
 ErrUnsafeArchive = errors.New("archive contains an unsafe path or file type")
 ErrArchiveTooLarge = errors.New("archive exceeds configured extraction limits")
)

type Limits struct { MaxEntries int; MaxFileBytes int64; MaxExpandedBytes int64; MaxCompressionRatio float64 }
func (l Limits) defaults() Limits {
 if l.MaxEntries<=0 {l.MaxEntries=10000}
 if l.MaxFileBytes<=0 {l.MaxFileBytes=100<<20}
 if l.MaxExpandedBytes<=0 {l.MaxExpandedBytes=512<<20}
 if l.MaxCompressionRatio<=0 {l.MaxCompressionRatio=200}
 return l
}

func Extract(zipPath,destination string,limits Limits) error {
 limits=limits.defaults()
 reader,err:=zip.OpenReader(zipPath);if err!=nil{return fmt.Errorf("open source ZIP: %w",err)}
 defer reader.Close()
 if len(reader.File)==0||len(reader.File)>limits.MaxEntries{return ErrArchiveTooLarge}
 if err:=os.Mkdir(destination,0700);err!=nil{return fmt.Errorf("create extraction root: %w",err)}
 rootInfo,err:=os.Lstat(destination);if err!=nil||!rootInfo.IsDir()||rootInfo.Mode()&os.ModeSymlink!=0{return ErrUnsafeArchive}
 root,err:=filepath.Abs(destination);if err!=nil{return err}
 seen:=make(map[string]bool,len(reader.File)*2)
 var expanded int64
 for _,entry:=range reader.File {
  name:=entry.Name
  if name==""||strings.ContainsRune(name,92)||strings.ContainsRune(name,0)||strings.HasPrefix(name,"/")||hasDrivePrefix(name) {
   return fmt.Errorf("%w: invalid ZIP entry name %q",ErrUnsafeArchive,name)
  }
  isDir:=strings.HasSuffix(name,"/")
  trimmed:=strings.TrimSuffix(name,"/")
  clean:=path.Clean(trimmed)
  if trimmed==""||clean=="."||clean==".."||strings.HasPrefix(clean,"../")||clean!=trimmed||!fs.ValidPath(clean)||path.IsAbs(clean) {
   return fmt.Errorf("%w: traversal entry %q",ErrUnsafeArchive,name)
  }
  mode:=entry.Mode()
  if mode&os.ModeSymlink!=0||(!isDir&&!mode.IsRegular())||(isDir&&!mode.IsDir()&&mode.Type()!=0) {
   return fmt.Errorf("%w: unsupported ZIP entry type %q",ErrUnsafeArchive,name)
  }
  if existing,ok:=seen[clean];ok {if isDir&&existing{continue};return fmt.Errorf("%w: duplicate ZIP entry %q",ErrUnsafeArchive,name)}
  if err:=recordParents(seen,clean);err!=nil{return err}
  seen[clean]=isDir
  target:=filepath.Join(root,filepath.FromSlash(clean))
  rel,err:=filepath.Rel(root,target);if err!=nil||!filepath.IsLocal(rel){return ErrUnsafeArchive}
  if isDir {if err:=os.MkdirAll(target,0700);err!=nil{return err};continue}
  size:=int64(entry.UncompressedSize64);compressed:=int64(entry.CompressedSize64)
  if size<0||compressed<0||size>limits.MaxFileBytes||size>limits.MaxExpandedBytes||expanded>limits.MaxExpandedBytes-size{return ErrArchiveTooLarge}
  if size>0&&compressed==0{return ErrArchiveTooLarge}
  if compressed>0&&float64(size)/float64(compressed)>limits.MaxCompressionRatio{return fmt.Errorf("%w: compression ratio too high for %q",ErrArchiveTooLarge,name)}
  expanded+=size
  if err:=os.MkdirAll(filepath.Dir(target),0700);err!=nil{return err}
  file,err:=os.OpenFile(target,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if err!=nil{return fmt.Errorf("create extracted file %q: %w",name,err)}
  source,err:=entry.Open();if err!=nil{file.Close();return err}
  n,copyErr:=io.Copy(file,io.LimitReader(source,size+1))
  sourceCloseErr:=source.Close();fileCloseErr:=file.Close()
  if copyErr!=nil{return fmt.Errorf("extract %q: %w",name,copyErr)}
  if sourceCloseErr!=nil{return fmt.Errorf("verify ZIP entry %q: %w",name,sourceCloseErr)}
  if fileCloseErr!=nil{return fileCloseErr}
  if n!=size{return fmt.Errorf("%w: extracted size mismatch for %q",ErrUnsafeArchive,name)}
 }
 return nil
}
func hasDrivePrefix(name string)bool{return len(name)>=2&&((name[0]>='A'&&name[0]<='Z')||(name[0]>='a'&&name[0]<='z'))&&name[1]==':'}
func recordParents(seen map[string]bool,name string)error{
 parts:=strings.Split(name,"/")
 for i:=1;i<len(parts);i++{parent:=strings.Join(parts[:i],"/");if isDir,ok:=seen[parent];ok&&!isDir{return fmt.Errorf("%w: file is used as a parent directory %q",ErrUnsafeArchive,parent)};seen[parent]=true}
 return nil
}
