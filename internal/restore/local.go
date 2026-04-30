package restore

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// deepDirs are top-level directories that should be grouped at two levels
// (dir/subdir) to preserve per-package granularity.
var deepDirs = map[string]bool{
	"internal": true, "src": true, "pkg": true, "lib": true, "app": true,
	"cmd": true, "pages": true, "routes": true, "components": true,
	"hooks": true, "store": true, "features": true, "views": true,
	"containers": true, "screens": true, "api": true, "controllers": true,
	"services": true, "middleware": true, "handlers": true,
}

// ignoreDirs are directory names excluded from the local scan.
var ignoreDirs = map[string]bool{
	".git": true, ".svn": true, ".hg": true, "node_modules": true,
	"vendor": true, "__pycache__": true, ".cache": true, "dist": true,
	"build": true, "target": true, ".tox": true, "venv": true,
	".venv": true, "coverage": true, ".nyc_output": true, "out": true,
	".next": true, ".nuxt": true, ".turbo": true, "Pods": true,
	"elm-stuff": true, "_build": true, "env": true, "docs-output": true,
}

// extToLanguage maps common file extensions to language display names.
var extToLanguage = map[string]string{
	".go": "Go", ".js": "JavaScript", ".ts": "TypeScript", ".tsx": "TypeScript",
	".jsx": "JavaScript", ".py": "Python", ".rb": "Ruby", ".rs": "Rust",
	".java": "Java", ".kt": "Kotlin", ".swift": "Swift", ".cs": "C#",
	".cpp": "C++", ".c": "C", ".h": "C", ".php": "PHP", ".scala": "Scala",
	".elm": "Elm", ".ex": "Elixir", ".exs": "Elixir", ".sh": "Shell",
	".bash": "Shell", ".zig": "Zig", ".lua": "Lua", ".r": "R", ".jl": "Julia",
}

// BuildProjectGraph generates a ProjectGraph from local repository analysis
// with no external API calls.
func BuildProjectGraph(ctx context.Context, rootDir, projectName string) (*ProjectGraph, error) {
	extCounts, dirFiles, totalFiles, err := collectFiles(ctx, rootDir)
	if err != nil {
		return nil, err
	}
	lang, languages := detectLanguages(extCounts)
	desc := readDescription(rootDir)
	domains := buildDomains(dirFiles)

	g := &ProjectGraph{
		Name:         projectName,
		Language:     lang,
		Description:  desc,
		Domains:      domains,
		ExternalDeps: DetectExternalDeps(rootDir),
		Stats: Stats{
			TotalFiles: totalFiles,
			Languages:  languages,
		},
		UpdatedAt: time.Now(),
	}
	g.CriticalFiles = localTopFiles(g.Domains, 10)
	return g, nil
}

// ReadClaudeMD reads and returns the contents of CLAUDE.md from rootDir,
// truncated to 3 000 runes. Returns "" if the file is absent.
func ReadClaudeMD(rootDir string) string {
	data, err := os.ReadFile(filepath.Join(rootDir, "CLAUDE.md"))
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(data))
	const maxRunes = 3000
	runes := []rune(content)
	if len(runes) > maxRunes {
		content = string(runes[:maxRunes]) + "\n\n*(CLAUDE.md truncated — showing first 3000 chars)*"
	}
	return content
}

// DetectExternalDeps scans rootDir for common dependency manifests and returns
// up to 15 top-level dependency names. Supports go.mod, package.json,
// requirements.txt, Cargo.toml, Gemfile, and pyproject.toml.
func DetectExternalDeps(rootDir string) []string { //nolint:gocyclo // manifest-per-format parsing; splitting would obscure the intent
	const maxDeps = 15
	seen := make(map[string]bool)
	var deps []string
	var npmRuntime, npmDev []string

	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		deps = append(deps, name)
	}

	// go.mod
	if data, err := os.ReadFile(filepath.Join(rootDir, "go.mod")); err == nil {
		inRequire := false
		ownModule := ""
		for _, line := range strings.Split(string(data), "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "module ") {
				if f := strings.Fields(t); len(f) >= 2 {
					ownModule = f[1]
				}
				continue
			}
			if t == "require (" {
				inRequire = true
				continue
			}
			if inRequire && t == ")" {
				inRequire = false
				continue
			}
			var mod string
			if strings.HasPrefix(t, "require ") {
				if f := strings.Fields(t); len(f) >= 2 {
					mod = f[1]
				}
			} else if inRequire {
				if i := strings.Index(t, "//"); i >= 0 {
					t = strings.TrimSpace(t[:i])
				}
				if f := strings.Fields(t); len(f) >= 1 {
					mod = f[0]
				}
			}
			if mod == "" || mod == ownModule {
				continue
			}
			segs := strings.Split(mod, "/")
			add(segs[len(segs)-1])
		}
	}

	// package.json — split runtime vs devDeps so runtime gets priority.
	if data, err := os.ReadFile(filepath.Join(rootDir, "package.json")); err == nil {
		var pkg struct {
			Dependencies    map[string]json.RawMessage `json:"dependencies"`
			DevDependencies map[string]json.RawMessage `json:"devDependencies"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			for name := range pkg.Dependencies {
				if name = strings.TrimSpace(name); name != "" {
					npmRuntime = append(npmRuntime, name)
				}
			}
			for name := range pkg.DevDependencies {
				if name = strings.TrimSpace(name); name != "" {
					npmDev = append(npmDev, name)
				}
			}
		}
	}

	// requirements.txt
	if data, err := os.ReadFile(filepath.Join(rootDir, "requirements.txt")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			name := line
			if i := strings.Index(name, " @ "); i >= 0 {
				name = name[:i]
			}
			for _, sep := range []string{";", " #", "[", "==", ">=", "<=", "!=", "~=", ">", "<"} {
				if i := strings.Index(name, sep); i >= 0 {
					name = name[:i]
				}
			}
			add(name)
		}
	}

	// Cargo.toml
	if data, err := os.ReadFile(filepath.Join(rootDir, "Cargo.toml")); err == nil {
		inDeps := false
		depth := 0
		for _, line := range strings.Split(string(data), "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "[") {
				inDeps = t == "[dependencies]" || t == "[dev-dependencies]" || t == "[build-dependencies]" ||
					t == "[workspace.dependencies]" || t == "[workspace.dev-dependencies]" || t == "[workspace.build-dependencies]"
				depth = 0
				continue
			}
			opens := strings.Count(t, "{")
			closes := strings.Count(t, "}")
			if inDeps && depth == 0 && strings.Contains(t, "=") && !strings.HasPrefix(t, "#") {
				parts := strings.SplitN(t, "=", 2)
				add(strings.TrimSpace(parts[0]))
			}
			depth += opens - closes
			if depth < 0 {
				depth = 0
			}
		}
	}

	// Gemfile
	if data, err := os.ReadFile(filepath.Join(rootDir, "Gemfile")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			t := strings.TrimSpace(line)
			if !strings.HasPrefix(t, "gem ") && !strings.HasPrefix(t, "gem\t") {
				continue
			}
			rest := strings.TrimSpace(t[3:])
			for _, q := range []string{"'", `"`} {
				if strings.HasPrefix(rest, q) {
					if end := strings.Index(rest[1:], q); end >= 0 {
						add(rest[1 : end+1])
						break
					}
				}
			}
		}
	}

	// pyproject.toml
	if data, err := os.ReadFile(filepath.Join(rootDir, "pyproject.toml")); err == nil {
		inPoetryDeps := false
		inProjectSection := false
		inProjectDepsArray := false
		for _, line := range strings.Split(string(data), "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "[") {
				inPoetryDeps = t == "[tool.poetry.dependencies]" || t == "[tool.poetry.dev-dependencies]"
				inProjectSection = t == "[project]"
				if !inProjectSection {
					inProjectDepsArray = false
				}
				continue
			}
			if inProjectSection && !inProjectDepsArray {
				if strings.HasPrefix(t, "dependencies") && strings.Contains(t, "=") {
					eqIdx := strings.Index(t, "=")
					rest := strings.TrimSpace(t[eqIdx+1:])
					openIdx := strings.Index(rest, "[")
					closeIdx := strings.Index(rest, "]")
					if openIdx >= 0 && closeIdx > openIdx {
						for _, part := range strings.Split(rest[openIdx+1:closeIdx], ",") {
							dep := cleanPyDep(strings.Trim(part, `"', `))
							if dep != "" {
								add(dep)
							}
						}
					} else {
						inProjectDepsArray = true
					}
					continue
				}
			}
			if inProjectDepsArray {
				if strings.HasPrefix(t, "]") {
					inProjectDepsArray = false
					continue
				}
				dep := cleanPyDep(strings.Trim(t, `"', `))
				if dep != "" && !strings.HasPrefix(dep, "#") {
					add(dep)
				}
				continue
			}
			if inPoetryDeps && strings.Contains(t, "=") && !strings.HasPrefix(t, "#") {
				parts := strings.SplitN(t, "=", 2)
				if name := strings.TrimSpace(parts[0]); name != "python" {
					add(name)
				}
			}
		}
	}

	// Priority: non-npm manifest deps (go.mod, Cargo.toml, requirements.txt, etc.)
	// fill the budget first; npm runtime deps are appended if space remains, dev
	// deps last. This keeps the most-structured manifests dominant.
	sort.Strings(deps)
	if len(deps) > maxDeps {
		deps = deps[:maxDeps]
	}
	sort.Strings(npmRuntime)
	for _, name := range npmRuntime {
		if len(deps) >= maxDeps {
			break
		}
		add(name)
	}
	sort.Strings(npmDev)
	for _, name := range npmDev {
		if len(deps) >= maxDeps {
			break
		}
		add(name)
	}
	return deps
}

func cleanPyDep(dep string) string {
	for _, sep := range []string{";", " #", "[", ">=", "<=", "==", "!=", "~=", ">", "<"} {
		if i := strings.Index(dep, sep); i >= 0 {
			dep = dep[:i]
		}
	}
	return strings.TrimSpace(dep)
}

// collectFiles walks rootDir and returns extension counts, files per directory
// key, and total file count.
func collectFiles(ctx context.Context, rootDir string) (extCounts map[string]int, dirFiles map[string][]string, total int, err error) {
	extCounts = make(map[string]int)
	dirFiles = make(map[string][]string)

	walkErr := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			name := d.Name()
			if path != rootDir && (ignoreDirs[name] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, rerr := filepath.Rel(rootDir, path)
		if rerr != nil {
			return nil
		}
		// Skip hidden files.
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := extToLanguage[ext]; !ok {
			return nil
		}
		extCounts[ext]++
		total++

		parts := strings.SplitN(rel, string(filepath.Separator), 3)
		dir := ""
		if len(parts) > 1 {
			dir = parts[0]
			if deepDirs[dir] && len(parts) > 2 {
				dir = parts[0] + string(filepath.Separator) + parts[1]
			}
		}
		dirFiles[dir] = append(dirFiles[dir], rel)
		return nil
	})
	return extCounts, dirFiles, total, walkErr
}

func detectLanguages(extCounts map[string]int) (primary string, languages []string) {
	langCounts := make(map[string]int)
	for ext, count := range extCounts {
		if lang, ok := extToLanguage[ext]; ok {
			langCounts[lang] += count
		}
	}
	type lc struct {
		lang  string
		count int
	}
	var sorted []lc
	for lang, count := range langCounts {
		sorted = append(sorted, lc{lang, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].lang < sorted[j].lang
	})
	for _, item := range sorted {
		languages = append(languages, item.lang)
	}
	if len(languages) > 0 {
		primary = languages[0]
	}
	if len(languages) > 5 {
		languages = languages[:5]
	}
	return primary, languages
}

func readDescription(rootDir string) string {
	for _, name := range []string{"README.md", "readme.md", "README.rst", "readme.rst", "README.txt"} {
		data, err := os.ReadFile(filepath.Join(rootDir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[![") ||
				strings.HasPrefix(line, "![") || strings.HasPrefix(line, "|") ||
				strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") ||
				isHorizontalRule(line) {
				continue
			}
			if line != "" && len([]rune(line)) < 250 {
				return line
			}
		}
	}
	return ""
}

func isHorizontalRule(line string) bool {
	if len(line) < 3 {
		return false
	}
	var ch rune
	count := 0
	for _, c := range line {
		if c == ' ' {
			continue
		}
		if ch == 0 {
			if c != '-' && c != '*' && c != '_' {
				return false
			}
			ch = c
		} else if c != ch {
			return false
		}
		count++
	}
	return ch != 0 && count >= 3
}

func buildDomains(dirFiles map[string][]string) []Domain {
	const maxKeyFiles = 8
	const maxDomains = 20

	var dirs []string
	for dir := range dirFiles {
		dirs = append(dirs, dir)
	}
	sort.Slice(dirs, func(i, j int) bool {
		ci, cj := len(dirFiles[dirs[i]]), len(dirFiles[dirs[j]])
		if ci != cj {
			return ci > cj
		}
		return dirs[i] < dirs[j]
	})
	if len(dirs) > maxDomains {
		dirs = dirs[:maxDomains]
	}

	var domains []Domain
	for _, dir := range dirs {
		files := dirFiles[dir]
		sort.Slice(files, func(i, j int) bool {
			pi, pj := entryPointPriority(files[i]), entryPointPriority(files[j])
			if pi != pj {
				return pi > pj
			}
			li, lj := len(files[i]), len(files[j])
			if li != lj {
				return li < lj
			}
			return files[i] < files[j]
		})
		keyFiles := files
		if len(keyFiles) > maxKeyFiles {
			keyFiles = keyFiles[:maxKeyFiles]
		}
		name := dir
		if name == "" {
			name = "Root"
		}
		domains = append(domains, Domain{
			Name:        name,
			Description: fmt.Sprintf("%d file(s)", len(files)),
			KeyFiles:    keyFiles,
		})
	}
	return domains
}

// localTopFiles picks the top files across all domains by entry-point priority.
// In local mode RelationshipCount is always 0 (no cross-domain data available).
func localTopFiles(domains []Domain, n int) []CriticalFile {
	seen := make(map[string]struct{})
	var files []CriticalFile
	for i := range domains {
		for _, f := range domains[i].KeyFiles {
			if _, ok := seen[f]; ok {
				continue
			}
			seen[f] = struct{}{}
			files = append(files, CriticalFile{Path: f})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		pi, pj := entryPointPriority(files[i].Path), entryPointPriority(files[j].Path)
		if pi != pj {
			return pi > pj
		}
		li, lj := len(files[i].Path), len(files[j].Path)
		if li != lj {
			return li < lj
		}
		return files[i].Path < files[j].Path
	})
	if len(files) > n {
		files = files[:n]
	}
	return files
}

func entryPointPriority(path string) int {
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	switch strings.ToLower(name) {
	case "main":
		return 4
	case "app", "application":
		return 3
	case "server", "index":
		return 2
	case "init", "__init__":
		return 1
	}
	return 0
}
