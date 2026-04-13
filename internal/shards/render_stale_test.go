package shards

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

func testCache() *Cache {
	ir := &api.ShardIR{
		Graph: api.ShardGraph{
			Nodes: []api.Node{
				{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/index.ts", "name": "index.ts"}},
				{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/utils.ts", "name": "utils.ts"}},
				{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"filePath": "src/index.ts", "name": "main"}},
				{ID: "fn2", Labels: []string{"Function"}, Properties: map[string]any{"filePath": "src/utils.ts", "name": "helper"}},
			},
			Relationships: []api.Relationship{
				{ID: "r1", Type: "defines_function", StartNode: "f1", EndNode: "fn1"},
				{ID: "r2", Type: "defines_function", StartNode: "f2", EndNode: "fn2"},
				{ID: "r3", Type: "imports", StartNode: "f1", EndNode: "f2"},
				{ID: "r4", Type: "calls", StartNode: "fn1", EndNode: "fn2"},
			},
		},
	}
	c := NewCache()
	c.Build(ir)
	return c
}

// testCacheNoImpact returns a cache where src/lonely.ts has no importers
// and no callers — so the impact section will be empty. lonely.ts imports
// index.ts (so it has deps) but nothing imports lonely.ts.
func testCacheNoImpact() *Cache {
	ir := &api.ShardIR{
		Graph: api.ShardGraph{
			Nodes: []api.Node{
				{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/index.ts", "name": "index.ts"}},
				{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/lonely.ts", "name": "lonely.ts"}},
			},
			Relationships: []api.Relationship{
				{ID: "r1", Type: "imports", StartNode: "f2", EndNode: "f1"},
			},
		},
	}
	c := NewCache()
	c.Build(ir)
	return c
}

func touchFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ── Stale file cleanup ──────────────────────────────────────────

func TestRenderAll_RemovesStaleThreeFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	touchFile(t, filepath.Join(dir, "src", "index.calls.ts"))
	touchFile(t, filepath.Join(dir, "src", "index.deps.ts"))
	touchFile(t, filepath.Join(dir, "src", "index.impact.ts"))

	cache := testCache()
	_, err := RenderAll(dir, cache, []string{"src/index.ts"}, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "src", "index.graph.ts")); err != nil {
		t.Error("expected index.graph.ts to exist")
	}

	for _, name := range []string{"index.calls.ts", "index.deps.ts", "index.impact.ts"} {
		if _, err := os.Stat(filepath.Join(dir, "src", name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, but it exists", name)
		}
	}
}

func TestRenderAllThreeFile_RemovesStaleGraphFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	touchFile(t, filepath.Join(dir, "src", "index.graph.ts"))

	cache := testCache()
	_, err := RenderAllThreeFile(dir, cache, []string{"src/index.ts"}, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "src", "index.graph.ts")); !os.IsNotExist(err) {
		t.Error("expected index.graph.ts to be removed after three-file render")
	}

	found := false
	for _, name := range []string{"index.calls.ts", "index.deps.ts", "index.impact.ts"} {
		if _, err := os.Stat(filepath.Join(dir, "src", name)); err == nil {
			found = true
		}
	}
	if !found {
		t.Error("expected at least one three-file shard to exist")
	}
}

// ── Happy-path content verification ─────────────────────────────

func TestRenderAllThreeFile_CallsContent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	cache := testCache()
	_, err := RenderAllThreeFile(dir, cache, []string{"src/index.ts"}, false)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "src", "index.calls.ts"))
	if err != nil {
		t.Fatal("index.calls.ts not written")
	}
	content := string(data)
	if !strings.Contains(content, "[calls]") {
		t.Error("calls file missing [calls] section header")
	}
	if !strings.Contains(content, "main") {
		t.Error("calls file missing 'main' function")
	}
	if !strings.Contains(content, "helper") {
		t.Error("calls file missing 'helper' callee")
	}
}

func TestRenderAllThreeFile_DepsContent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	cache := testCache()
	_, err := RenderAllThreeFile(dir, cache, []string{"src/index.ts"}, false)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "src", "index.deps.ts"))
	if err != nil {
		t.Fatal("index.deps.ts not written")
	}
	content := string(data)
	if !strings.Contains(content, "[deps]") {
		t.Error("deps file missing [deps] section header")
	}
	if !strings.Contains(content, "imports") {
		t.Error("deps file missing 'imports' line")
	}
	if !strings.Contains(content, "utils.ts") {
		t.Error("deps file missing utils.ts import")
	}
}

func TestRenderAllThreeFile_ImpactContent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	cache := testCache()
	// utils.ts has an importer (index.ts) so it will have impact data
	_, err := RenderAllThreeFile(dir, cache, []string{"src/utils.ts"}, false)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "src", "utils.impact.ts"))
	if err != nil {
		t.Fatal("utils.impact.ts not written")
	}
	content := string(data)
	if !strings.Contains(content, "[impact]") {
		t.Error("impact file missing [impact] section header")
	}
	if !strings.Contains(content, "risk") {
		t.Error("impact file missing risk line")
	}
	if !strings.Contains(content, "direct") {
		t.Error("impact file missing direct count")
	}
}

func TestRenderAll_GraphContent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	cache := testCache()
	_, err := RenderAll(dir, cache, []string{"src/index.ts"}, false)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "src", "index.graph.ts"))
	if err != nil {
		t.Fatal("index.graph.ts not written")
	}
	content := string(data)
	if !strings.Contains(content, "[deps]") {
		t.Error("graph file missing [deps] section")
	}
	if !strings.Contains(content, "[calls]") {
		t.Error("graph file missing [calls] section")
	}
	if !strings.Contains(content, "imports") {
		t.Error("graph file missing imports data")
	}
	if !strings.Contains(content, "main") {
		t.Error("graph file missing main function")
	}
}

// ── Empty section cleanup ───────────────────────────────────────

func TestRenderAllThreeFile_EmptySectionRemovesStaleFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	// lonely.ts has no importers and no callers — impact will be empty
	// Pre-create a stale .impact file
	touchFile(t, filepath.Join(dir, "src", "lonely.impact.ts"))

	cache := testCacheNoImpact()
	_, err := RenderAllThreeFile(dir, cache, []string{"src/lonely.ts"}, false)
	if err != nil {
		t.Fatal(err)
	}

	// deps file should exist (lonely.ts is imported by index.ts)
	if _, err := os.Stat(filepath.Join(dir, "src", "lonely.deps.ts")); err != nil {
		t.Error("expected lonely.deps.ts to exist (it has an importer)")
	}

	// impact file should be removed (no importers of lonely.ts, no callers)
	if _, err := os.Stat(filepath.Join(dir, "src", "lonely.impact.ts")); !os.IsNotExist(err) {
		t.Error("expected lonely.impact.ts to be removed (empty impact section)")
	}
}

// ── Path traversal on delete ────────────────────────────────────

func TestSafeRemove_BlocksTraversal(t *testing.T) {
	dir := t.TempDir()

	// Create a file outside the repo dir
	outside := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	repoDir := filepath.Join(dir, "repo")
	os.MkdirAll(repoDir, 0o755)

	// Attempt traversal
	safeRemove(repoDir, "../outside.txt")

	// File should still exist
	if _, err := os.Stat(outside); err != nil {
		t.Error("safeRemove should not delete files outside repoDir via traversal")
	}
}

func TestSafeRemove_AllowsInsideRepo(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "src", "old.graph.ts")
	touchFile(t, target)

	safeRemove(dir, "src/old.graph.ts")

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("safeRemove should delete files inside repoDir")
	}
}

// ── isShardFile ─────────────────────────────────────────────────

func TestIsShardFile_AllFormats(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"index.graph.ts", true},
		{"index.calls.ts", true},
		{"index.deps.ts", true},
		{"index.impact.ts", true},
		{"index.graph.go", true},
		{"index.calls.py", true},
		{"index.ts", false},
		{"index.test.ts", false},
		{"graph.ts", false},
		{"calls.ts", false},
	}
	for _, tt := range tests {
		if got := isShardFile(tt.name); got != tt.want {
			t.Errorf("isShardFile(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
