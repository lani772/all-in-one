package objectstore

import "testing"

func TestValidSourceObjectKey(t *testing.T){
 for _,key:=range []string{"sources/project1/upload1/source.zip","sources/project_1/upload-2/source.zip"}{if !validSourceObjectKey(key){t.Errorf("expected %q valid",key)}}
}
func TestRejectsUnsafeSourceObjectKeys(t *testing.T){
 for _,key:=range []string{"","/sources/project1/upload1/source.zip","sources/../upload1/source.zip","sources/project1/upload1/../../secret","sources/project1/upload1/source.zip/extra","sites/project1/deployments/deploy1/files/index.html","sources/project1/upload1\\source.zip"}{if validSourceObjectKey(key){t.Errorf("expected %q rejected",key)}}
}
