package analyze

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// skipDirs are directory names that should never be included in the archive.
var skipDirs = map[string]bool{
	".git":             true,
	".claude":          true,
	".idea":            true,
	".vscode":          true,
	".cache":           true,
	".turbo":           true,
	".nx":              true,
	".next":            true,
	".nuxt":            true,
	".terraform":       true,
	".tox":             true,
	".venv":            true,
	".pnpm-store":      true,
	"__pycache__":      true,
	"__snapshots__":    true,
	"bower_components": true,
	"build":            true,
	"coverage":         true,
	"dist":             true,
	"node_modules":     true,
	"out":              true,
	"target":           true,
	"vendor":           true,
	"venv":             true,
}

// createZip archives the repository at dir into a temporary ZIP file and
// returns its path. The caller is responsible for removing the file.
//
// Strategy: use git archive when inside a Git repo (respects .gitignore,
// deterministic output). Falls back to a manual directory walk otherwise.
func createZip(dir string) (string, error) {
	f, err := os.CreateTemp("", "supermodel-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	dest := f.Name()
	f.Close()

	if isGitRepo(dir) && isWorktreeClean(dir) {
		if err := gitArchive(dir, dest); err == nil {
			return dest, nil
		}
	}

	if err := walkZip(dir, dest); err != nil {
		os.Remove(dest)
		return "", err
	}
	return dest, nil
}

func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}
// isWorktreeClean reports whether there are no uncommitted changes.
// When the worktree is dirty, git archive HEAD would silently omit local
// edits, so we fall back to the directory walk instead.
func isWorktreeClean(dir string) bool {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	return err == nil && strings.TrimSpace(string(out)) == ""
}

func gitArchive(dir, dest string) error {
	cmd := exec.Command("git", "-C", dir, "archive", "--format=zip", "-o", dest, "HEAD")
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// walkZip creates a ZIP of dir, excluding skipDirs, hidden files, and
// files larger than 10 MB.
func walkZip(dir, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") || info.Size() > 10<<20 {
			return nil
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
}
