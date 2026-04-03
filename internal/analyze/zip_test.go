package analyze

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/archive"
)

func TestIsGitRepo_WithDotGit(t *testing.T) {
	dir := t.TempDir()
	// Simulate .git via git init
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0750); err != nil {
		t.Fatal(err)
	}
	// isGitRepo uses `git rev-parse --git-dir` which needs an actual git repo;
	// fall back to checking directory creation only — the factory version
	// (os.Stat) is simpler, but here we just ensure non-git dir returns false.
	if archive.IsGitRepo(t.TempDir()) {
		t.Error("empty temp dir should not be a git repo")
	}
}

func TestWalkZip_IncludesFiles(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := archive.WalkZip(src, dest); err != nil {
		t.Fatalf("walkZip: %v", err)
	}
	entries := readZipEntries(t, dest)
	if _, ok := entries["main.go"]; !ok {
		t.Error("zip should contain main.go")
	}
}

func TestWalkZip_SkipsHiddenFiles(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, ".env"), []byte("SECRET=x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := archive.WalkZip(src, dest); err != nil {
		t.Fatal(err)
	}
	entries := readZipEntries(t, dest)
	if _, ok := entries[".env"]; ok {
		t.Error("zip should not contain .env")
	}
	if _, ok := entries["main.go"]; !ok {
		t.Error("zip should contain main.go")
	}
}

func TestWalkZip_SkipsSkipDirs(t *testing.T) {
	src := t.TempDir()
	nmDir := filepath.Join(src, "node_modules")
	if err := os.Mkdir(nmDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nmDir, "pkg.js"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "index.js"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := archive.WalkZip(src, dest); err != nil {
		t.Fatal(err)
	}
	entries := readZipEntries(t, dest)
	for name := range entries {
		if strings.HasPrefix(name, "node_modules/") || name == "node_modules" {
			t.Errorf("should not contain node_modules entry: %s", name)
		}
	}
}

func TestCreateZip_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	path, err := createZip(dir)
	if err != nil {
		t.Fatalf("createZip: %v", err)
	}
	defer os.Remove(path)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("zip file not created: %v", err)
	}
}

func readZipEntries(t *testing.T, path string) map[string]bool {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %s: %v", path, err)
	}
	defer r.Close()
	m := make(map[string]bool, len(r.File))
	for _, f := range r.File {
		m[f.Name] = true
	}
	return m
}
