package files

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Security-critical: files/dirs that must never be zipped.
var hardBlocked = map[string]bool{
	".aws": true, ".ssh": true, ".gnupg": true, ".terraform": true,
}

var hardBlockedPatterns = []string{
	".env", "*.key", "*.pem", "*.p12", "*.pfx", "*.crt", "*.cert",
	"*.tfstate", "*.tfstate.backup", "*secret*", "*credential*", "*password*",
}

var zipSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".venv": true,
	"venv": true, "__pycache__": true, "dist": true, "build": true,
	".next": true, ".nuxt": true, ".cache": true, ".turbo": true,
	"coverage": true, ".nyc_output": true, "__snapshots__": true,
	".terraform": true,
}

var zipSkipFiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"bun.lockb": true, "Gemfile.lock": true, "poetry.lock": true,
	"go.sum": true, "Cargo.lock": true,
}

var zipSkipExtensions = map[string]bool{
	".min.js": true, ".min.css": true, ".bundle.js": true, ".map": true,
	".ico": true, ".woff": true, ".woff2": true, ".ttf": true,
	".eot": true, ".otf": true, ".mp4": true, ".mp3": true,
	".wav": true, ".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".webp": true,
}

const maxFileSize = 500 * 1024 // 500KB

// loadCustomExclusions reads .supermodel.json from repoDir and adds any
// custom exclude_dirs and exclude_exts to the skip lists.
func loadCustomExclusions(repoDir string) {
	cfgPath := filepath.Join(repoDir, ".supermodel.json")
	if abs, err := filepath.Abs(cfgPath); err == nil {
		cfgPath = abs
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return
	}
	var cfg struct {
		ExcludeDirs []string `json:"exclude_dirs"`
		ExcludeExts []string `json:"exclude_exts"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v — custom exclusions will be ignored\n",
			cfgPath, err)
		return
	}
	for _, d := range cfg.ExcludeDirs {
		zipSkipDirs[d] = true
	}
	for _, e := range cfg.ExcludeExts {
		zipSkipExtensions[e] = true
	}
}

// matchPattern does simple glob matching (*, ?).
func matchPattern(pattern, name string) bool {
	pattern = strings.ToLower(pattern)
	name = strings.ToLower(name)

	if !strings.ContainsAny(pattern, "*?") {
		return strings.Contains(name, pattern)
	}

	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return name == pattern
	}

	if parts[0] != "" && !strings.HasPrefix(name, parts[0]) {
		return false
	}
	last := parts[len(parts)-1]
	if last != "" && !strings.HasSuffix(name, last) {
		return false
	}
	remaining := name
	for _, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(remaining, part)
		if idx < 0 {
			return false
		}
		remaining = remaining[idx+len(part):]
	}
	return true
}

func shouldInclude(relPath string, fileSize int64) bool {
	parts := strings.Split(filepath.ToSlash(relPath), "/")

	for _, part := range parts[:len(parts)-1] {
		if zipSkipDirs[part] || hardBlocked[part] {
			return false
		}
		if strings.HasPrefix(part, ".") {
			return false
		}
	}

	filename := parts[len(parts)-1]

	if isSidecarFile(filename) {
		return false
	}

	for _, pat := range hardBlockedPatterns {
		if matchPattern(pat, filename) {
			return false
		}
	}

	if zipSkipFiles[filename] {
		return false
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if zipSkipExtensions[ext] {
		return false
	}

	if strings.HasSuffix(filename, ".min.js") || strings.HasSuffix(filename, ".min.css") {
		return false
	}

	if fileSize > maxFileSize {
		return false
	}

	return true
}

// CreateZipFile creates a zip archive of the repo directory, respecting filters,
// writes it to a temporary file, and returns the path. The caller is responsible
// for removing the file.
// If onlyFiles is non-nil, only those relative paths are included (incremental mode).
func CreateZipFile(repoDir string, onlyFiles []string) (string, error) {
	loadCustomExclusions(repoDir)

	f, err := os.CreateTemp("", "supermodel-sidecars-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp zip: %w", err)
	}
	dest := f.Name()

	zw := zip.NewWriter(f)

	if onlyFiles != nil {
		for _, rel := range onlyFiles {
			full := filepath.Join(repoDir, rel)
			info, err := os.Lstat(full)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if !shouldInclude(rel, info.Size()) {
				continue
			}
			if err := addFileToZip(zw, full, rel); err != nil {
				zw.Close()
				f.Close()
				os.Remove(dest)
				return "", err
			}
		}
	} else {
		err = filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			rel, _ := filepath.Rel(repoDir, path)
			if rel == "." {
				return nil
			}

			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}

			if info.IsDir() {
				name := info.Name()
				if zipSkipDirs[name] || hardBlocked[name] || strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
				return nil
			}

			if !shouldInclude(rel, info.Size()) {
				return nil
			}

			return addFileToZip(zw, path, rel)
		})
		if err != nil {
			zw.Close()
			f.Close()
			os.Remove(dest)
			return "", err
		}
	}

	if err := zw.Close(); err != nil {
		f.Close()
		os.Remove(dest)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(dest)
		return "", err
	}
	return dest, nil
}

// DryRunList returns the list of files that would be included in the zip.
func DryRunList(repoDir string) ([]string, error) {
	loadCustomExclusions(repoDir)
	var files []string

	err := filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(repoDir, path)
		if rel == "." {
			return nil
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			if zipSkipDirs[name] || hardBlocked[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if shouldInclude(rel, info.Size()) {
			files = append(files, rel)
		}
		return nil
	})

	return files, err
}

// isSidecarFile checks if a filename is a generated sidecar (e.g. foo.graph.ts).
func isSidecarFile(filename string) bool {
	ext := filepath.Ext(filename)
	if ext == "" {
		return false
	}
	stem := strings.TrimSuffix(filename, ext)
	tag := filepath.Ext(stem)
	if tag == "" {
		return false
	}
	tag = strings.TrimPrefix(tag, ".")
	return tag == SidecarExt
}

func addFileToZip(w *zip.Writer, fullPath, relPath string) error {
	f, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zw, err := w.Create(filepath.ToSlash(relPath))
	if err != nil {
		return err
	}

	_, err = io.Copy(zw, f)
	return err
}
