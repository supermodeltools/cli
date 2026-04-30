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
	commitSHA, err := gitOutput(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("not a git repo: %w", err)
	}

	dirty, err := gitOutput(dir, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return commitSHA, nil
	}
	dirty = filterFingerprintStatus(dirty)

	if dirty == "" {
		return commitSHA, nil
	}

	// Dirty: hash tracked changes plus untracked file contents. Generated
	// files are filtered because they are not uploaded for analysis.
	h := sha256.New()
	fmt.Fprintf(h, "status\x00%s\x00", dirty)
	if err := hashDirtyFiles(h, dir, dirty); err != nil {
		return commitSHA + ":dirty", nil
	}
	if err := hashUntrackedFiles(h, dir); err != nil {
		return commitSHA + ":dirty", nil
	}
	sum := h.Sum(nil)
	return commitSHA + ":" + hex.EncodeToString(sum[:8]), nil
}

func hashDirtyFiles(h io.Writer, dir, status string) error {
	for _, line := range strings.Split(status, "\n") {
		if line == "" || strings.HasPrefix(line, "?? ") {
			continue
		}
		rel := statusPath(line)
		if rel == "" || ignoreFingerprintPath(rel) {
			continue
		}
		full := filepath.Join(dir, rel)
		info, err := os.Lstat(full)
		if os.IsNotExist(err) {
			fmt.Fprintf(h, "tracked\x00%s\x00deleted\x00", rel)
			continue
		}
		if err != nil {
			return err
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		fmt.Fprintf(h, "tracked\x00%s\x00%d\x00", rel, info.Size())
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
	}
	return nil
}

func hashUntrackedFiles(h io.Writer, dir string) error {
	out, err := gitOutput(dir, "ls-files", "--others", "--exclude-standard")
	if err != nil || out == "" {
		return err
	}
	files := strings.Split(out, "\n")
	sort.Strings(files)
	for _, rel := range files {
		if rel == "" {
			continue
		}
		if ignoreFingerprintPath(rel) {
			continue
		}
		full := filepath.Join(dir, rel)
		info, err := os.Lstat(full)
		if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		fmt.Fprintf(h, "untracked\x00%s\x00%d\x00", rel, info.Size())
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
	}
	return nil
}

func filterFingerprintStatus(status string) string {
	var kept []string
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		path := statusPath(line)
		if path == "" || ignoreFingerprintPath(path) {
			continue
		}
		kept = append(kept, line)
	}
	sort.Strings(kept)
	return strings.Join(kept, "\n")
}

func statusPath(line string) string {
	if len(line) < 4 {
		return ""
	}
	path := strings.TrimSpace(line[3:])
	if _, after, ok := strings.Cut(path, " -> "); ok {
		path = after
	}
	return filepath.ToSlash(path)
}

func ignoreFingerprintPath(path string) bool {
	path = filepath.ToSlash(path)
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return false
	}
	switch parts[0] {
	case ".supermodel", "docs-output", "node_modules", "vendor", "dist", "build", "coverage":
		return true
	}
	return isGeneratedShardPath(path)
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

// gitOutput runs a git command in dir and returns its trimmed stdout.
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// AnalysisKey builds a cache key for a specific analysis type on a repo state.
// version is the CLI version string and is included so the cache is invalidated
// automatically after an upgrade.
func AnalysisKey(fingerprint, analysisType, version string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s", fingerprint, analysisType, version)
	return hex.EncodeToString(h.Sum(nil))
}
