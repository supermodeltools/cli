package archdocs

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
// In both cases, skipDirs entries are excluded.
func createZip(dir string) (string, error) {
	f, err := os.CreateTemp("", "supermodel-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	dest := f.Name()
	f.Close()

	if isGitRepo(dir) {
		if err := gitArchive(dir, dest); err == nil {
			if err := filterSkipDirs(dest); err != nil {
				os.Remove(dest)
				return "", fmt.Errorf("filter archive: %w", err)
			}
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

// filterSkipDirs removes entries from a ZIP whose path contains a skipDirs segment.
func filterSkipDirs(zipPath string) error {
	data, err := os.ReadFile(zipPath)
	if err != nil {
		return err
	}

	r, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		return err
	}

	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	for _, f := range r.File {
		if shouldSkip(f.Name) {
			continue
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		_, err = io.Copy(w, rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return zw.Close()
}

// shouldSkip reports whether a zip entry path contains a skipDirs segment.
func shouldSkip(path string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(path), "/") {
		if skipDirs[seg] {
			return true
		}
	}
	return false
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
