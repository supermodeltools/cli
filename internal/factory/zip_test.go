package factory

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/archive"
)

// ── isGitRepo ─────────────────────────────────────────────────────────────────

func TestIsGitRepo_WithDotGit(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0750); err != nil {
		t.Fatal(err)
	}
	if !isGitRepo(dir) {
		t.Error("dir with .git should be detected as git repo")
	}
}

func TestIsGitRepo_WithoutDotGit(t *testing.T) {
	dir := t.TempDir()
	if isGitRepo(dir) {
		t.Error("dir without .git should not be detected as git repo")
	}
}

// ── walkZip ───────────────────────────────────────────────────────────────────

func TestWalkZip_IncludesRegularFiles(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "main.go"), "package main")
	writeFile(t, filepath.Join(src, "README.md"), "# readme")

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := archive.WalkZip(src, dest); err != nil {
		t.Fatalf("walkZip: %v", err)
	}

	entries := zipEntries(t, dest)
	if _, ok := entries["main.go"]; !ok {
		t.Error("zip should contain main.go")
	}
	if _, ok := entries["README.md"]; !ok {
		t.Error("zip should contain README.md")
	}
}

func TestWalkZip_SkipsHiddenFiles(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, ".env"), "SECRET=123")
	writeFile(t, filepath.Join(src, "main.go"), "package main")

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := archive.WalkZip(src, dest); err != nil {
		t.Fatalf("walkZip: %v", err)
	}

	entries := zipEntries(t, dest)
	if _, ok := entries[".env"]; ok {
		t.Error("zip should not contain hidden file .env")
	}
	if _, ok := entries["main.go"]; !ok {
		t.Error("zip should contain main.go")
	}
}

func TestWalkZip_SkipsNodeModules(t *testing.T) {
	src := t.TempDir()
	nmDir := filepath.Join(src, "node_modules")
	if err := os.Mkdir(nmDir, 0750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(nmDir, "pkg.js"), "module.exports = {}")
	writeFile(t, filepath.Join(src, "index.js"), "console.log('hi')")

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := archive.WalkZip(src, dest); err != nil {
		t.Fatalf("walkZip: %v", err)
	}

	entries := zipEntries(t, dest)
	for name := range entries {
		if strings.HasPrefix(name, "node_modules/") || name == "node_modules" {
			t.Errorf("zip should not contain node_modules entry: %s", name)
		}
	}
	if _, ok := entries["index.js"]; !ok {
		t.Error("zip should contain index.js")
	}
}

func TestWalkZip_SkipsAllSkipDirs(t *testing.T) {
	src := t.TempDir()
	for dir := range archive.SkipDirs {
		d := filepath.Join(src, dir)
		if err := os.Mkdir(d, 0750); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(d, "file.go"), "package x")
	}
	writeFile(t, filepath.Join(src, "real.go"), "package main")

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := archive.WalkZip(src, dest); err != nil {
		t.Fatalf("walkZip: %v", err)
	}

	entries := zipEntries(t, dest)
	if len(entries) != 1 {
		t.Errorf("should only contain 1 file (real.go), got %d: %v", len(entries), entries)
	}
	if _, ok := entries["real.go"]; !ok {
		t.Error("zip should contain real.go")
	}
}

func TestWalkZip_EmptyDir(t *testing.T) {
	src := t.TempDir()
	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := archive.WalkZip(src, dest); err != nil {
		t.Fatalf("walkZip on empty dir: %v", err)
	}
	entries := zipEntries(t, dest)
	if len(entries) != 0 {
		t.Errorf("empty dir: want 0 entries, got %d", len(entries))
	}
}

func TestWalkZip_NestedFiles(t *testing.T) {
	src := t.TempDir()
	sub := filepath.Join(src, "internal", "api")
	if err := os.MkdirAll(sub, 0750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "client.go"), "package api")

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := archive.WalkZip(src, dest); err != nil {
		t.Fatalf("walkZip: %v", err)
	}

	entries := zipEntries(t, dest)
	want := filepath.ToSlash(filepath.Join("internal", "api", "client.go"))
	if _, ok := entries[want]; !ok {
		t.Errorf("zip should contain %q, got entries: %v", want, entries)
	}
}

// ── CreateZip ─────────────────────────────────────────────────────────────────

func TestCreateZip_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), "package main")

	path, err := CreateZip(dir)
	if err != nil {
		t.Fatalf("CreateZip: %v", err)
	}
	defer os.Remove(path)

	if path == "" {
		t.Fatal("CreateZip returned empty path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("zip file not created: %v", err)
	}

	entries := zipEntries(t, path)
	if _, ok := entries["main.go"]; !ok {
		t.Error("zip should contain main.go")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func zipEntries(t *testing.T, path string) map[string]bool {
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
