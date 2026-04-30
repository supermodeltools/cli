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

func TestRenderAll_RemovesStaleSplitShards(t *testing.T) {
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

func TestRenderAll_DryRunDoesNotRemoveStaleSplitShards(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	for _, name := range []string{"index.calls.ts", "index.deps.ts", "index.impact.ts"} {
		touchFile(t, filepath.Join(dir, "src", name))
	}

	cache := testCache()
	_, err := RenderAll(dir, cache, []string{"src/index.ts"}, true)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "src", "index.graph.ts")); !os.IsNotExist(err) {
		t.Error("dry-run should not write index.graph.ts")
	}
	for _, name := range []string{"index.calls.ts", "index.deps.ts", "index.impact.ts"} {
		if _, err := os.Stat(filepath.Join(dir, "src", name)); err != nil {
			t.Errorf("dry-run should not remove %s: %v", name, err)
		}
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
