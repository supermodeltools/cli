package shards

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
	"docs-output": true,
	".terraform":  true,
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

// zipExclusions holds the merged skip lists for a single zip operation.
// It is built fresh for every CreateZipFile / DryRunList call so the global
// maps are never mutated (eliminates concurrent-write races).
type zipExclusions struct {
	skipDirs map[string]bool
	skipExts map[string]bool
	patterns []string // glob patterns matched against the slash-relative path
}

// buildExclusions merges the base skip lists with any custom entries read
// from .supermodel.json in repoDir and returns a per-call copy.
//
// .supermodel.json format:
//
//	{
//	  "exclude_dirs": ["generated", "fixtures"],
//	  "exclude_exts": [".pb.go", ".generated.ts"],
//	  "exclude_patterns": ["**/testdata/**", "docs/api/**"]
//	}
func buildExclusions(repoDir string) *zipExclusions {
	ex := &zipExclusions{
		skipDirs: make(map[string]bool, len(zipSkipDirs)+4),
		skipExts: make(map[string]bool, len(zipSkipExtensions)+4),
	}
	for k, v := range zipSkipDirs {
		ex.skipDirs[k] = v
	}
	for k, v := range zipSkipExtensions {
		ex.skipExts[k] = v
	}

	cfgPath := filepath.Join(repoDir, ".supermodel.json")
	if abs, err := filepath.Abs(cfgPath); err == nil {
		cfgPath = abs
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return ex
	}
	var cfg struct {
		ExcludeDirs     []string `json:"exclude_dirs"`
		ExcludeExts     []string `json:"exclude_exts"`
		ExcludePatterns []string `json:"exclude_patterns"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v — custom exclusions will be ignored\n",
			cfgPath, err)
		return ex
	}
	for _, d := range cfg.ExcludeDirs {
		ex.skipDirs[d] = true
	}
	for _, e := range cfg.ExcludeExts {
		ex.skipExts[e] = true
	}
	ex.patterns = cfg.ExcludePatterns
	return ex
}

// matchesPattern reports whether the slash-relative path matches any of the
// user-supplied glob patterns. Supports ** as a multi-segment wildcard.
func matchesPattern(relSlash string, patterns []string) bool {
	for _, pat := range patterns {
		pat = filepath.ToSlash(pat)
		// Try direct match
		if ok, _ := matchGlob(pat, relSlash); ok {
			return true
		}
		// Also match against just the filename component
		if ok, _ := matchGlob(pat, filepath.Base(relSlash)); ok {
			return true
		}
	}
	return false
}

// matchGlob extends filepath.Match with ** support.
func matchGlob(pattern, name string) (bool, error) {
	if !strings.Contains(pattern, "**") {
		return filepath.Match(pattern, name)
	}
	// Replace ** with a sentinel, split on /, handle each segment
	// Simple approach: match the suffix/prefix around **
	parts := strings.SplitN(pattern, "**", 2)
	prefix, suffix := parts[0], parts[1]
	suffix = strings.TrimPrefix(suffix, "/")
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix != "" && !strings.HasPrefix(name, prefix+"/") && name != prefix {
		return false, nil
	}
	if suffix == "" {
		return true, nil
	}
	return filepath.Match(suffix, filepath.Base(name))
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

func shouldInclude(relPath string, fileSize int64, ex *zipExclusions) bool {
	slashRel := filepath.ToSlash(relPath)
	parts := strings.Split(slashRel, "/")

	// User-defined glob patterns take precedence
	if len(ex.patterns) > 0 && matchesPattern(slashRel, ex.patterns) {
		return false
	}

	for _, part := range parts[:len(parts)-1] {
		if ex.skipDirs[part] || hardBlocked[part] {
			return false
		}
		if strings.HasPrefix(part, ".") {
			return false
		}
	}

	filename := parts[len(parts)-1]

	if isShardFile(filename) {
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
	if ex.skipExts[ext] {
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
	ex := buildExclusions(repoDir)

	f, err := os.CreateTemp("", "supermodel-shards-*.zip")
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
			if !shouldInclude(rel, info.Size(), ex) {
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
				if ex.skipDirs[name] || hardBlocked[name] || strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
				return nil
			}

			if !shouldInclude(rel, info.Size(), ex) {
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
	ex := buildExclusions(repoDir)
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
			if ex.skipDirs[name] || hardBlocked[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if shouldInclude(rel, info.Size(), ex) {
			files = append(files, rel)
		}
		return nil
	})

	return files, err
}

// LangStat holds a file count for a single file extension.
type LangStat struct {
	Ext   string
	Count int
}

// LanguageStats counts files by extension (without leading dot), sorted by
// count descending. Only non-empty extensions are included; at most 10 are returned.
func LanguageStats(files []string) []LangStat {
	counts := make(map[string]int)
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if ext == "" {
			continue
		}
		counts[strings.TrimPrefix(ext, ".")]++
	}
	stats := make([]LangStat, 0, len(counts))
	for ext, n := range counts {
		stats = append(stats, LangStat{Ext: ext, Count: n})
	}
	// sort descending by count, then alphabetically for ties
	for i := 1; i < len(stats); i++ {
		for j := i; j > 0; j-- {
			a, b := stats[j-1], stats[j]
			if a.Count < b.Count || (a.Count == b.Count && a.Ext > b.Ext) {
				stats[j-1], stats[j] = b, a
			}
		}
	}
	if len(stats) > 10 {
		stats = stats[:10]
	}
	return stats
}

// PrintLanguageBarChart writes a compact bar chart of language stats to stderr.
func PrintLanguageBarChart(stats []LangStat, totalFiles int) {
	if len(stats) == 0 {
		return
	}
	maxCount := stats[0].Count
	const maxBar = 28
	fmt.Fprintf(os.Stderr, "\n  %d files to upload\n\n", totalFiles)
	for _, s := range stats {
		barLen := maxBar * s.Count / maxCount
		if barLen < 1 {
			barLen = 1
		}
		bar := strings.Repeat("█", barLen)
		fmt.Fprintf(os.Stderr, "  %-6s %s %d\n", s.Ext, bar, s.Count)
	}
	fmt.Fprintln(os.Stderr)
}

// shardTags are the extension tags used by all shard formats.
var shardTags = map[string]bool{
	"graph":  true, // single-file format
	"calls":  true, // three-file format
	"deps":   true,
	"impact": true,
}

// isShardFile checks if a filename is a generated shard (e.g. foo.graph.ts, foo.calls.ts).
func isShardFile(filename string) bool {
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
	return shardTags[tag]
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
