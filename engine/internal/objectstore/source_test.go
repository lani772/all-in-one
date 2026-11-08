package objectstore

import "testing"

func TestValidSourceObjectKeys(t *testing.T){
 for _,key:=range []string{"sources/project1/upload1/source.zip","sources/project_1/upload-2/source.zip"}{if !validSourceObjectKey(key){t.Errorf("expected %q valid",key)}}
}
func TestRejectsUnsafeSourceObjectKeys(t *testing.T){
 for _,key:=range []string{"","/sources/project1/upload1/source.zip","sources/../upload1/source.zip","sources/project1/upload1/../../secret","sources/project1/upload1/source.zip/extra", "sources/project1/upload1\\source.zip"}{if validSourceObjectKey(key){t.Errorf("expected %q rejected",key)}}
}
func TestSealedNamespaceIsSeparate(t *testing.T){
 if !validSealedSourceKey("sealed-sources/project1/upload1/source.zip"){t.Fatal("expected valid sealed key")}
 for _,key:=range []string{"sources/project1/upload1/source.zip","sealed-sources/../upload1/source.zip","sealed-sources/project1/upload1/source.zip/extra"}{if validSealedSourceKey(key){t.Errorf("expected %q rejected",key)}}
}
