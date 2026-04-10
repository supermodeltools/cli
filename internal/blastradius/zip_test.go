package blastradius

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── isGitRepo ─────────────────────────────────────────────────────────────────

func TestIsGitRepo_NonGitDir(t *testing.T) {
	if isGitRepo(t.TempDir()) {
		t.Error("empty temp dir should not be a git repo")
	}
}

// ── isWorktreeClean ───────────────────────────────────────────────────────────

func TestIsWorktreeClean_NonGitDir(t *testing.T) {
	// git status on a non-repo exits non-zero → returns false
	if isWorktreeClean(t.TempDir()) {
		t.Error("non-git dir should not be considered clean")
	}
}

// ── walkZip ───────────────────────────────────────────────────────────────────

func TestWalkZip_IncludesSourceFiles(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := walkZip(src, dest); err != nil {
		t.Fatalf("walkZip: %v", err)
	}
	entries := readBlastZipEntries(t, dest)
	if !entries["main.go"] {
		t.Error("zip should contain main.go")
	}
}

func TestWalkZip_SkipsHiddenFiles(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, ".env"), []byte("SECRET=x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "app.ts"), []byte("export {}"), 0600); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := walkZip(src, dest); err != nil {
		t.Fatal(err)
	}
	entries := readBlastZipEntries(t, dest)
	if entries[".env"] {
		t.Error("zip should not contain .env")
	}
	if !entries["app.ts"] {
		t.Error("zip should contain app.ts")
	}
}

func TestWalkZip_SkipsNodeModules(t *testing.T) {
	src := t.TempDir()
	nmDir := filepath.Join(src, "node_modules")
	if err := os.Mkdir(nmDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nmDir, "pkg.js"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "index.ts"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := walkZip(src, dest); err != nil {
		t.Fatal(err)
	}
	entries := readBlastZipEntries(t, dest)
	for name := range entries {
		if strings.HasPrefix(name, "node_modules/") || name == "node_modules" {
			t.Errorf("should not contain node_modules entry: %s", name)
		}
	}
	if !entries["index.ts"] {
		t.Error("zip should contain index.ts")
	}
}

func TestWalkZip_SkipsOtherSkipDirs(t *testing.T) {
	for _, dir := range []string{"dist", "build", "vendor", ".git"} {
		src := t.TempDir()
		skipDir := filepath.Join(src, dir)
		if err := os.Mkdir(skipDir, 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skipDir, "file.js"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}

		dest := filepath.Join(t.TempDir(), "out.zip")
		if err := walkZip(src, dest); err != nil {
			t.Fatalf("walkZip with %s: %v", dir, err)
		}
		entries := readBlastZipEntries(t, dest)
		for name := range entries {
			if strings.HasPrefix(name, dir+"/") {
				t.Errorf("should not contain %s/ entry: %s", dir, name)
			}
		}
	}
}

// ── createZip ─────────────────────────────────────────────────────────────────

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
	// Verify it's a valid zip
	entries := readBlastZipEntries(t, path)
	if !entries["main.go"] {
		t.Error("created zip should contain main.go")
	}
}

func readBlastZipEntries(t *testing.T, path string) map[string]bool {
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
