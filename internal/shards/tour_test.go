package shards

import (
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
