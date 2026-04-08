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
