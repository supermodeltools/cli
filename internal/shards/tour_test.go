package shards

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

// TestTopoStrategy_LeavesFirst verifies that files with no outbound imports
// (leaves of the dependency graph) appear before files that import them.
func TestTopoStrategy_LeavesFirst(t *testing.T) {
	// main.go imports lib.go; lib.go imports util.go. Expected: util, lib, main.
	nodes := []api.Node{
		{ID: "f:src/util.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/util.go"}},
		{ID: "f:src/lib.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/lib.go"}},
		{ID: "f:src/main.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/main.go"}},
	}
	rels := []api.Relationship{
		{ID: "r1", Type: "imports", StartNode: "f:src/main.go", EndNode: "f:src/lib.go"},
		{ID: "r2", Type: "imports", StartNode: "f:src/lib.go", EndNode: "f:src/util.go"},
	}
	cache := NewCache()
	cache.Build(&api.ShardIR{Graph: api.ShardGraph{Nodes: nodes, Relationships: rels}})

	order := TopoStrategy{}.Order(cache)

	if len(order) != 3 {
		t.Fatalf("expected 3 files in order, got %d: %v", len(order), order)
	}
	idx := map[string]int{}
	for i, f := range order {
		idx[f] = i
	}
	if idx["src/util.go"] > idx["src/lib.go"] {
		t.Errorf("leaf util.go should precede lib.go; got %v", order)
	}
	if idx["src/lib.go"] > idx["src/main.go"] {
		t.Errorf("lib.go should precede main.go; got %v", order)
	}
}

// TestTopoStrategy_Deterministic verifies that two equivalent graphs produce
// identical orderings regardless of relationship insertion order.
func TestTopoStrategy_Deterministic(t *testing.T) {
	nodes := []api.Node{
		{ID: "f:a.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "a.go"}},
		{ID: "f:b.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "b.go"}},
		{ID: "f:c.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "c.go"}},
	}
	// a and b are both leaves that c imports.
	rels1 := []api.Relationship{
		{ID: "r1", Type: "imports", StartNode: "f:c.go", EndNode: "f:a.go"},
		{ID: "r2", Type: "imports", StartNode: "f:c.go", EndNode: "f:b.go"},
	}
	rels2 := []api.Relationship{
		{ID: "r2", Type: "imports", StartNode: "f:c.go", EndNode: "f:b.go"},
		{ID: "r1", Type: "imports", StartNode: "f:c.go", EndNode: "f:a.go"},
	}

	c1 := NewCache()
	c1.Build(&api.ShardIR{Graph: api.ShardGraph{Nodes: nodes, Relationships: rels1}})
	c2 := NewCache()
	c2.Build(&api.ShardIR{Graph: api.ShardGraph{Nodes: nodes, Relationships: rels2}})

	o1 := TopoStrategy{}.Order(c1)
	o2 := TopoStrategy{}.Order(c2)

	if strings.Join(o1, "|") != strings.Join(o2, "|") {
		t.Errorf("non-deterministic ordering\n  run1: %v\n  run2: %v", o1, o2)
	}
}

// TestTopoStrategy_HandlesCycles verifies cycles don't cause infinite loops
// and all files still appear in the output.
func TestTopoStrategy_HandlesCycles(t *testing.T) {
	nodes := []api.Node{
		{ID: "f:a.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "a.go"}},
		{ID: "f:b.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "b.go"}},
	}
	rels := []api.Relationship{
		{ID: "r1", Type: "imports", StartNode: "f:a.go", EndNode: "f:b.go"},
		{ID: "r2", Type: "imports", StartNode: "f:b.go", EndNode: "f:a.go"},
	}
	cache := NewCache()
	cache.Build(&api.ShardIR{Graph: api.ShardGraph{Nodes: nodes, Relationships: rels}})

	order := TopoStrategy{}.Order(cache)

	if len(order) != 2 {
		t.Fatalf("cycle participants must all appear; got %v", order)
	}
}

// TestRenderTour_ContainsDomainHeadings verifies domain grouping appears in output.
func TestRenderTour_ContainsDomainHeadings(t *testing.T) {
	nodes := []api.Node{
		{ID: "f:src/a.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/a.go"}},
	}
	cache := NewCache()
	cache.Build(&api.ShardIR{Graph: api.ShardGraph{Nodes: nodes}})
	cache.FileDomain["src/a.go"] = "Core/Utils"

	body := RenderTour(cache, TopoStrategy{}, "..")

	if !strings.Contains(body, "# Repository Tour") {
		t.Errorf("missing title in tour body:\n%s", body)
	}
	if !strings.Contains(body, "Domain: Core") {
		t.Errorf("missing domain heading: %s", body)
	}
	if !strings.Contains(body, "Subdomain: Utils") {
		t.Errorf("missing subdomain heading: %s", body)
	}
	if !strings.Contains(body, "src/a.go") {
		t.Errorf("missing file entry: %s", body)
	}
}

// TestRenderTour_ShardLinkRelative verifies the shard link uses .. prefix so
// that TOUR.md in .supermodel/ can resolve to shards next to source files.
func TestRenderTour_ShardLinkRelative(t *testing.T) {
	nodes := []api.Node{
		{ID: "f:src/a.ts", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/a.ts"}},
	}
	cache := NewCache()
	cache.Build(&api.ShardIR{Graph: api.ShardGraph{Nodes: nodes}})

	body := RenderTour(cache, TopoStrategy{}, "..")

	if !strings.Contains(body, "../src/a.graph.ts") {
		t.Errorf("expected relative shard link '../src/a.graph.ts' in body:\n%s", body)
	}
}

// TestBFSSeedStrategy_ReachableOnly verifies BFS from seed only emits files
// reachable by walking the undirected import graph.
func TestBFSSeedStrategy_ReachableOnly(t *testing.T) {
	// Reachable: a↔b↔c. Unreachable: z.
	nodes := []api.Node{
		{ID: "f:a.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "a.go"}},
		{ID: "f:b.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "b.go"}},
		{ID: "f:c.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "c.go"}},
		{ID: "f:z.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "z.go"}},
	}
	rels := []api.Relationship{
		{ID: "r1", Type: "imports", StartNode: "f:a.go", EndNode: "f:b.go"},
		{ID: "r2", Type: "imports", StartNode: "f:b.go", EndNode: "f:c.go"},
	}
	cache := NewCache()
	cache.Build(&api.ShardIR{Graph: api.ShardGraph{Nodes: nodes, Relationships: rels}})

	order := BFSSeedStrategy{Seed: "a.go"}.Order(cache)

	if len(order) != 3 {
		t.Fatalf("BFS should reach 3 files (a,b,c), got %v", order)
	}
	if order[0] != "a.go" {
		t.Errorf("seed must come first, got %v", order)
	}
	for _, f := range order {
		if f == "z.go" {
			t.Errorf("unreachable z.go leaked into BFS: %v", order)
		}
	}
}

// TestBFSSeedStrategy_MissingSeed returns nil for a seed not in the cache.
func TestBFSSeedStrategy_MissingSeed(t *testing.T) {
	cache := NewCache()
	cache.Build(&api.ShardIR{})
	if got := (BFSSeedStrategy{Seed: "nonexistent.go"}).Order(cache); len(got) != 0 {
		t.Errorf("missing seed should yield empty order, got %v", got)
	}
}

// TestDFSSeedStrategy_DifferentFromBFS verifies DFS produces a different order
// than BFS on a branching graph (proving we're actually doing DFS).
func TestDFSSeedStrategy_DifferentFromBFS(t *testing.T) {
	// Star from root to a,b,c,d. BFS sees them at depth 1; DFS descends first.
	// With linearly-chained children we can actually see the difference.
	// Build: root → x → leaf_x; root → y → leaf_y
	nodes := []api.Node{
		{ID: "f:root.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "root.go"}},
		{ID: "f:x.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "x.go"}},
		{ID: "f:leaf_x.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "leaf_x.go"}},
		{ID: "f:y.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "y.go"}},
		{ID: "f:leaf_y.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "leaf_y.go"}},
	}
	rels := []api.Relationship{
		{ID: "r1", Type: "imports", StartNode: "f:root.go", EndNode: "f:x.go"},
		{ID: "r2", Type: "imports", StartNode: "f:root.go", EndNode: "f:y.go"},
		{ID: "r3", Type: "imports", StartNode: "f:x.go", EndNode: "f:leaf_x.go"},
		{ID: "r4", Type: "imports", StartNode: "f:y.go", EndNode: "f:leaf_y.go"},
	}
	cache := NewCache()
	cache.Build(&api.ShardIR{Graph: api.ShardGraph{Nodes: nodes, Relationships: rels}})

	bfs := BFSSeedStrategy{Seed: "root.go"}.Order(cache)
	dfs := DFSSeedStrategy{Seed: "root.go"}.Order(cache)

	// BFS should see root, x, y (depth 1) before any leaves (depth 2).
	bfsIdx := map[string]int{}
	for i, f := range bfs {
		bfsIdx[f] = i
	}
	if bfsIdx["leaf_x.go"] < bfsIdx["y.go"] || bfsIdx["leaf_y.go"] < bfsIdx["x.go"] {
		t.Errorf("BFS should visit depth-1 before depth-2: %v", bfs)
	}

	// DFS should descend all the way down one branch before the other.
	// After root, it visits x, then leaf_x, before crossing to y.
	dfsIdx := map[string]int{}
	for i, f := range dfs {
		dfsIdx[f] = i
	}
	if dfsIdx["leaf_x.go"] > dfsIdx["y.go"] && dfsIdx["leaf_y.go"] > dfsIdx["x.go"] {
		t.Errorf("DFS should descend one branch fully before the other: %v", dfs)
	}
}

// TestCentralityStrategy_MostDependedFirst verifies centrality orders by
// transitive-dependent count descending.
func TestCentralityStrategy_MostDependedFirst(t *testing.T) {
	// util is a leaf depended on by lib and main. lib is depended on by main.
	// Transitive-dependent counts: util=2, lib=1, main=0.
	nodes := []api.Node{
		{ID: "f:util.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "util.go"}},
		{ID: "f:lib.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "lib.go"}},
		{ID: "f:main.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "main.go"}},
	}
	rels := []api.Relationship{
		{ID: "r1", Type: "imports", StartNode: "f:lib.go", EndNode: "f:util.go"},
		{ID: "r2", Type: "imports", StartNode: "f:main.go", EndNode: "f:lib.go"},
		{ID: "r3", Type: "imports", StartNode: "f:main.go", EndNode: "f:util.go"},
	}
	cache := NewCache()
	cache.Build(&api.ShardIR{Graph: api.ShardGraph{Nodes: nodes, Relationships: rels}})

	order := CentralityStrategy{}.Order(cache)

	if len(order) != 3 {
		t.Fatalf("expected 3 files, got %v", order)
	}
	if order[0] != "util.go" {
		t.Errorf("most-depended-on file should come first; got %v", order)
	}
	if order[2] != "main.go" {
		t.Errorf("least-depended-on file should come last; got %v", order)
	}
}

// TestChunkTour_SplitsAtEntryBoundaries verifies long tours get chunked into
// chapters that each stay within budget.
func TestChunkTour_SplitsAtEntryBoundaries(t *testing.T) {
	// Build a tour with 5 entries, then chunk at a budget small enough to force
	// multiple chapters but large enough to fit the preamble.
	nodes := make([]api.Node, 5)
	for i := range nodes {
		name := fmt.Sprintf("f%d.go", i)
		nodes[i] = api.Node{
			ID:         "f:" + name,
			Labels:     []string{"File"},
			Properties: map[string]any{"filePath": name},
		}
	}
	cache := NewCache()
	cache.Build(&api.ShardIR{Graph: api.ShardGraph{Nodes: nodes}})

	body := RenderTour(cache, TopoStrategy{}, "..")
	chapters := ChunkTour(body, 60) // small budget

	if len(chapters) < 2 {
		t.Fatalf("expected multiple chapters at small budget, got %d", len(chapters))
	}
	for i, ch := range chapters {
		if !strings.Contains(ch, "Chapter") {
			t.Errorf("chapter %d missing 'Chapter' header: %s", i+1, ch)
		}
	}
	// First chapter has "next" link, last has "prev" link.
	if !strings.Contains(chapters[0], "next") {
		t.Errorf("first chapter missing next link: %s", chapters[0])
	}
	if !strings.Contains(chapters[len(chapters)-1], "prev") {
		t.Errorf("last chapter missing prev link: %s", chapters[len(chapters)-1])
	}
}

// TestChunkTour_FitsInBudget returns a single chunk when body fits.
func TestChunkTour_FitsInBudget(t *testing.T) {
	chapters := ChunkTour("short body", 10000)
	if len(chapters) != 1 {
		t.Errorf("short body should produce 1 chunk, got %d", len(chapters))
	}
}

// TestResolveStrategy_ValidNames checks all strategy names resolve.
func TestResolveStrategy_ValidNames(t *testing.T) {
	cases := []struct {
		name    string
		seed    string
		wantErr bool
	}{
		{"topo", "", false},
		{"", "", false},
		{"bfs-seed", "foo.go", false},
		{"bfs-seed", "", true},
		{"dfs-seed", "foo.go", false},
		{"dfs-seed", "", true},
		{"centrality", "", false},
		{"nonsense", "", true},
	}
	for _, tc := range cases {
		_, err := ResolveStrategy(tc.name, tc.seed)
		if (err != nil) != tc.wantErr {
			t.Errorf("ResolveStrategy(%q, %q): wantErr=%v got %v", tc.name, tc.seed, tc.wantErr, err)
		}
	}
}

// TestRenderNarrative_ContainsKeyInfo verifies the narrative covers domain,
// imports, importers, functions, and risk.
func TestRenderNarrative_ContainsKeyInfo(t *testing.T) {
	nodes := []api.Node{
		{ID: "f:src/a.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/a.go"}},
		{ID: "f:src/b.go", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/b.go"}},
		{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "doWork", "filePath": "src/a.go"}},
		{ID: "fn2", Labels: []string{"Function"}, Properties: map[string]any{"name": "helper", "filePath": "src/a.go"}},
	}
	rels := []api.Relationship{
		{ID: "r1", Type: "imports", StartNode: "f:src/a.go", EndNode: "f:src/b.go"},
		{ID: "r2", Type: "calls", StartNode: "fn1", EndNode: "fn2"},
	}
	cache := NewCache()
	cache.Build(&api.ShardIR{Graph: api.ShardGraph{Nodes: nodes, Relationships: rels}})
	cache.FileDomain["src/a.go"] = "Core/Utils"

	got := RenderNarrative("src/a.go", cache, "//")

	wantSubstrings := []string{
		"Narrative:",
		"Core",
		"Utils",
		"imports",
		"doWork",
		"helper",
		"Risk:",
	}
	for _, s := range wantSubstrings {
		if !strings.Contains(got, s) {
			t.Errorf("narrative missing %q\n---\n%s", s, got)
		}
	}
	// Should use comment prefix on each line.
	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if !strings.HasPrefix(line, "//") {
			t.Errorf("narrative line missing comment prefix: %q", line)
		}
	}
}

// TestRenderAll_Narrate prepends the narrative preamble when narrate=true.
func TestRenderAll_Narrate(t *testing.T) {
	dir := t.TempDir()
	nodes := []api.Node{
		{ID: "fa", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/a.go"}},
		{ID: "fb", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/b.go"}},
	}
	rels := []api.Relationship{
		{ID: "r1", Type: "imports", StartNode: "fa", EndNode: "fb"},
	}
	cache := NewCache()
	cache.Build(&api.ShardIR{Graph: api.ShardGraph{Nodes: nodes, Relationships: rels}})

	n, err := RenderAll(dir, cache, []string{"src/a.go"}, true, false)
	if err != nil || n != 1 {
		t.Fatalf("RenderAll narrate: n=%d err=%v", n, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "src", "a.graph.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Narrative:") {
		t.Errorf("shard missing narrative when narrate=true:\n%s", data)
	}
}
