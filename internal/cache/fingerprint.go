package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
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

	dirty, err := gitOutput(dir, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return commitSHA, nil
	}

	if dirty == "" {
		return commitSHA, nil
	}

	// Dirty: hash the diff to capture uncommitted changes.
	diff, err := gitOutput(dir, "diff", "HEAD")
	if err != nil {
		return commitSHA + ":dirty", nil
	}
	h := sha256.Sum256([]byte(diff))
	return commitSHA + ":" + hex.EncodeToString(h[:8]), nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// AnalysisKey builds a cache key for a specific analysis type on a repo state.
func AnalysisKey(fingerprint, analysisType string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s", fingerprint, analysisType)
	return hex.EncodeToString(h.Sum(nil))
}
