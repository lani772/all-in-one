package inspector

import (
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "strings"
)

type Result struct {
 Framework string `json:"framework"`
 PackageManager string `json:"packageManager"`
 PackageName string `json:"packageName,omitempty"`
 BuildScript string `json:"buildScript,omitempty"`
 DeploymentTarget string `json:"deploymentTarget"`
 OutputDirectory string `json:"outputDirectory,omitempty"`
 Entrypoints []string `json:"entrypoints"`
 Warnings []string `json:"warnings"`
 FilesSeen int `json:"filesSeen"`
}
type packageJSON struct {
 Name string `json:"name"`
 Scripts map[string]string `json:"scripts"`
 Dependencies map[string]string `json:"dependencies"`
 DevDependencies map[string]string `json:"devDependencies"`
}

func Inspect(root string)(Result,error){
 result:=Result{Framework:"unknown",PackageManager:"unknown",DeploymentTarget:"static",Entrypoints:[]string{},Warnings:[]string{}}
 exists:=func(name string)bool{info,err:=os.Lstat(filepath.Join(root,name));return err==nil&&!info.IsDir()&&info.Mode().IsRegular()}
 for _,name:=range []string{"index.html","package.json","src/main.tsx","src/main.jsx","src/main.ts","src/main.js","app/page.tsx","app/page.jsx","pages/index.tsx","pages/index.jsx","vite.config.ts","vite.config.js","next.config.js","next.config.mjs","next.config.ts","next.config.mts","angular.json","svelte.config.js","svelte.config.ts"}{if exists(name){result.Entrypoints=append(result.Entrypoints,name)}}
 switch{case exists("pnpm-lock.yaml"):result.PackageManager="pnpm";case exists("yarn.lock"):result.PackageManager="yarn";case exists("bun.lock")||exists("bun.lockb"):result.PackageManager="bun";case exists("package-lock.json"):result.PackageManager="npm"}
 if exists("package.json"){
  data,err:=os.ReadFile(filepath.Join(root,"package.json"));if err!=nil{return result,err};if len(data)>2<<20{return result,errors.New("package.json exceeds inspection limit")}
  var pkg packageJSON;if err:=json.Unmarshal(data,&pkg);err!=nil{return result,errors.New("package.json is invalid JSON")}
  result.PackageName=pkg.Name;deps:=map[string]bool{};for name:=range pkg.Dependencies{deps[name]=true};for name:=range pkg.DevDependencies{deps[name]=true}
  switch{
  case deps["next"]||exists("next.config.js")||exists("next.config.mjs")||exists("next.config.ts")||exists("next.config.mts"):
   result.Framework="nextjs";config:=readNextConfig(root)
   if strings.Contains(config,"output: 'export'")||strings.Contains(config,`output: "export"`){result.DeploymentTarget="static";result.OutputDirectory="out"}else{result.DeploymentTarget="node-runtime";result.Warnings=append(result.Warnings,"Next.js server-side rendering requires a separately managed Node.js runtime.")}
  case deps["vite"]&&deps["react"]:result.Framework="react-vite";result.OutputDirectory="dist"
  case deps["vite"]&&deps["vue"]:result.Framework="vue-vite";result.OutputDirectory="dist"
  case deps["vite"]:result.Framework="vite";result.OutputDirectory="dist"
  case deps["@angular/core"]:result.Framework="angular";result.OutputDirectory="dist"
  case deps["@sveltejs/kit"]:result.Framework="sveltekit";result.DeploymentTarget="node-runtime";result.Warnings=append(result.Warnings,"SvelteKit server output requires a managed runtime adapter.")
  case deps["svelte"]:result.Framework="svelte";result.OutputDirectory="dist"
  case deps["vue"]:result.Framework="vue";result.OutputDirectory="dist"
  case deps["react"]||deps["react-dom"]:result.Framework="react";result.OutputDirectory="build"
  case exists("index.html"):result.Framework="static-html"
  }
  if pkg.Scripts["build"]!=""{result.BuildScript="build"}else if result.Framework!="static-html"{result.Warnings=append(result.Warnings,"No build script was declared in package.json.")}
 }else if exists("index.html"){result.Framework="static-html"}else{result.Warnings=append(result.Warnings,"No package.json or index.html was found at the project root.")}
 if result.PackageManager=="unknown"&&exists("package.json"){result.Warnings=append(result.Warnings,"No supported package-manager lockfile was found; dependency installation is not yet reproducible.")}
 _=filepath.WalkDir(root,func(_ string,entry os.DirEntry,err error)error{if err!=nil{return err};if entry.IsDir(){switch strings.ToLower(entry.Name()){case ".git","node_modules",".next":return filepath.SkipDir}};if !entry.IsDir(){result.FilesSeen++};return nil})
 return result,nil
}
func readNextConfig(root string)string{
 for _,name:=range []string{"next.config.js","next.config.mjs","next.config.ts","next.config.mts"}{data,err:=os.ReadFile(filepath.Join(root,name));if err==nil&&len(data)<=1<<20{return string(data)}}
 return ""
}
