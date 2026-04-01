package mcp

// createZip is a thin shim so the mcp package can archive the repo
// without importing the analyze slice. It delegates to git archive
// and falls back to a directory walk — identical logic to analyze/zip.go
// but duplicated to preserve vertical slice isolation.

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".next":        true,
	".terraform":   true,
}

func createZip(dir string) (string, error) {
	f, err := os.CreateTemp("", "supermodel-mcp-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	dest := f.Name()
	f.Close()

	if isGitRepo(dir) {
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

func gitArchive(dir, dest string) error {
	cmd := exec.Command("git", "-C", dir, "archive", "--format=zip", "-o", dest, "HEAD")
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

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
