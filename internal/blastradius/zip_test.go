package blastradius

import (
	"archive/zip"
	"os"
	"os/exec"
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

func TestWalkZip_SkipsLargeFiles(t *testing.T) {
	src := t.TempDir()
	// Create a file just over 10 MB
	bigFile := filepath.Join(src, "huge.dat")
	if err := os.WriteFile(bigFile, make([]byte, 10<<20+1), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "small.go"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := walkZip(src, dest); err != nil {
		t.Fatal(err)
	}
	entries := readBlastZipEntries(t, dest)
	if entries["huge.dat"] {
		t.Error("file over 10 MB should be excluded from zip")
	}
	if !entries["small.go"] {
		t.Error("small file should be included in zip")
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

func TestWalkZip_CreateDestError(t *testing.T) {
	src := t.TempDir()
	dest := filepath.Join(t.TempDir(), "nonexistent-subdir", "out.zip")
	if err := walkZip(src, dest); err == nil {
		t.Error("walkZip should fail when dest directory does not exist")
	}
}

// TestWalkZip_WalkError covers L101-103: filepath.Walk calls the callback with
// a non-nil error when the source directory does not exist.
func TestWalkZip_WalkError(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := walkZip("/nonexistent-dir-xyzzy-blast", dest); err == nil {
		t.Error("walkZip should fail when source directory does not exist")
	}
}

// TestWalkZip_OpenFileError covers L122-124: os.Open fails when the file exists
// but is not readable (e.g. mode 0000).
func TestWalkZip_OpenFileError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	src := t.TempDir()
	secret := filepath.Join(src, "secret.go")
	if err := os.WriteFile(secret, []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(secret, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(secret, 0600) }) //nolint:errcheck
	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := walkZip(src, dest); err == nil {
		t.Error("walkZip should fail when a source file cannot be opened")
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
	// Verify it's a valid zip
	entries := readBlastZipEntries(t, path)
	if !entries["main.go"] {
		t.Error("created zip should contain main.go")
	}
}

// TestCreateZip_CreateTempError covers L48-50: createZip returns an error when
// os.CreateTemp fails due to an invalid TMPDIR.
func TestCreateZip_CreateTempError(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "nonexistent-tmp"))
	_, err := createZip(t.TempDir())
	if err == nil {
		t.Error("createZip should fail when os.CreateTemp fails")
	}
}

// TestCreateZip_NonExistentDir covers L60-63: createZip cleans up the temp
// file and returns an error when walkZip fails on a missing source dir.
func TestCreateZip_NonExistentDir(t *testing.T) {
	_, err := createZip("/nonexistent-dir-blast-createzip-xyz")
	if err == nil {
		t.Error("createZip should fail when directory does not exist")
	}
}

func initCleanBlastGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %v\n%s", args, err, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "ci@test.local")
	run("git", "config", "user.name", "CI")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "init")
	return dir
}

func TestGitArchive_CleanRepo(t *testing.T) {
	dir := initCleanBlastGitRepo(t)
	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := gitArchive(dir, dest); err != nil {
		t.Fatalf("gitArchive: %v", err)
	}
	entries := readBlastZipEntries(t, dest)
	if !entries["main.go"] {
		t.Error("git archive should contain main.go")
	}
}

func TestIsWorktreeClean_CleanRepo(t *testing.T) {
	dir := initCleanBlastGitRepo(t)
	if !isWorktreeClean(dir) {
		t.Error("freshly committed repo should be considered clean")
	}
}

func TestCreateZip_CleanGitRepo(t *testing.T) {
	dir := initCleanBlastGitRepo(t)
	path, err := createZip(dir)
	if err != nil {
		t.Fatalf("createZip on clean git repo: %v", err)
	}
	defer os.Remove(path)
	entries := readBlastZipEntries(t, path)
	if !entries["main.go"] {
		t.Error("zip should contain main.go from git archive")
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
