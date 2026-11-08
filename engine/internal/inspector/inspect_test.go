package inspector

import (
 "os"
 "path/filepath"
 "testing"
)
func TestDetectsReactVite(t *testing.T){
 root:=t.TempDir();write:=func(name,body string){t.Helper();p:=filepath.Join(root,name);if err:=os.MkdirAll(filepath.Dir(p),0700);err!=nil{t.Fatal(err)};if err:=os.WriteFile(p,[]byte(body),0600);err!=nil{t.Fatal(err)}}
 write("package.json",`{"name":"demo","scripts":{"build":"vite build"},"dependencies":{"react":"^19","vite":"^6"}}`);write("pnpm-lock.yaml","lockfileVersion: '9.0'");write("src/main.tsx","")
 got,err:=Inspect(root);if err!=nil{t.Fatal(err)};if got.Framework!="react-vite"||got.PackageManager!="pnpm"||got.OutputDirectory!="dist"||got.BuildScript!="build"{t.Fatalf("unexpected inspection: %+v",got)}
}
func TestDetectsNextStaticExport(t *testing.T){
 root:=t.TempDir();if err:=os.WriteFile(filepath.Join(root,"package.json"),[]byte(`{"dependencies":{"next":"15","react":"19"}}`),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,"next.config.ts"),[]byte(`const config = { output: 'export' }; export default config;`),0600);err!=nil{t.Fatal(err)}
 got,err:=Inspect(root);if err!=nil{t.Fatal(err)};if got.Framework!="nextjs"||got.DeploymentTarget!="static"||got.OutputDirectory!="out"{t.Fatalf("unexpected inspection: %+v",got)}
}
func TestInvalidPackageJSONRejected(t *testing.T){
 root:=t.TempDir();if err:=os.WriteFile(filepath.Join(root,"package.json"),[]byte("{"),0600);err!=nil{t.Fatal(err)}
 if _,err:=Inspect(root);err==nil{t.Fatal("expected invalid package.json error")}
}
