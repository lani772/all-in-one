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
	Entrypoints []string `json:"entrypoints"`
	DeploymentTarget string `json:"deploymentTarget"`
	Warnings []string `json:"warnings"`
	FilesSeen int `json:"filesSeen"`
}

type packageJSON struct {
	Name string `json:"name"`
	Scripts map[string]string `json:"scripts"`
	Dependencies map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func Inspect(root string) (Result, error) {
	result := Result{Framework:"unknown", PackageManager:"not-applicable", DeploymentTarget:"static", Entrypoints:[]string{}, Warnings:[]string{}}
	exists := func(name string) bool { _, err := os.Stat(filepath.Join(root, name)); return err == nil }
	candidates := []string{"index.html","src/main.tsx","src/main.jsx","src/main.ts","src/main.js","app/page.tsx","app/page.jsx","pages/index.tsx","pages/index.jsx"}
	for _, candidate := range candidates { if exists(candidate) { result.Entrypoints = append(result.Entrypoints, candidate) } }

	switch {
	case exists("pnpm-lock.yaml"): result.PackageManager = "pnpm"
	case exists("yarn.lock"): result.PackageManager = "yarn"
	case exists("bun.lock") || exists("bun.lockb"): result.PackageManager = "bun"
	case exists("package-lock.json"): result.PackageManager = "npm"
	}

	if exists("package.json") {
		data, err := os.ReadFile(filepath.Join(root, "package.json"))
		if err != nil { return result, err }
		var pkg packageJSON
		if err := json.Unmarshal(data, &pkg); err != nil { return result, errors.New("package.json is invalid JSON") }
		result.PackageName = pkg.Name
		if pkg.Scripts != nil { result.BuildScript = pkg.Scripts["build"] }
		deps := map[string]bool{}
		for name := range pkg.Dependencies { deps[name] = true }
		for name := range pkg.DevDependencies { deps[name] = true }
		switch {
		case deps["next"] || exists("next.config.js") || exists("next.config.ts"):
			result.Framework, result.DeploymentTarget = "nextjs", "node-runtime-required"
		case deps["vite"] && deps["react"] || deps["vite"] && deps["react-dom"]:
			result.Framework = "react-vite"
		case deps["vite"] && deps["vue"]:
			result.Framework = "vue-vite"
		case deps["@angular/core"]:
			result.Framework = "angular"
		case deps["@sveltejs/kit"] || deps["svelte"]:
			result.Framework = "svelte"
		case deps["vue"]:
			result.Framework = "vue"
		case deps["react"] || deps["react-dom"]:
			result.Framework = "react"
		case deps["vite"]:
			result.Framework = "vite"
		}
		if result.PackageManager == "not-applicable" { result.PackageManager = "unknown" }
		if result.BuildScript == "" { result.Warnings = append(result.Warnings, "No build script was declared in package.json.") }
		if result.PackageManager == "unknown" { result.Warnings = append(result.Warnings, "No supported dependency lockfile was found; reproducible installation is not confirmed.") }
	} else if exists("index.html") {
		result.Framework = "static-html"
	} else {
		result.Warnings = append(result.Warnings, "No package.json or root index.html was found.")
	}
	if result.Framework == "nextjs" {
		result.Warnings = append(result.Warnings, "Next.js needs a separate sandboxed Node.js runtime; this worker does not execute the project.")
	}
	if result.Framework != "static-html" && len(result.Entrypoints) == 0 {
		result.Warnings = append(result.Warnings, "No recognized frontend entrypoint was found at the project root.")
	}
	if err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil { return err }
		if entry.IsDir() && entry.Name() != "." && (entry.Name() == ".git" || entry.Name() == "node_modules") { return filepath.SkipDir }
		if !entry.IsDir() { result.FilesSeen++ }
		return nil
	}); err != nil { return result, err }
	return result, nil
}

func IsSupportedPackageManager(value string) bool {
	switch strings.ToLower(value) {
	case "npm", "pnpm", "yarn", "bun": return true
	default: return false
	}
}
