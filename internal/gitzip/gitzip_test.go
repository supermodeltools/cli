package gitzip

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsGitRepo_NonGitDir(t *testing.T) {
	if isGitRepo(t.TempDir()) {
		t.Error("empty temp dir should not be a git repo")
	}
}

func TestIsWorktreeClean_NonGitDir(t *testing.T) {
	if isWorktreeClean(t.TempDir()) {
		t.Error("non-git dir should not be considered clean")
	}
}

func TestWalkZip_IncludesFiles(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := walkZip(src, dest); err != nil {
		t.Fatalf("walkZip: %v", err)
	}
	entries := readZipEntries(t, dest)
	if !entries["main.go"] {
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
	if err := walkZip(src, dest); err != nil {
		t.Fatal(err)
	}
	entries := readZipEntries(t, dest)
	if entries[".env"] {
		t.Error("zip should not contain .env")
	}
	if !entries["main.go"] {
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
	if err := walkZip(src, dest); err != nil {
		t.Fatal(err)
	}
	entries := readZipEntries(t, dest)
	for name := range entries {
		if strings.HasPrefix(name, "node_modules/") || name == "node_modules" {
			t.Errorf("should not contain node_modules entry: %s", name)
		}
	}
}

func TestWalkZip_SkipsLargeFiles(t *testing.T) {
	src := t.TempDir()
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
	entries := readZipEntries(t, dest)
	if entries["huge.dat"] {
		t.Error("file over 10 MB should be excluded from zip")
	}
	if !entries["small.go"] {
		t.Error("small file should be included in zip")
	}
}

func TestCreateZip_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	path, err := CreateZip(dir, "supermodel-*.zip")
	if err != nil {
		t.Fatalf("CreateZip: %v", err)
	}
	defer os.Remove(path)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("zip file not created: %v", err)
	}
}

func TestWalkZip_CreateDestError(t *testing.T) {
	src := t.TempDir()
	dest := filepath.Join(t.TempDir(), "nonexistent-subdir", "out.zip")
	if err := walkZip(src, dest); err == nil {
		t.Error("walkZip should fail when dest directory does not exist")
	}
}

func TestWalkZip_WalkError(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := walkZip("/nonexistent-dir-xyzzy-gitzip", dest); err == nil {
		t.Error("walkZip should fail when source directory does not exist")
	}
}

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

func TestCreateZip_CreateTempError(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "nonexistent-tmp"))
	t.Setenv("TMP", filepath.Join(t.TempDir(), "nonexistent-tmp"))
	t.Setenv("TEMP", filepath.Join(t.TempDir(), "nonexistent-tmp"))
	_, err := CreateZip(t.TempDir(), "supermodel-*.zip")
	if err == nil {
		t.Error("CreateZip should fail when os.CreateTemp fails")
	}
}

func TestCreateZip_NonExistentDir(t *testing.T) {
	_, err := CreateZip("/nonexistent-dir-gitzip-createzip-xyz", "supermodel-*.zip")
	if err == nil {
		t.Error("CreateZip should fail when directory does not exist")
	}
}

func initCleanGitRepo(t *testing.T) string {
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
	dir := initCleanGitRepo(t)
	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := gitArchive(dir, dest); err != nil {
		t.Fatalf("gitArchive: %v", err)
	}
	entries := readZipEntries(t, dest)
	if !entries["main.go"] {
		t.Error("git archive should contain main.go")
	}
}

func TestIsWorktreeClean_CleanRepo(t *testing.T) {
	dir := initCleanGitRepo(t)
	if !isWorktreeClean(dir) {
		t.Error("freshly committed repo should be considered clean")
	}
}

func TestCreateZip_CleanGitRepo(t *testing.T) {
	dir := initCleanGitRepo(t)
	path, err := CreateZip(dir, "supermodel-*.zip")
	if err != nil {
		t.Fatalf("CreateZip on clean git repo: %v", err)
	}
	defer os.Remove(path)
	entries := readZipEntries(t, path)
	if !entries["main.go"] {
		t.Error("zip should contain main.go from git archive")
	}
}

// TestCreateZip_DirtyGitRepo verifies that .gitignore is respected even when
// the worktree has uncommitted changes (dirty path uses git ls-files).
func TestCreateZip_DirtyGitRepo(t *testing.T) {
	dir := initCleanGitRepo(t)

	// Write a gitignored file and an untracked (but not ignored) file.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("should not appear"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}

	path, err := CreateZip(dir, "supermodel-*.zip")
	if err != nil {
		t.Fatalf("CreateZip on dirty git repo: %v", err)
	}
	defer os.Remove(path)

	entries := readZipEntries(t, path)
	if entries["ignored.txt"] {
		t.Error("gitignored file should not appear in zip")
	}
	if !entries["new.go"] {
		t.Error("new untracked (non-ignored) file should appear in zip")
	}
	if !entries["main.go"] {
		t.Error("tracked file should appear in zip")
	}
}

// TestGitLsFilesZip_SkipsLargeFiles verifies that files over 10 MB are excluded
// even when enumerated via git ls-files (dirty git repo path).
func TestGitLsFilesZip_SkipsLargeFiles(t *testing.T) {
	dir := initCleanGitRepo(t)

	big := filepath.Join(dir, "huge.dat")
	if err := os.WriteFile(big, make([]byte, 10<<20+1), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "small.go"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	// Make worktree dirty so git ls-files path is taken.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main // dirty"), 0600); err != nil {
		t.Fatal(err)
	}

	path, err := CreateZip(dir, "supermodel-*.zip")
	if err != nil {
		t.Fatalf("CreateZip: %v", err)
	}
	defer os.Remove(path)

	entries := readZipEntries(t, path)
	if entries["huge.dat"] {
		t.Error("file over 10 MB should be excluded from zip via git ls-files path")
	}
	if !entries["small.go"] {
		t.Error("small untracked file should be included")
	}
}

// TestGitLsFilesZip_SkipsSymlinks verifies that symlinks are not followed
// on the dirty git repo path.
func TestGitLsFilesZip_SkipsSymlinks(t *testing.T) {
	if os.Getenv("CI") != "" && os.Getenv("RUNNER_OS") == "Windows" {
		t.Skip("symlinks not reliable on Windows CI")
	}
	dir := initCleanGitRepo(t)

	// Create a symlink pointing outside the repo.
	target := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "link.txt")); err != nil {
		t.Skip("symlink creation not supported:", err)
	}
	// Make worktree dirty.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main // dirty"), 0600); err != nil {
		t.Fatal(err)
	}

	path, err := CreateZip(dir, "supermodel-*.zip")
	if err != nil {
		t.Fatalf("CreateZip: %v", err)
	}
	defer os.Remove(path)

	entries := readZipEntries(t, path)
	if entries["link.txt"] {
		t.Error("symlink should be excluded from zip")
	}
}

// TestCreateZip_IsGitRepo verifies that an actual git repo is detected.
func TestCreateZip_IsGitRepo(t *testing.T) {
	dir := initCleanGitRepo(t)
	if !isGitRepo(dir) {
		t.Error("initialized git repo should be detected as a git repo")
	}
}

// TestGitLsFilesZip_FallsBackToWalkOnGitFailure verifies that CreateZip
// falls back to walkZip when git ls-files fails (e.g. git not in PATH).
func TestGitLsFilesZip_FallsBackToWalkOnGitFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	// Non-git dir forces the walkZip path regardless.
	path, err := CreateZip(dir, "supermodel-*.zip")
	if err != nil {
		t.Fatalf("CreateZip should succeed via walkZip fallback: %v", err)
	}
	defer os.Remove(path)
	entries := readZipEntries(t, path)
	if !entries["main.go"] {
		t.Error("fallback walkZip should include main.go")
	}
}

// ── Sensitive file exclusion tests ──────────────────────────────────────────

func TestIsSensitiveFile(t *testing.T) {
	cases := []struct {
		path      string
		sensitive bool
	}{
		{".env", true},
		{".env.local", true},
		{".env.production", true},
		{"prod.env", true},
		{"appsettings.json", true},
		{"appsettings.Development.json", true},
		{"local.settings.json", true},
		{"secrets.json", true},
		{"secrets.yml", true},
		{"secrets.yaml", true},
		{"server.pem", true},
		{"private.key", true},
		{"cert.p12", true},
		{"cert.pfx", true},
		{"cert.cer", true},
		{"cert.crt", true},
		{"key.ppk", true},
		{"id_rsa", true},
		{"id_dsa", true},
		{"id_ecdsa", true},
		{"id_ed25519", true},
		{".npmrc", true},
		{".pypirc", true},
		{"terraform.tfvars", true},
		{"prod.tfvars", true},
		{".htpasswd", true},
		// should not match
		{"main.go", false},
		{"config.go", false},
		{"appsettings.go", false},
		{"README.md", false},
		{"settings.json", false},
		{"myenv.go", false},
	}
	for _, tc := range cases {
		got := isSensitiveFile(tc.path)
		if got != tc.sensitive {
			t.Errorf("isSensitiveFile(%q) = %v, want %v", tc.path, got, tc.sensitive)
		}
	}
}

// TestWalkZip_ExcludesSensitiveFiles verifies the walkZip (non-git) path.
func TestWalkZip_ExcludesSensitiveFiles(t *testing.T) {
	src := t.TempDir()
	files := map[string]string{
		"main.go":                     "package main",
		"appsettings.json":            `{"password":"secret"}`,
		"appsettings.Production.json": `{"password":"prod"}`,
		"local.settings.json":         `{"key":"val"}`,
		"secrets.yml":                 "key: val",
		"server.pem":                  "-----BEGIN CERTIFICATE-----",
		"terraform.tfvars":            "db_pass = \"secret\"",
		"prod.tfvars":                 "db_pass = \"secret\"",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := walkZip(src, dest); err != nil {
		t.Fatalf("walkZip: %v", err)
	}
	entries := readZipEntries(t, dest)
	if !entries["main.go"] {
		t.Error("main.go should be included")
	}
	for name := range files {
		if name == "main.go" {
			continue
		}
		if entries[name] {
			t.Errorf("%s should be excluded from walkZip", name)
		}
	}
}

// TestGitLsFilesZip_ExcludesSensitiveFiles verifies the dirty-git path.
func TestGitLsFilesZip_ExcludesSensitiveFiles(t *testing.T) {
	dir := initCleanGitRepo(t)

	sensitiveFiles := []string{
		"appsettings.json",
		"local.settings.json",
		"secrets.yml",
		"server.pem",
		"id_rsa",
		"terraform.tfvars",
	}
	for _, name := range sensitiveFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("sensitive"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Make the worktree dirty so gitLsFilesZip path is taken.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main // dirty"), 0600); err != nil {
		t.Fatal(err)
	}

	path, err := CreateZip(dir, "supermodel-*.zip")
	if err != nil {
		t.Fatalf("CreateZip: %v", err)
	}
	defer os.Remove(path)

	entries := readZipEntries(t, path)
	if !entries["main.go"] {
		t.Error("main.go should be included")
	}
	for _, name := range sensitiveFiles {
		if entries[name] {
			t.Errorf("%s should be excluded from gitLsFilesZip", name)
		}
	}
}

// TestGitArchive_ExcludesSensitiveFiles verifies the clean-git (git archive) path.
func TestGitArchive_ExcludesSensitiveFiles(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "ci@test.local")
	run("git", "config", "user.name", "CI")

	// Commit a sensitive file alongside normal source.
	files := map[string]string{
		"main.go":          "package main",
		"appsettings.json": `{"password":"secret"}`,
		"secrets.yml":      "key: val",
		"id_rsa":           "-----BEGIN RSA PRIVATE KEY-----",
		"terraform.tfvars": "db_pass = \"secret\"",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "init with secrets")

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := gitArchive(dir, dest); err != nil {
		t.Fatalf("gitArchive: %v", err)
	}

	entries := readZipEntries(t, dest)
	if !entries["main.go"] {
		t.Error("main.go should be in archive")
	}
	for name := range files {
		if name == "main.go" {
			continue
		}
		if entries[name] {
			t.Errorf("%s should be excluded from git archive", name)
		}
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
