package shards

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── isShardFile ───────────────────────────────────────────────────────────────

func TestIsShardFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"handler.graph.go", true},
		{"handler.graph.ts", true},
		{"handler.graph.py", true},
		{"handler.go", false},
		{"handler", false},
		{"", false},
		{".graph.go", true}, // .graph stem is still a shard extension
		{"handler.other.go", false},
	}
	for _, tc := range cases {
		got := isShardFile(tc.name)
		if got != tc.want {
			t.Errorf("isShardFile(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// ── matchPattern ─────────────────────────────────────────────────────────────

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		// Exact substring match (no wildcards)
		{"test", "handler_test.go", true},
		{"test", "handler.go", false},
		// Wildcard *
		{"*.min.js", "app.min.js", true},
		{"*.min.js", "app.js", false},
		{"*.min.js", "app.min.css", false},
		// * in middle
		{"lock*file", "lockfile", true},
		{"lock*file", "lock.file", true},
		{"lock*file", "other", false},
		// Case insensitive
		{"*.PNG", "image.png", true},
		{"test", "TEST_FILE.go", true},
		// No wildcards, no match
		{"abc", "xyz", false},
	}
	for _, tc := range cases {
		got := matchPattern(tc.pattern, tc.name)
		if got != tc.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

// ── shouldInclude ─────────────────────────────────────────────────────────────

func TestShouldInclude_BasicFile(t *testing.T) {
	ex := &zipExclusions{
		skipDirs: map[string]bool{},
		skipExts: map[string]bool{},
	}
	if !shouldInclude("src/main.go", 100, ex) {
		t.Error("basic Go file should be included")
	}
}

func TestShouldInclude_SkipDir(t *testing.T) {
	ex := &zipExclusions{
		skipDirs: map[string]bool{"node_modules": true},
		skipExts: map[string]bool{},
	}
	if shouldInclude("node_modules/pkg/index.js", 100, ex) {
		t.Error("node_modules file should be excluded")
	}
}

func TestShouldInclude_SkipExt(t *testing.T) {
	ex := &zipExclusions{
		skipDirs: map[string]bool{},
		skipExts: map[string]bool{".png": true},
	}
	if shouldInclude("assets/logo.png", 100, ex) {
		t.Error(".png file should be excluded when in skipExts")
	}
}

func TestShouldInclude_ShardFile(t *testing.T) {
	ex := &zipExclusions{
		skipDirs: map[string]bool{},
		skipExts: map[string]bool{},
	}
	if shouldInclude("src/handler.graph.go", 100, ex) {
		t.Error("shard files should be excluded")
	}
}

func TestShouldInclude_MinifiedJS(t *testing.T) {
	ex := &zipExclusions{
		skipDirs: map[string]bool{},
		skipExts: map[string]bool{},
	}
	if shouldInclude("dist/bundle.min.js", 100, ex) {
		t.Error("minified JS should be excluded")
	}
}

func TestShouldInclude_MinifiedCSS(t *testing.T) {
	ex := &zipExclusions{
		skipDirs: map[string]bool{},
		skipExts: map[string]bool{},
	}
	if shouldInclude("styles/app.min.css", 100, ex) {
		t.Error("minified CSS should be excluded")
	}
}

func TestShouldInclude_HardBlockedDir(t *testing.T) {
	ex := &zipExclusions{
		skipDirs: map[string]bool{},
		skipExts: map[string]bool{},
	}
	// .aws is in hardBlocked map
	if shouldInclude(".aws/credentials", 100, ex) {
		t.Error("files under hardBlocked dir .aws should be excluded")
	}
}

func TestShouldInclude_HiddenDir(t *testing.T) {
	ex := &zipExclusions{
		skipDirs: map[string]bool{},
		skipExts: map[string]bool{},
	}
	// .hidden directory → HasPrefix(part, ".") → false
	if shouldInclude(".hidden/secret.txt", 100, ex) {
		t.Error("files under hidden directories should be excluded")
	}
}

func TestShouldInclude_LockFile(t *testing.T) {
	ex := &zipExclusions{
		skipDirs: map[string]bool{},
		skipExts: map[string]bool{},
	}
	if shouldInclude("package-lock.json", 100, ex) {
		t.Error("package-lock.json should be excluded")
	}
}

func TestMatchPattern_QuestionMarkOnly(t *testing.T) {
	// Pattern has ? but no * → len(parts) == 1 after split on * → name == pattern
	// Since "?" is treated as literal, matchPattern("config?", "config?") == true
	// and matchPattern("config?", "config1") == false
	if matchPattern("config?", "config?") != true {
		t.Error("exact match with ? as literal should return true")
	}
	if matchPattern("config?", "config1") != false {
		t.Error("non-matching ? literal should return false")
	}
}

func TestShouldInclude_TooLarge(t *testing.T) {
	ex := &zipExclusions{
		skipDirs: map[string]bool{},
		skipExts: map[string]bool{},
	}
	if shouldInclude("data/huge.dat", maxFileSize+1, ex) {
		t.Error("file exceeding maxFileSize should be excluded")
	}
}

// ── buildExclusions ───────────────────────────────────────────────────────────

func TestBuildExclusions_NoConfig(t *testing.T) {
	dir := t.TempDir()
	ex := buildExclusions(dir)
	if ex == nil {
		t.Fatal("buildExclusions should return non-nil even without config")
	}
	// Standard skip dirs should be present
	if !ex.skipDirs["node_modules"] {
		t.Error("node_modules should be in default skip dirs")
	}
}

func TestBuildExclusions_WithConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"exclude_dirs":["myfolder"],"exclude_exts":[".dat"]}`
	if err := os.WriteFile(filepath.Join(dir, ".supermodel.json"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	ex := buildExclusions(dir)
	if !ex.skipDirs["myfolder"] {
		t.Error("custom exclude_dir 'myfolder' should be added")
	}
	if !ex.skipExts[".dat"] {
		t.Error("custom exclude_ext '.dat' should be added")
	}
}

func TestBuildExclusions_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".supermodel.json"), []byte("{invalid}"), 0644); err != nil {
		t.Fatal(err)
	}
	// Should not panic — just returns defaults.
	ex := buildExclusions(dir)
	if ex == nil {
		t.Fatal("buildExclusions should not return nil on bad JSON")
	}
}

// ── LanguageStats ─────────────────────────────────────────────────────────────

func TestLanguageStats_Basic(t *testing.T) {
	files := []string{
		"main.go", "handler.go", "util.go",
		"index.ts", "types.ts",
		"style.css",
	}
	stats := LanguageStats(files)
	// Should be sorted descending by count: go(3), ts(2), css(1)
	if len(stats) < 3 {
		t.Fatalf("expected at least 3 stats, got %d", len(stats))
	}
	if stats[0].Ext != "go" || stats[0].Count != 3 {
		t.Errorf("first stat: got {%s %d}, want {go 3}", stats[0].Ext, stats[0].Count)
	}
	if stats[1].Ext != "ts" || stats[1].Count != 2 {
		t.Errorf("second stat: got {%s %d}, want {ts 2}", stats[1].Ext, stats[1].Count)
	}
}

func TestLanguageStats_Empty(t *testing.T) {
	if got := LanguageStats(nil); len(got) != 0 {
		t.Errorf("nil: want empty, got %v", got)
	}
	if got := LanguageStats([]string{}); len(got) != 0 {
		t.Errorf("empty slice: want empty, got %v", got)
	}
}

func TestLanguageStats_NoExtension(t *testing.T) {
	files := []string{"Makefile", "LICENSE", "main.go"}
	stats := LanguageStats(files)
	// Makefile and LICENSE have no extension, should be skipped
	if len(stats) != 1 || stats[0].Ext != "go" {
		t.Errorf("no-ext files should be skipped; got %v", stats)
	}
}

func TestLanguageStats_Cap10(t *testing.T) {
	// Generate 15 distinct extensions
	files := make([]string, 15)
	for i := range files {
		files[i] = fmt.Sprintf("file%02d.ext%02d", i, i)
	}
	stats := LanguageStats(files)
	if len(stats) > 10 {
		t.Errorf("LanguageStats should cap at 10, got %d", len(stats))
	}
}

func TestShouldInclude_HardBlockedPattern(t *testing.T) {
	ex := &zipExclusions{
		skipDirs: map[string]bool{},
		skipExts: map[string]bool{},
	}
	// "*.key" is in hardBlockedPatterns
	if shouldInclude("secrets/server.key", 100, ex) {
		t.Error("*.key file should be excluded by hardBlockedPatterns")
	}
	// ".env" is in hardBlockedPatterns
	if shouldInclude(".env", 100, ex) {
		t.Error(".env file should be excluded by hardBlockedPatterns")
	}
}

// ── CreateZipFile ─────────────────────────────────────────────────────────────

func TestCreateZipFile_WalkMode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	// A file that should be excluded
	if err := os.WriteFile(filepath.Join(dir, "main.graph.go"), []byte("// generated"), 0600); err != nil {
		t.Fatal(err)
	}

	path, err := CreateZipFile(dir, nil)
	if err != nil {
		t.Fatalf("CreateZipFile(walk): %v", err)
	}
	defer os.Remove(path)

	r, err := openZipEntries(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if !r["main.go"] {
		t.Error("expected main.go in zip")
	}
	if r["main.graph.go"] {
		t.Error("shard file should be excluded from zip")
	}
}

func TestCreateZipFile_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	hiddenDir := filepath.Join(dir, ".hidden")
	if err := os.MkdirAll(hiddenDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hiddenDir, "secret.go"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}

	path, err := CreateZipFile(dir, nil)
	if err != nil {
		t.Fatalf("CreateZipFile: %v", err)
	}
	defer os.Remove(path)

	r, _ := openZipEntries(t, path)
	for name := range r {
		if strings.HasPrefix(name, ".hidden/") {
			t.Errorf("hidden dir file should be excluded: %s", name)
		}
	}
	if !r["main.go"] {
		t.Error("main.go should be included")
	}
}

func TestCreateZipFile_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	nmDir := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(nmDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nmDir, "dep.js"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.ts"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	path, err := CreateZipFile(dir, nil)
	if err != nil {
		t.Fatalf("CreateZipFile: %v", err)
	}
	defer os.Remove(path)

	r, _ := openZipEntries(t, path)
	for name := range r {
		if strings.HasPrefix(name, "node_modules/") {
			t.Errorf("node_modules should be excluded: %s", name)
		}
	}
}

func TestCreateZipFile_OnlyFilesMode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b"), 0600); err != nil {
		t.Fatal(err)
	}

	// onlyFiles mode: include only a.go
	path, err := CreateZipFile(dir, []string{"a.go"})
	if err != nil {
		t.Fatalf("CreateZipFile(onlyFiles): %v", err)
	}
	defer os.Remove(path)

	r, _ := openZipEntries(t, path)
	if !r["a.go"] {
		t.Error("a.go should be included in onlyFiles mode")
	}
	if r["b.go"] {
		t.Error("b.go should NOT be included when not in onlyFiles")
	}
}

func TestCreateZipFile_WalkMode_Subdir(t *testing.T) {
	// Covers L229 "return nil" for a non-skipped directory.
	dir := t.TempDir()
	subDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(subDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}

	path, err := CreateZipFile(dir, nil)
	if err != nil {
		t.Fatalf("CreateZipFile(walk+subdir): %v", err)
	}
	defer os.Remove(path)

	r, _ := openZipEntries(t, path)
	if !r["src/main.go"] {
		t.Error("src/main.go should be included from subdirectory")
	}
}

func TestCreateZipFile_OnlyFiles_SkipsSymlink(t *testing.T) {
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.go")
	linkFile := filepath.Join(dir, "link.go")
	if err := os.WriteFile(realFile, []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	path, err := CreateZipFile(dir, []string{"link.go", "real.go"})
	if err != nil {
		t.Fatalf("CreateZipFile(symlink): %v", err)
	}
	defer os.Remove(path)

	r, _ := openZipEntries(t, path)
	if r["link.go"] {
		t.Error("symlink should be excluded from onlyFiles mode")
	}
	if !r["real.go"] {
		t.Error("real.go should be included")
	}
}

func TestCreateZipFile_WalkMode_SkipsSymlinks(t *testing.T) {
	// Covers L220 symlink detection in walk mode.
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.go")
	linkFile := filepath.Join(dir, "link.go")
	if err := os.WriteFile(realFile, []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	path, err := CreateZipFile(dir, nil)
	if err != nil {
		t.Fatalf("CreateZipFile(walk+symlinks): %v", err)
	}
	defer os.Remove(path)

	r, _ := openZipEntries(t, path)
	if r["link.go"] {
		t.Error("symlink should be excluded in walk mode")
	}
	if !r["real.go"] {
		t.Error("real.go should be included")
	}
}

func TestCreateZipFile_OnlyFiles_SkipsNonexistent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.go"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	// "ghost.go" doesn't exist → Lstat error → silently skipped
	path, err := CreateZipFile(dir, []string{"real.go", "ghost.go"})
	if err != nil {
		t.Fatalf("CreateZipFile with nonexistent file: %v", err)
	}
	defer os.Remove(path)

	r, _ := openZipEntries(t, path)
	if !r["real.go"] {
		t.Error("real.go should be included")
	}
}

func TestCreateZipFile_OnlyFiles_SkipsShard(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "handler.graph.go"), []byte("// shard"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package h"), 0600); err != nil {
		t.Fatal(err)
	}

	path, err := CreateZipFile(dir, []string{"handler.graph.go", "handler.go"})
	if err != nil {
		t.Fatalf("CreateZipFile: %v", err)
	}
	defer os.Remove(path)

	r, _ := openZipEntries(t, path)
	if r["handler.graph.go"] {
		t.Error("shard file should be excluded in onlyFiles mode")
	}
	if !r["handler.go"] {
		t.Error("source file should be included")
	}
}

// ── DryRunList ────────────────────────────────────────────────────────────────

func TestDryRunList_Basic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.graph.go"), []byte("// shard"), 0600); err != nil {
		t.Fatal(err)
	}

	files, err := DryRunList(dir)
	if err != nil {
		t.Fatalf("DryRunList: %v", err)
	}

	found := false
	for _, f := range files {
		if f == "main.go" {
			found = true
		}
		if f == "main.graph.go" {
			t.Error("shard file should be excluded from DryRunList")
		}
	}
	if !found {
		t.Error("main.go should be in DryRunList")
	}
}

func TestDryRunList_WithSubdir(t *testing.T) {
	// Covers L282 "return nil" for a non-skipped directory in DryRunList.
	dir := t.TempDir()
	subDir := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(subDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "util.go"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	files, err := DryRunList(dir)
	if err != nil {
		t.Fatalf("DryRunList(subdir): %v", err)
	}
	want := filepath.Join("pkg", "util.go")
	found := false
	for _, f := range files {
		if f == want {
			found = true
		}
	}
	if !found {
		t.Errorf("%s should be in DryRunList; got %v", want, files)
	}
}

func TestDryRunList_SkipsSymlinks(t *testing.T) {
	// Covers L273 symlink detection in DryRunList.
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.go")
	linkFile := filepath.Join(dir, "link.go")
	if err := os.WriteFile(realFile, []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	files, err := DryRunList(dir)
	if err != nil {
		t.Fatalf("DryRunList(symlink): %v", err)
	}
	for _, f := range files {
		if f == "link.go" {
			t.Error("symlink should be excluded from DryRunList")
		}
	}
	found := false
	for _, f := range files {
		if f == "real.go" {
			found = true
		}
	}
	if !found {
		t.Error("real.go should be included in DryRunList")
	}
}

func TestDryRunList_SkipsHiddenAndSkipDirs(t *testing.T) {
	dir := t.TempDir()
	// Hidden dir
	hiddenDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(hiddenDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hiddenDir, "HEAD"), []byte("ref"), 0600); err != nil {
		t.Fatal(err)
	}
	// node_modules skip dir
	nmDir := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(nmDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nmDir, "dep.js"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	files, err := DryRunList(dir)
	if err != nil {
		t.Fatalf("DryRunList: %v", err)
	}

	for _, f := range files {
		if strings.HasPrefix(f, ".git/") || strings.HasPrefix(f, "node_modules/") {
			t.Errorf("DryRunList should skip %s", f)
		}
	}
	found := false
	for _, f := range files {
		if f == "app.go" {
			found = true
		}
	}
	if !found {
		t.Error("app.go should be in DryRunList")
	}
}

// ── PrintLanguageBarChart ─────────────────────────────────────────────────────

func TestPrintLanguageBarChart_Empty(t *testing.T) {
	// Should not panic on empty stats.
	PrintLanguageBarChart(nil, 0)
}

func TestPrintLanguageBarChart_Basic(t *testing.T) {
	// Should not panic or error for normal input.
	stats := []LangStat{
		{Ext: "go", Count: 10},
		{Ext: "ts", Count: 5},
		{Ext: "py", Count: 1}, // barLen calculation covers the barLen < 1 branch
	}
	PrintLanguageBarChart(stats, 16)
}

func TestPrintLanguageBarChart_SmallCount(t *testing.T) {
	// Single stat with count 1 (maxCount = 1, barLen = 28*1/1 = 28, not < 1)
	// Use a small maxCount relative to others to trigger barLen < 1 branch:
	// stats[0].Count = 100, stats[1].Count = 1 → barLen = 28*1/100 = 0 < 1 → barLen = 1
	stats := []LangStat{
		{Ext: "go", Count: 100},
		{Ext: "rs", Count: 1},
	}
	PrintLanguageBarChart(stats, 101)
}

func TestLanguageStats_TiesSortedAlphabetically(t *testing.T) {
	// b.go, a.ts — same count (1 each), should sort a before b alphabetically
	files := []string{"b.go", "a.ts"}
	stats := LanguageStats(files)
	if len(stats) != 2 {
		t.Fatalf("expected 2, got %d", len(stats))
	}
	if stats[0].Ext != "go" || stats[1].Ext != "ts" {
		// alphabetically "go" < "ts", so go comes first
		t.Errorf("ties: got [%s, %s], want [go, ts]", stats[0].Ext, stats[1].Ext)
	}
}

// TestAddFileToZip_OpenError covers L366: addFileToZip returns error when the
// source file cannot be opened (nonexistent path).
func TestAddFileToZip_OpenError(t *testing.T) {
	tmp, err := os.CreateTemp("", "test-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	zw := zip.NewWriter(tmp)
	defer zw.Close()

	err = addFileToZip(zw, "/nonexistent/path/file.go", "file.go")
	if err == nil {
		t.Error("expected error when source file does not exist")
	}
}

func openZipEntries(t *testing.T, path string) (map[string]bool, error) {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	m := make(map[string]bool, len(r.File))
	for _, f := range r.File {
		m[f.Name] = true
	}
	return m, nil
}

// ── CreateZipFile / DryRunList error paths ────────────────────────────────────

// TestCreateZipFile_OnlyFiles_UnreadableFileError covers L202-207:
// addFileToZip returns an error when a file in onlyFiles is not readable.
func TestCreateZipFile_OnlyFiles_UnreadableFileError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	// Name must not match any hardBlockedPattern (e.g. "*secret*").
	locked := filepath.Join(dir, "locked.go")
	if err := os.WriteFile(locked, []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0600) }) //nolint:errcheck

	_, err := CreateZipFile(dir, []string{"locked.go"})
	if err == nil {
		t.Error("CreateZipFile should fail when an onlyFiles entry cannot be opened")
	}
}

// TestCreateZipFile_WalkMode_UnreadableSubdir covers L211-213: the Walk callback
// receives err != nil for an unreadable subdirectory and returns nil to skip it.
// Since the callback returns nil (not the error), the walk succeeds overall.
func TestCreateZipFile_WalkMode_UnreadableSubdir(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	subdir := filepath.Join(dir, "secret")
	if err := os.MkdirAll(subdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "file.go"), []byte("package x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(subdir, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(subdir, 0755) }) //nolint:errcheck

	// Walk mode (onlyFiles == nil) — the unreadable subdir triggers L211-213 but
	// CreateZipFile succeeds because the error is silently skipped.
	path, err := CreateZipFile(dir, nil)
	if err != nil {
		t.Fatalf("CreateZipFile should succeed when walk errors are silently skipped: %v", err)
	}
	defer os.Remove(path)
}

// TestCreateZipFile_WalkMode_UnreadableFile covers L238-243: addFileToZip
// returns an error for an unreadable file during the walk, causing CreateZipFile
// to clean up and return an error.
func TestCreateZipFile_WalkMode_UnreadableFile(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	secret := filepath.Join(dir, "locked.go")
	if err := os.WriteFile(secret, []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(secret, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(secret, 0600) }) //nolint:errcheck

	// Walk mode — the unreadable file causes addFileToZip to fail.
	_, err := CreateZipFile(dir, nil)
	if err == nil {
		t.Error("CreateZipFile should fail when a file in walk mode cannot be opened")
	}
}

// TestDryRunList_WalkError covers L264-266: DryRunList's Walk callback receives
// err != nil (from an unreadable subdir) and silently skips it (returns nil).
func TestDryRunList_WalkError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	subdir := filepath.Join(dir, "locked")
	if err := os.MkdirAll(subdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "file.go"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(subdir, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(subdir, 0755) }) //nolint:errcheck

	// DryRunList should succeed despite the locked subdir.
	files, err := DryRunList(dir)
	if err != nil {
		t.Fatalf("DryRunList should succeed when walk errors are skipped: %v", err)
	}
	_ = files
}

// TestCreateZipFile_CreateTempError covers L182-184: CreateZipFile returns
// an error when os.CreateTemp fails (TMPDIR points to a nonexistent directory).
func TestCreateZipFile_CreateTempError(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "nonexistent-tmp-dir"))
	t.Setenv("TMP", filepath.Join(t.TempDir(), "nonexistent-tmp-dir"))
	t.Setenv("TEMP", filepath.Join(t.TempDir(), "nonexistent-tmp-dir"))
	_, err := CreateZipFile(t.TempDir(), nil)
	if err == nil {
		t.Error("expected error when os.CreateTemp fails due to invalid TMPDIR")
	}
}
