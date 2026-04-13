package shards

import (
	"os"
	"path/filepath"
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

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRenderAll_RemovesStaleThreeFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "index.ts"), []byte("export function main() {}"), 0o644)

	// Create stale three-file shards
	writeFile(t, filepath.Join(dir, "src", "index.calls.ts"))
	writeFile(t, filepath.Join(dir, "src", "index.deps.ts"))
	writeFile(t, filepath.Join(dir, "src", "index.impact.ts"))

	cache := testCache()
	_, err := RenderAll(dir, cache, []string{"src/index.ts"}, false)
	if err != nil {
		t.Fatal(err)
	}

	// .graph should exist
	if _, err := os.Stat(filepath.Join(dir, "src", "index.graph.ts")); err != nil {
		t.Error("expected index.graph.ts to exist")
	}

	// .calls/.deps/.impact should be gone
	for _, name := range []string{"index.calls.ts", "index.deps.ts", "index.impact.ts"} {
		if _, err := os.Stat(filepath.Join(dir, "src", name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, but it exists", name)
		}
	}
}

func TestRenderAllThreeFile_RemovesStaleGraphFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "index.ts"), []byte("export function main() {}"), 0o644)

	// Create stale single .graph shard
	writeFile(t, filepath.Join(dir, "src", "index.graph.ts"))

	cache := testCache()
	_, err := RenderAllThreeFile(dir, cache, []string{"src/index.ts"}, false)
	if err != nil {
		t.Fatal(err)
	}

	// .graph should be gone
	if _, err := os.Stat(filepath.Join(dir, "src", "index.graph.ts")); !os.IsNotExist(err) {
		t.Error("expected index.graph.ts to be removed after three-file render")
	}

	// At least one of .calls/.deps/.impact should exist
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

func TestIsShardFile_AllFormats(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"index.graph.ts", true},
		{"index.calls.ts", true},
		{"index.deps.ts", true},
		{"index.impact.ts", true},
		{"index.ts", false},
		{"index.test.ts", false},
		{"graph.ts", false},
	}
	for _, tt := range tests {
		if got := isShardFile(tt.name); got != tt.want {
			t.Errorf("isShardFile(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
