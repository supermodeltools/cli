package factory

import (
	"archive/zip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// skipDirs are directory names that should never be included in the archive.
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
	".nuxt":        true,
	"coverage":     true,
	".terraform":   true,
	".tox":         true,
}

// CreateZip archives the repository at dir into a temporary ZIP file and
// returns its path. The caller is responsible for removing the file.
//
// Strategy: use git archive when the repo is clean (committed state matches
// working tree, so the archive reflects what the user is actually looking at).
// Falls back to a manual directory walk otherwise.
func CreateZip(dir string) (string, error) {
	f, err := os.CreateTemp("", "supermodel-factory-*.zip")
	if err != nil {
		return "", err
	}
	dest := f.Name()
	f.Close()

	if isGitRepo(dir) && isWorktreeClean(dir) {
		if err := gitArchive(dir, dest); err == nil {
			return dest, nil
		}
	}

	if err := walkZip(dir, dest); err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	return dest, nil
}

func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// isWorktreeClean reports whether there are no uncommitted changes. When the
// worktree is dirty, git archive HEAD would silently omit local edits, so we
// fall back to the directory walk instead.
func isWorktreeClean(dir string) bool {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output() //nolint:gosec // dir is user-supplied cwd
	return err == nil && strings.TrimSpace(string(out)) == ""
}

func gitArchive(dir, dest string) error {
	cmd := exec.Command("git", "-C", dir, "archive", "--format=zip", "-o", dest, "HEAD") //nolint:gosec // dir is user-supplied cwd; dest is temp file
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// walkZip creates a ZIP of dir, excluding skipDirs, hidden files, symlinks,
// and files larger than 10 MB.
func walkZip(dir, dest string) error {
	out, err := os.Create(dest) //nolint:gosec // dest is a temp file path from os.CreateTemp
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
		// Skip symlinks: os.Open follows them, which could read files outside dir.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
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
		return copyFile(path, w)
	})
}

func copyFile(path string, w io.Writer) error {
	f, err := os.Open(path) //nolint:gosec // path is from filepath.Walk within dir; symlinks already excluded above
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	f.Close()
	return err
}
