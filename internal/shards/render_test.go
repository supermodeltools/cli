package shards

import (
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

// makeRenderCache builds a Cache from a ShardIR for render tests.
func makeRenderCache(ir *api.ShardIR) *Cache {
	c := NewCache()
	c.Build(ir)
	return c
}

func shardIR(nodes []api.Node, rels []api.Relationship) *api.ShardIR {
	return &api.ShardIR{
		Graph: api.ShardGraph{
			Nodes:         nodes,
			Relationships: rels,
		},
	}
}

// TestRenderCallsSection_Deterministic verifies that renderCallsSection produces
// the same output regardless of the order relationships were appended to the cache.
// This catches non-determinism from map iteration or relationship ordering in the
// API response, which would cause unnecessary shard file rewrites on each run.
func TestRenderCallsSection_Deterministic(t *testing.T) {
	nodes := []api.Node{
		{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "handle", "filePath": "src/a.go"}},
		{ID: "fn2", Labels: []string{"Function"}, Properties: map[string]any{"name": "parse", "filePath": "src/b.go"}},
		{ID: "fn3", Labels: []string{"Function"}, Properties: map[string]any{"name": "validate", "filePath": "src/c.go"}},
	}

	// Build two equivalent graphs with relationships in reversed order.
	// If rendering is deterministic, both must produce identical output.
	ir1 := shardIR(nodes, []api.Relationship{
		{ID: "r1", Type: "calls", StartNode: "fn2", EndNode: "fn1"},
		{ID: "r2", Type: "calls", StartNode: "fn3", EndNode: "fn1"},
	})
	ir2 := shardIR(nodes, []api.Relationship{
		{ID: "r2", Type: "calls", StartNode: "fn3", EndNode: "fn1"},
		{ID: "r1", Type: "calls", StartNode: "fn2", EndNode: "fn1"},
	})

	c1 := makeRenderCache(ir1)
	c2 := makeRenderCache(ir2)

	out1 := renderCallsSection("src/a.go", c1, "//")
	out2 := renderCallsSection("src/a.go", c2, "//")

	if out1 != out2 {
		t.Errorf("renderCallsSection output differs based on relationship order:\ngot1:\n%s\ngot2:\n%s", out1, out2)
	}
}

// TestRenderCalleesSection_Deterministic mirrors TestRenderCallsSection_Deterministic
// but targets the callee path: a single caller with multiple callees whose relationships
// appear in reversed order must produce identical output.
func TestRenderCalleesSection_Deterministic(t *testing.T) {
	nodes := []api.Node{
		{ID: "fn_caller", Labels: []string{"Function"}, Properties: map[string]any{"name": "dispatch", "filePath": "src/a.go"}},
		{ID: "fn_c1", Labels: []string{"Function"}, Properties: map[string]any{"name": "alpha", "filePath": "src/b.go"}},
		{ID: "fn_c2", Labels: []string{"Function"}, Properties: map[string]any{"name": "beta", "filePath": "src/c.go"}},
	}

	ir1 := shardIR(nodes, []api.Relationship{
		{ID: "r1", Type: "calls", StartNode: "fn_caller", EndNode: "fn_c1"},
		{ID: "r2", Type: "calls", StartNode: "fn_caller", EndNode: "fn_c2"},
	})
	ir2 := shardIR(nodes, []api.Relationship{
		{ID: "r2", Type: "calls", StartNode: "fn_caller", EndNode: "fn_c2"},
		{ID: "r1", Type: "calls", StartNode: "fn_caller", EndNode: "fn_c1"},
	})

	c1 := makeRenderCache(ir1)
	c2 := makeRenderCache(ir2)

	out1 := renderCallsSection("src/a.go", c1, "//")
	out2 := renderCallsSection("src/a.go", c2, "//")

	if out1 != out2 {
		t.Errorf("callee output differs based on relationship order:\ngot1:\n%s\ngot2:\n%s", out1, out2)
	}
}

// TestRenderCallsSection_SameNameFunctions ensures that two functions with the same
// name (but different IDs, e.g. methods on different types) are ordered by ID when
// they share a name, preventing non-determinism from the unstable sort.
func TestRenderCallsSection_SameNameFunctions(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			{ID: "fn_a", Labels: []string{"Function"}, Properties: map[string]any{"name": "String", "filePath": "src/types.go"}},
			{ID: "fn_b", Labels: []string{"Function"}, Properties: map[string]any{"name": "String", "filePath": "src/types.go"}},
			{ID: "caller1", Labels: []string{"Function"}, Properties: map[string]any{"name": "callA", "filePath": "src/other.go"}},
			{ID: "caller2", Labels: []string{"Function"}, Properties: map[string]any{"name": "callB", "filePath": "src/other.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "calls", StartNode: "caller1", EndNode: "fn_a"},
			{ID: "r2", Type: "calls", StartNode: "caller2", EndNode: "fn_b"},
		},
	)

	c := makeRenderCache(ir)

	// Run renderCallsSection multiple times to detect non-determinism
	first := renderCallsSection("src/types.go", c, "//")
	for i := 0; i < 10; i++ {
		out := renderCallsSection("src/types.go", c, "//")
		if out != first {
			t.Errorf("renderCallsSection is non-deterministic (run %d differs from run 0):\nfirst:\n%s\nlater:\n%s", i+1, first, out)
		}
	}
}

// TestRenderCallsSection_ContainsCallerAndCallee verifies basic content correctness.
func TestRenderCallsSection_ContainsCallerAndCallee(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			{ID: "fn_target", Labels: []string{"Function"}, Properties: map[string]any{"name": "processRequest", "filePath": "src/handler.go"}},
			{ID: "fn_caller", Labels: []string{"Function"}, Properties: map[string]any{"name": "main", "filePath": "src/main.go"}},
			{ID: "fn_callee", Labels: []string{"Function"}, Properties: map[string]any{"name": "validate", "filePath": "src/util.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "calls", StartNode: "fn_caller", EndNode: "fn_target"},
			{ID: "r2", Type: "calls", StartNode: "fn_target", EndNode: "fn_callee"},
		},
	)

	c := makeRenderCache(ir)
	out := renderCallsSection("src/handler.go", c, "//")

	if out == "" {
		t.Fatal("expected non-empty output for function with caller and callee")
	}
	if !strings.Contains(out, "[calls]") {
		t.Errorf("should contain [calls] header:\n%s", out)
	}
	if !strings.Contains(out, "processRequest ← main") {
		t.Errorf("should show caller relationship:\n%s", out)
	}
	if !strings.Contains(out, "processRequest → validate") {
		t.Errorf("should show callee relationship:\n%s", out)
	}
}

// TestRenderCallsSection_EmptyWhenNoCallRelationships returns empty for a file
// with functions that have no callers or callees.
func TestRenderCallsSection_EmptyWhenNoCallRelationships(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "isolated", "filePath": "src/a.go"}},
		},
		nil,
	)
	c := makeRenderCache(ir)
	out := renderCallsSection("src/a.go", c, "//")
	if out != "" {
		t.Errorf("expected empty output for function with no call relationships, got:\n%s", out)
	}
}
