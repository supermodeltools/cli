package factory

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/supermodeltools/cli/internal/archive"
)

// CreateZip archives the repository at dir into a temporary ZIP file and
// returns its path. The caller is responsible for removing the file.
//
// Uses git archive only when the worktree is clean (so the archive reflects
// what the user is actually looking at). Falls back to directory walk otherwise.
func CreateZip(dir string) (string, error) {
	f, err := os.CreateTemp("", "supermodel-factory-*.zip")
	if err != nil {
		return "", err
	}
	dest := f.Name()
	f.Close()

	if isGitRepo(dir) && isWorktreeClean(dir) {
		if err := archive.GitArchive(dir, dest); err == nil {
			if err := archive.FilterSkipDirs(dest); err != nil {
				_ = os.Remove(dest)
				return "", err
			}
			return dest, nil
		}
	}

	if err := archive.WalkZip(dir, dest); err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	return dest, nil
}

func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func isWorktreeClean(dir string) bool {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output() //nolint:gosec // dir is user-supplied cwd
	return err == nil && strings.TrimSpace(string(out)) == ""
}

func copyFile(path string, w io.Writer) error {
	f, err := os.Open(path) //nolint:gosec // path is from filepath.Walk; symlinks excluded by caller
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	f.Close()
	return err
}
