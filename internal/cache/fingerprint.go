package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// RepoFingerprint returns a fast, content-based cache key for the repo at dir.
//
// For clean git repos (~1ms): returns the commit SHA.
// For dirty git repos (~100ms): returns commitSHA:dirtyHash.
// For non-git dirs: returns empty string and an error.
func RepoFingerprint(dir string) (string, error) {
	commitSHA, err := gitOutputTrim(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("not a git repo: %w", err)
	}

	statusOut, err := gitOutputRaw(dir, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return commitSHA, nil
	}
	dirtyEntries := filterFingerprintStatus(parseGitStatusZ(statusOut))

	if len(dirtyEntries) == 0 {
		return commitSHA, nil
	}

	// Dirty: hash tracked changes plus untracked file contents. Generated
	// files are filtered because they are not uploaded for analysis.
	h := sha256.New()
	fmt.Fprint(h, "status\x00")
	writeStatusEntries(h, dirtyEntries)
	if err := hashDirtyFiles(h, dir, dirtyEntries); err != nil {
		return commitSHA + ":dirty", nil
	}
	if err := hashUntrackedFiles(h, dir); err != nil {
		return commitSHA + ":dirty", nil
	}
	sum := h.Sum(nil)
	return commitSHA + ":" + hex.EncodeToString(sum[:8]), nil
}

type gitStatusEntry struct {
	code  string
	paths []string
}

func parseGitStatusZ(status string) []gitStatusEntry {
	if status == "" {
		return nil
	}
	records := strings.Split(status, "\x00")
	entries := make([]gitStatusEntry, 0, len(records))
	for i := 0; i < len(records); i++ {
		record := records[i]
		if record == "" || len(record) < 4 {
			continue
		}
		entry := gitStatusEntry{
			code:  record[:2],
			paths: []string{filepath.ToSlash(record[3:])},
		}
		if isRenameOrCopyStatus(entry.code) && i+1 < len(records) && records[i+1] != "" {
			entry.paths = append(entry.paths, filepath.ToSlash(records[i+1]))
			i++
		}
		entries = append(entries, entry)
	}
	return entries
}

func filterFingerprintStatus(entries []gitStatusEntry) []gitStatusEntry {
	kept := make([]gitStatusEntry, 0, len(entries))
	for _, entry := range entries {
		if !statusEntryTouchesUploadablePath(entry) {
			continue
		}
		kept = append(kept, entry)
	}
	sort.Slice(kept, func(i, j int) bool {
		return statusEntryKey(kept[i]) < statusEntryKey(kept[j])
	})
	return kept
}

func statusEntryTouchesUploadablePath(entry gitStatusEntry) bool {
	for _, path := range entry.paths {
		if path != "" && !ignoreFingerprintPath(path) {
			return true
		}
	}
	return false
}

func statusEntryKey(entry gitStatusEntry) string {
	return entry.code + "\x00" + strings.Join(entry.paths, "\x00")
}

func writeStatusEntries(h io.Writer, entries []gitStatusEntry) {
	for _, entry := range entries {
		fmt.Fprintf(h, "%s\x00", entry.code)
		for _, path := range entry.paths {
			fmt.Fprintf(h, "%s\x00", path)
		}
	}
}

func isRenameOrCopyStatus(code string) bool {
	return strings.ContainsAny(code, "RC")
}

func isRenameStatus(code string) bool {
	return strings.Contains(code, "R")
}

func hashDirtyFiles(h io.Writer, dir string, entries []gitStatusEntry) error {
	for _, entry := range entries {
		if entry.code == "??" || len(entry.paths) == 0 {
			continue
		}

		if isRenameOrCopyStatus(entry.code) && len(entry.paths) > 1 {
			newPath := entry.paths[0]
			oldPath := entry.paths[1]
			if isRenameStatus(entry.code) && !ignoreFingerprintPath(oldPath) && oldPath != newPath {
				fmt.Fprintf(h, "tracked\x00%s\x00deleted\x00", oldPath)
			}
			if !ignoreFingerprintPath(newPath) {
				if err := hashFileState(h, dir, "tracked", newPath, true); err != nil {
					return err
				}
			}
			continue
		}

		rel := entry.paths[0]
		if rel == "" || ignoreFingerprintPath(rel) {
			continue
		}
		if err := hashFileState(h, dir, "tracked", rel, true); err != nil {
			return err
		}
	}
	return nil
}

func hashUntrackedFiles(h io.Writer, dir string) error {
	out, err := gitOutputRaw(dir, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil || out == "" {
		return err
	}
	files := splitNUL(out)
	sort.Strings(files)
	for _, rel := range files {
		if rel == "" {
			continue
		}
		rel = filepath.ToSlash(rel)
		if ignoreFingerprintPath(rel) {
			continue
		}
		if err := hashFileState(h, dir, "untracked", rel, false); err != nil {
			return err
		}
	}
	return nil
}

func splitNUL(out string) []string {
	if out == "" {
		return nil
	}
	fields := strings.Split(out, "\x00")
	if len(fields) > 0 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}
	return fields
}

func hashFileState(h io.Writer, dir, kind, rel string, missingAsDeleted bool) error {
	full := filepath.Join(dir, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		if missingAsDeleted {
			fmt.Fprintf(h, "%s\x00%s\x00deleted\x00", kind, rel)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	fmt.Fprintf(h, "%s\x00%s\x00%d\x00", kind, rel, info.Size())
	f, err := os.Open(full)
	if err != nil {
		return err
	}
	if _, err := io.Copy(h, f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprint(h, "\x00")
	return nil
}

func ignoreFingerprintPath(path string) bool {
	path = filepath.ToSlash(path)
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts[:len(parts)-1] {
		if strings.HasPrefix(part, ".") || defaultUploadSkipDir(part) {
			return true
		}
	}
	filename := parts[len(parts)-1]
	return isGeneratedShardPath(path) ||
		isSensitiveFingerprintPath(path) ||
		defaultUploadSkipFile(filename) ||
		defaultUploadSkipExtension(filename)
}

func defaultUploadSkipDir(name string) bool {
	// Keep this in lockstep with internal/shards/zip.go upload exclusions.
	// The cache package cannot import shards because shards already imports cache.
	switch name {
	case ".git", "node_modules", "vendor", ".venv", "venv", "__pycache__", "dist", "build",
		".next", ".nuxt", ".cache", ".turbo", "coverage", ".nyc_output", "__snapshots__",
		"docs-output", ".terraform":
		return true
	default:
		return false
	}
}

func defaultUploadSkipFile(name string) bool {
	// Keep this in lockstep with internal/shards/zip.go upload exclusions.
	switch name {
	case "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "bun.lockb", "Gemfile.lock",
		"poetry.lock", "go.sum", "Cargo.lock":
		return true
	default:
		return false
	}
}

func defaultUploadSkipExtension(name string) bool {
	// Keep this in lockstep with internal/shards/zip.go upload exclusions.
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".map", ".ico", ".woff", ".woff2", ".ttf", ".eot", ".otf", ".mp4", ".mp3",
		".wav", ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return strings.HasSuffix(name, ".min.js") ||
			strings.HasSuffix(name, ".min.css") ||
			strings.HasSuffix(name, ".bundle.js")
	}
}

func isSensitiveFingerprintPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if base == ".env" {
		return true
	}
	for _, suffix := range []string{".key", ".pem", ".p12", ".pfx", ".crt", ".cert", ".tfstate", ".tfstate.backup"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return strings.Contains(base, "secret") ||
		strings.Contains(base, "credential") ||
		strings.Contains(base, "password")
}

func isGeneratedShardPath(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext == "" {
		return false
	}
	stem := strings.TrimSuffix(base, ext)
	tag := strings.TrimPrefix(filepath.Ext(stem), ".")
	switch tag {
	case "graph", "calls", "deps", "impact":
		return true
	default:
		return false
	}
}

func gitOutputRaw(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// gitOutputTrim runs a git command in dir and returns stdout without trailing whitespace.
func gitOutputTrim(dir string, args ...string) (string, error) {
	out, err := gitOutputRaw(dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// AnalysisKey builds a cache key for a specific analysis type on a repo state.
// version is the CLI version string and is included so the cache is invalidated
// automatically after an upgrade.
func AnalysisKey(fingerprint, analysisType, version string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s", fingerprint, analysisType, version)
	return hex.EncodeToString(h.Sum(nil))
}
