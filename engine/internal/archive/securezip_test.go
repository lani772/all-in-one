package archive

import (
 "archive/zip"
 "os"
 "path/filepath"
 "strings"
 "testing"
)
type zipEntry struct{name,body string;mode os.FileMode}
func makeZip(t *testing.T,entries []zipEntry)string{
 t.Helper();file:=filepath.Join(t.TempDir(),"source.zip");out,err:=os.Create(file);if err!=nil{t.Fatal(err)}
 writer:=zip.NewWriter(out)
 for _,item:=range entries{header:=&zip.FileHeader{Name:item.name};header.SetMode(item.mode);entry,err:=writer.CreateHeader(header);if err!=nil{t.Fatal(err)};if _,err:=entry.Write([]byte(item.body));err!=nil{t.Fatal(err)}}
 if err:=writer.Close();err!=nil{t.Fatal(err)};if err:=out.Close();err!=nil{t.Fatal(err)};return file
}
func TestExtractValidArchive(t *testing.T){
 z:=makeZip(t,[]zipEntry{{"index.html","hello",0644},{"assets/app.js","console.log(1)",0644}})
 dest:=filepath.Join(t.TempDir(),"out");if err:=Extract(z,dest,Limits{});err!=nil{t.Fatal(err)}
 got,err:=os.ReadFile(filepath.Join(dest,"assets","app.js"));if err!=nil{t.Fatal(err)};if string(got)!="console.log(1)"{t.Fatalf("unexpected content: %q",got)}
}
func TestRejectsTraversalAndAbsolutePaths(t *testing.T){
 for _,name:=range []string{"../escape.txt","/absolute.txt","C:/drive.txt","a/../../escape.txt"}{
  z:=makeZip(t,[]zipEntry{{name,"x",0644}});err:=Extract(z,filepath.Join(t.TempDir(),"out"),Limits{})
  if err==nil||!strings.Contains(err.Error(),"unsafe"){t.Errorf("entry %q was not rejected: %v",name,err)}
 }
}
func TestRejectsSymlinksAndLimits(t *testing.T){
 z:=makeZip(t,[]zipEntry{{"link","target",os.ModeSymlink|0777}})
 if err:=Extract(z,filepath.Join(t.TempDir(),"out"),Limits{});err==nil{t.Fatal("symlink was accepted")}
 z=makeZip(t,[]zipEntry{{"large.txt",strings.Repeat("x",20),0644}})
 if err:=Extract(z,filepath.Join(t.TempDir(),"out"),Limits{MaxExpandedBytes:10});err==nil{t.Fatal("oversized archive was accepted")}
}
func TestRejectsDuplicateEntries(t *testing.T){
 z:=makeZip(t,[]zipEntry{{"index.html","one",0644},{"index.html","two",0644}})
 if err:=Extract(z,filepath.Join(t.TempDir(),"out"),Limits{});err==nil{t.Fatal("duplicate entry was accepted")}
}

func TestRejectsCompressionRatioAndEntryCount(t *testing.T) {
 file := filepath.Join(t.TempDir(), "bomb.zip")
 out, err := os.Create(file); if err != nil { t.Fatal(err) }
 writer := zip.NewWriter(out)
 header := &zip.FileHeader{Name:"bomb.txt", Method:zip.Deflate}
 entry, err := writer.CreateHeader(header); if err != nil { t.Fatal(err) }
 if _, err := entry.Write([]byte(strings.Repeat("a", 64*1024))); err != nil { t.Fatal(err) }
 if err := writer.Close(); err != nil { t.Fatal(err) }; if err := out.Close(); err != nil { t.Fatal(err) }
 if err := Extract(file, filepath.Join(t.TempDir(),"out"), Limits{MaxCompressionRatio:10}); err == nil { t.Fatal("high compression ratio was accepted") }

 z := makeZip(t, []zipEntry{{"a.txt","a",0644},{"b.txt","b",0644}})
 if err := Extract(z, filepath.Join(t.TempDir(),"out"), Limits{MaxEntries:1}); err == nil { t.Fatal("too many entries were accepted") }
}
