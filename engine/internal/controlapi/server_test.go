package controlapi

import "testing"

func TestSlugify(t *testing.T) {
 cases:=map[string]string{"My Frontend Project":"my-frontend-project","  Portfolio 2026  ":"portfolio-2026","!!!":"project","React + Vite App":"react-vite-app"}
 for input,want:=range cases {if got:=slugify(input);got!=want {t.Errorf("slugify(%q)=%q, want %q",input,got,want)}}
}
func TestValidArchiveName(t *testing.T) {
 for _,name:=range []string{"site.zip","SITE.ZIP","project-1.zip"} {if !validArchiveName(name){t.Errorf("expected %q accepted",name)}}
 for _,name:=range []string{"","../site.zip","folder/site.zip","site.tar.gz","site.zip/evil","bad\\site.zip"} {if validArchiveName(name){t.Errorf("expected %q rejected",name)}}
}
func TestValidSourceKey(t *testing.T) {
 if !validSourceKey("sources/project1/upload1/source.zip","project1"){t.Fatal("expected valid source key")}
 for _,key:=range []string{"sources/project2/upload1/source.zip","sources/project1/../source.zip","sources/project1/upload1/source.zip/extra"} {if validSourceKey(key,"project1"){t.Errorf("expected %q rejected",key)}}
}
