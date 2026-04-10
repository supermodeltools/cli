package shards

import (
	"os"
	"path/filepath"
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
