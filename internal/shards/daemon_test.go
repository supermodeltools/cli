package shards

import (
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

// ── helpers ─────────────────────────────────────────────────────────────────

func buildIR(nodes []api.Node, rels []api.Relationship) *api.ShardIR {
	return &api.ShardIR{
		Graph: api.ShardGraph{
			Nodes:         nodes,
			Relationships: rels,
		},
	}
}

func newNode(id string, labels []string, props ...string) api.Node {
	p := make(map[string]any)
	for i := 0; i+1 < len(props); i += 2 {
		p[props[i]] = props[i+1]
	}
	return api.Node{ID: id, Labels: labels, Properties: p}
}

func newRel(id, typ, start, end string) api.Relationship {
	return api.Relationship{ID: id, Type: typ, StartNode: start, EndNode: end}
}

func nodeIDSet(ir *api.ShardIR) map[string]bool {
	m := make(map[string]bool, len(ir.Graph.Nodes))
	for _, n := range ir.Graph.Nodes {
		m[n.ID] = true
	}
	return m
}

func hasRelEdge(ir *api.ShardIR, start, end string) bool {
	for _, r := range ir.Graph.Relationships {
		if r.StartNode == start && r.EndNode == end {
			return true
		}
	}
	return false
}

// ── merge tests ─────────────────────────────────────────────────────────────

func TestMergeGraph_BasicMerge(t *testing.T) {
	existing := buildIR(
		[]api.Node{
			newNode("file-a", []string{"File"}, "filePath", "/repo/a.go"),
			newNode("fn-a1", []string{"Function"}, "filePath", "/repo/a.go", "name", "Foo"),
		},
		[]api.Relationship{newRel("r1", "DEFINES", "file-a", "fn-a1")},
	)
	incremental := buildIR(
		[]api.Node{
			newNode("file-b", []string{"File"}, "filePath", "/repo/b.go"),
			newNode("fn-b1", []string{"Function"}, "filePath", "/repo/b.go", "name", "Bar"),
		},
		[]api.Relationship{newRel("r2", "DEFINES", "file-b", "fn-b1")},
	)

	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/b.go"})

	result := d.GetIR()
	ids := nodeIDSet(result)
	for _, want := range []string{"file-a", "fn-a1", "file-b", "fn-b1"} {
		if !ids[want] {
			t.Errorf("expected node %q after basic merge", want)
		}
	}
	if len(result.Graph.Nodes) != 4 {
		t.Errorf("expected 4 nodes, got %d", len(result.Graph.Nodes))
	}
}

func TestMergeGraph_FileReplacement(t *testing.T) {
	existing := buildIR(
		[]api.Node{
			newNode("file-a-old", []string{"File"}, "filePath", "/repo/a.go"),
			newNode("fn-old", []string{"Function"}, "filePath", "/repo/a.go", "name", "Foo"),
			newNode("file-b", []string{"File"}, "filePath", "/repo/b.go"),
		},
		[]api.Relationship{newRel("r1", "DEFINES", "file-a-old", "fn-old")},
	)
	incremental := buildIR(
		[]api.Node{
			newNode("file-a-new", []string{"File"}, "filePath", "/repo/a.go"),
			newNode("fn-new", []string{"Function"}, "filePath", "/repo/a.go", "name", "Foo"),
		},
		[]api.Relationship{newRel("r2", "DEFINES", "file-a-new", "fn-new")},
	)

	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/a.go"})

	result := d.GetIR()
	ids := nodeIDSet(result)

	if ids["file-a-old"] {
		t.Error("old File node for a.go should have been removed")
	}
	if ids["fn-old"] {
		t.Error("old Function node for a.go should have been removed")
	}
	if !ids["file-a-new"] {
		t.Error("new File node for a.go should be present")
	}
	if !ids["fn-new"] {
		t.Error("new Function node for a.go should be present")
	}
	if !ids["file-b"] {
		t.Error("unrelated file-b node should still be present")
	}
}

func TestMergeGraph_UUIDRemapping(t *testing.T) {
	existing := buildIR(
		[]api.Node{
			newNode("file-a-old", []string{"File"}, "filePath", "/repo/a.go"),
			newNode("fn-foo-old", []string{"Function"}, "filePath", "/repo/a.go", "name", "Foo"),
			newNode("file-b", []string{"File"}, "filePath", "/repo/b.go"),
			newNode("fn-bar", []string{"Function"}, "filePath", "/repo/b.go", "name", "Bar"),
		},
		[]api.Relationship{
			newRel("r1", "DEFINES", "file-a-old", "fn-foo-old"),
			newRel("r2", "DEFINES", "file-b", "fn-bar"),
			newRel("r3", "CALLS", "fn-bar", "fn-foo-old"),
		},
	)
	incremental := buildIR(
		[]api.Node{
			newNode("file-a-new", []string{"File"}, "filePath", "/repo/a.go"),
			newNode("fn-foo-new", []string{"Function"}, "filePath", "/repo/a.go", "name", "Foo"),
		},
		[]api.Relationship{newRel("r4", "DEFINES", "file-a-new", "fn-foo-new")},
	)

	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/a.go"})

	result := d.GetIR()
	if !hasRelEdge(result, "fn-bar", "fn-foo-new") {
		t.Error("CALLS relationship should be remapped: fn-bar → fn-foo-new")
	}
	if hasRelEdge(result, "fn-bar", "fn-foo-old") {
		t.Error("CALLS relationship should not still reference stale fn-foo-old")
	}
}

func TestMergeGraph_ExternalDependencyResolution(t *testing.T) {
	existing := buildIR(
		[]api.Node{
			newNode("file-button", []string{"File"}, "filePath", "/repo/web/src/components/ui/button.tsx"),
			newNode("file-main", []string{"File"}, "filePath", "/repo/main.go"),
		},
		nil,
	)
	incremental := buildIR(
		[]api.Node{
			newNode("file-main-new", []string{"File"}, "filePath", "/repo/main.go"),
			newNode("ext-button", []string{"LocalDependency"}, "importPath", "@/components/ui/button"),
		},
		[]api.Relationship{newRel("r1", "IMPORTS", "file-main-new", "ext-button")},
	)

	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/main.go"})

	result := d.GetIR()
	ids := nodeIDSet(result)

	if ids["ext-button"] {
		t.Error("resolved LocalDependency placeholder should not appear in merged graph")
	}
	if !ids["file-button"] {
		t.Error("real file-button node must remain in merged graph")
	}
	if !hasRelEdge(result, "file-main-new", "file-button") {
		t.Error("IMPORTS relationship should be remapped to the real file-button node")
	}
}

func TestMergeGraph_EmptyIncremental(t *testing.T) {
	existing := buildIR(
		[]api.Node{
			newNode("file-a", []string{"File"}, "filePath", "/repo/a.go"),
			newNode("fn-a1", []string{"Function"}, "filePath", "/repo/a.go", "name", "Alpha"),
		},
		[]api.Relationship{newRel("r1", "DEFINES", "file-a", "fn-a1")},
	)

	d := NewTestDaemon(existing)
	d.MergeGraph(buildIR(nil, nil), []string{"/repo/a.go"})

	if result := d.GetIR(); result == nil {
		t.Fatal("GetIR() returned nil after empty incremental merge")
	}
}

func TestMergeGraph_IncrementalNoMatchingExisting(t *testing.T) {
	existing := buildIR(
		[]api.Node{newNode("file-a", []string{"File"}, "filePath", "/repo/a.go")},
		nil,
	)
	incremental := buildIR(
		[]api.Node{
			newNode("file-c", []string{"File"}, "filePath", "/repo/c.go"),
			newNode("fn-c1", []string{"Function"}, "filePath", "/repo/c.go", "name", "NewFunc"),
		},
		[]api.Relationship{newRel("r1", "DEFINES", "file-c", "fn-c1")},
	)

	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/c.go"})

	result := d.GetIR()
	ids := nodeIDSet(result)

	if !ids["file-c"] {
		t.Error("file-c should be added to the graph")
	}
	if !ids["fn-c1"] {
		t.Error("fn-c1 should be added to the graph")
	}
	if !ids["file-a"] {
		t.Error("file-a should remain untouched")
	}
	if !hasRelEdge(result, "file-c", "fn-c1") {
		t.Error("DEFINES relationship for new file should be present")
	}
}

func TestMergeGraph_NilExisting(t *testing.T) {
	d := NewTestDaemon(nil)
	incremental := buildIR(
		[]api.Node{newNode("file-a", []string{"File"}, "filePath", "/repo/a.go")},
		nil,
	)

	d.MergeGraph(incremental, []string{"/repo/a.go"})

	result := d.GetIR()
	if result == nil {
		t.Fatal("expected non-nil ShardIR after merging into nil daemon")
	}
	if !nodeIDSet(result)["file-a"] {
		t.Error("file-a should be present when merging into nil existing graph")
	}
}

// ── domain preservation tests ───────────────────────────────────────────────

func TestMergeGraph_DomainsPreservedOnIncremental(t *testing.T) {
	existing := &api.ShardIR{
		Graph: api.ShardGraph{
			Nodes: []api.Node{newNode("file-a", []string{"File"}, "filePath", "/repo/a.go")},
		},
		Domains: []api.ShardDomain{
			{Name: "CommandCLI"},
			{Name: "ApiClient"},
			{Name: "SharedKernel"},
		},
	}
	incremental := &api.ShardIR{
		Graph: api.ShardGraph{
			Nodes: []api.Node{newNode("file-b", []string{"File"}, "filePath", "/repo/b.go")},
		},
		Domains: []api.ShardDomain{
			{Name: "LocalCache"}, // garbage domain from 1-file classification
		},
	}

	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/b.go"})

	result := d.GetIR()
	if len(result.Domains) != 3 {
		t.Fatalf("expected 3 domains preserved, got %d", len(result.Domains))
	}
	names := make(map[string]bool)
	for _, dom := range result.Domains {
		names[dom.Name] = true
	}
	for _, want := range []string{"CommandCLI", "ApiClient", "SharedKernel"} {
		if !names[want] {
			t.Errorf("domain %q should be preserved, got domains: %v", want, result.Domains)
		}
	}
}

func TestMergeGraph_DeletedFilePrunesRelationships(t *testing.T) {
	// When a file is deleted, its nodes are removed from the graph.
	// Relationships referencing those deleted nodes must also be pruned.
	existing := buildIR(
		[]api.Node{
			newNode("file-a", []string{"File"}, "filePath", "/repo/a.go"),
			newNode("fn-a", []string{"Function"}, "filePath", "/repo/a.go", "name", "Foo"),
			newNode("file-b", []string{"File"}, "filePath", "/repo/b.go"),
			newNode("fn-b", []string{"Function"}, "filePath", "/repo/b.go", "name", "Bar"),
		},
		[]api.Relationship{
			newRel("r1", "DEFINES", "file-a", "fn-a"),
			newRel("r2", "DEFINES", "file-b", "fn-b"),
			newRel("r3", "CALLS", "fn-b", "fn-a"), // b.go calls a.go — should be pruned when a.go deleted
		},
	)
	// Incremental has no nodes for a.go (it was deleted).
	incremental := buildIR(nil, nil)

	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/a.go"})

	result := d.GetIR()
	ids := nodeIDSet(result)

	if ids["file-a"] || ids["fn-a"] {
		t.Error("nodes for deleted a.go should be removed")
	}
	if !ids["file-b"] || !ids["fn-b"] {
		t.Error("nodes for b.go should remain")
	}
	if hasRelEdge(result, "fn-b", "fn-a") {
		t.Error("CALLS rel referencing deleted fn-a should be pruned")
	}
	if hasRelEdge(result, "file-a", "fn-a") {
		t.Error("DEFINES rel for deleted a.go should be pruned")
	}
}

func TestMergeGraph_DomainsPreservedEvenWhenIncrementalHasMore(t *testing.T) {
	existing := &api.ShardIR{
		Graph: api.ShardGraph{
			Nodes: []api.Node{newNode("file-a", []string{"File"}, "filePath", "/repo/a.go")},
		},
		Domains: []api.ShardDomain{{Name: "Original"}},
	}
	incremental := &api.ShardIR{
		Graph: api.ShardGraph{
			Nodes: []api.Node{newNode("file-b", []string{"File"}, "filePath", "/repo/b.go")},
		},
		Domains: []api.ShardDomain{
			{Name: "New1"},
			{Name: "New2"},
			{Name: "New3"},
		},
	}

	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/b.go"})

	result := d.GetIR()
	if len(result.Domains) != 1 || result.Domains[0].Name != "Original" {
		t.Errorf("expected original domain preserved, got %v", result.Domains)
	}
}

// ── computeAffectedFiles tests ───────────────────────────────────────────────

// TestComputeAffectedFiles_OldCalleeIncluded verifies that when a function in a
// changed file used to call a function in another file, that other file is
// included in the affected set even if the call was removed by the update.
//
// Before the fix, computeAffectedFiles only walked d.cache.Callers (new state)
// and oldImports. It never consulted pre-merge callee relationships. As a result,
// if funcA stopped calling funcB, file B was not marked affected and its shard
// kept the stale "funcB ← funcA" line.
func TestComputeAffectedFiles_OldCalleeIncluded(t *testing.T) {
	ir := buildIR(
		[]api.Node{
			newNode("file-a", []string{"File"}, "filePath", "a.go"),
			newNode("fn-a", []string{"Function"}, "filePath", "a.go", "name", "FuncA"),
			newNode("file-b", []string{"File"}, "filePath", "b.go"),
			newNode("fn-b", []string{"Function"}, "filePath", "b.go", "name", "FuncB"),
		},
		[]api.Relationship{
			// FuncA calls FuncB (before the incremental update).
			newRel("calls-1", "calls", "fn-a", "fn-b"),
		},
	)
	d := NewTestDaemon(ir)
	d.cache = NewCache()
	d.cache.Build(ir)

	// Simulate pre-merge callees snapshot: fn-a used to call fn-b (in b.go).
	oldCalleeFiles := map[string][]string{
		"fn-a": {"b.go"},
	}

	// After the incremental update, FuncA no longer calls FuncB, so the new
	// cache has no callee relationship. Only a.go is in changedFiles.
	affected := d.computeAffectedFiles([]string{"a.go"}, nil, oldCalleeFiles)

	affectedSet := make(map[string]bool, len(affected))
	for _, f := range affected {
		affectedSet[f] = true
	}

	if !affectedSet["a.go"] {
		t.Error("expected a.go (changed file) in affected set")
	}
	if !affectedSet["b.go"] {
		t.Error("expected b.go (old callee) in affected set — shard needs to drop stale '← FuncA' entry")
	}
}

// TestComputeAffectedFiles_CurrentCallersIncluded verifies that files currently
// calling a function in the changed file are marked affected (existing behaviour).
func TestComputeAffectedFiles_CurrentCallersIncluded(t *testing.T) {
	ir := buildIR(
		[]api.Node{
			newNode("file-a", []string{"File"}, "filePath", "a.go"),
			newNode("fn-a", []string{"Function"}, "filePath", "a.go", "name", "FuncA"),
			newNode("file-c", []string{"File"}, "filePath", "c.go"),
			newNode("fn-c", []string{"Function"}, "filePath", "c.go", "name", "FuncC"),
		},
		[]api.Relationship{
			// FuncC (in c.go) calls FuncA (in a.go).
			newRel("calls-2", "calls", "fn-c", "fn-a"),
		},
	)
	d := NewTestDaemon(ir)
	d.cache = NewCache()
	d.cache.Build(ir)

	affected := d.computeAffectedFiles([]string{"a.go"}, nil, nil)

	affectedSet := make(map[string]bool, len(affected))
	for _, f := range affected {
		affectedSet[f] = true
	}

	if !affectedSet["c.go"] {
		t.Error("expected c.go (current caller) in affected set")
	}
}
