package shards

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
	repocache "github.com/supermodeltools/cli/internal/cache"
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

// TestMergeGraph_NoDependencyPath covers L407: a LocalDependency with no filePath,
// name, or importPath is skipped (fp stays "").
func TestMergeGraph_NoDependencyPath(t *testing.T) {
	existing := buildIR(
		[]api.Node{newNode("file-a", []string{"File"}, "filePath", "/repo/a.go")},
		nil,
	)
	incremental := buildIR(
		[]api.Node{
			newNode("file-a-new", []string{"File"}, "filePath", "/repo/a.go"),
			// LocalDependency with no path properties → fp == "" → skip
			newNode("dep-nopath", []string{"LocalDependency"}),
		},
		[]api.Relationship{newRel("r1", "IMPORTS", "file-a-new", "dep-nopath")},
	)
	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/a.go"})

	result := d.GetIR()
	// dep-nopath should remain (not resolved) since it has no path to match
	ids := nodeIDSet(result)
	if !ids["dep-nopath"] {
		t.Error("dep-nopath with no path should remain in the merged graph (unresolved)")
	}
}

// TestMergeGraph_ExactFilepathMatch covers L411: a LocalDependency whose fp
// exactly matches an existing file's filePath gets resolved to that node.
func TestMergeGraph_ExactFilepathMatch(t *testing.T) {
	existing := buildIR(
		[]api.Node{
			newNode("file-util", []string{"File"}, "filePath", "/repo/util.go"),
			newNode("file-main", []string{"File"}, "filePath", "/repo/main.go"),
		},
		nil,
	)
	incremental := buildIR(
		[]api.Node{
			newNode("file-main-new", []string{"File"}, "filePath", "/repo/main.go"),
			// importPath exactly matches existing file's filePath → L411 is taken
			newNode("dep-util", []string{"LocalDependency"}, "importPath", "/repo/util.go"),
		},
		[]api.Relationship{newRel("r1", "IMPORTS", "file-main-new", "dep-util")},
	)
	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/main.go"})

	result := d.GetIR()
	// dep-util should be resolved to file-util; rel should point to file-util
	if hasRelEdge(result, "file-main-new", "file-util") {
		// resolved successfully
	} else {
		t.Error("dep-util should be resolved to existing file-util via exact path match")
	}
}

// TestMergeGraph_TildeImportPath covers L420: importPath with "~/" prefix is
// stripped before suffix matching.
func TestMergeGraph_TildeImportPath(t *testing.T) {
	existing := buildIR(
		[]api.Node{
			newNode("file-utils", []string{"File"}, "filePath", "/repo/src/utils.ts"),
			newNode("file-main", []string{"File"}, "filePath", "/repo/main.ts"),
		},
		nil,
	)
	incremental := buildIR(
		[]api.Node{
			newNode("file-main-new", []string{"File"}, "filePath", "/repo/main.ts"),
			// "~/" prefix → stripped → "src/utils" → suffix-matched to /repo/src/utils.ts
			newNode("dep-tilde", []string{"LocalDependency"}, "importPath", "~/src/utils"),
		},
		[]api.Relationship{newRel("r1", "IMPORTS", "file-main-new", "dep-tilde")},
	)
	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/main.ts"})

	result := d.GetIR()
	ids := nodeIDSet(result)
	if ids["dep-tilde"] {
		t.Error("dep-tilde should be resolved (remapped to file-utils)")
	}
}

// TestMergeGraph_ExtRemapStartNode covers L546: a relationship whose StartNode
// is in extRemap gets its StartNode remapped.
func TestMergeGraph_ExtRemapStartNode(t *testing.T) {
	existing := buildIR(
		[]api.Node{
			newNode("file-db", []string{"File"}, "filePath", "/repo/db.go"),
			newNode("file-handler", []string{"File"}, "filePath", "/repo/handler.go"),
		},
		nil,
	)
	incremental := buildIR(
		[]api.Node{
			newNode("file-handler-new", []string{"File"}, "filePath", "/repo/handler.go"),
			// importPath exactly matches existing db.go → resolved to file-db
			newNode("dep-db", []string{"LocalDependency"}, "importPath", "/repo/db.go"),
			// A node that dep-db "calls" — so StartNode of the rel is dep-db
			newNode("fn-connect", []string{"Function"}, "filePath", "/repo/db.go", "name", "Connect"),
		},
		[]api.Relationship{
			// dep-db is the StartNode → extRemap[dep-db] = file-db → L546 triggered
			newRel("r1", "IMPORTS", "dep-db", "fn-connect"),
		},
	)
	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/handler.go"})

	result := d.GetIR()
	// rel StartNode should have been remapped from dep-db to file-db
	if !hasRelEdge(result, "file-db", "fn-connect") {
		t.Error("relationship StartNode should be remapped from dep-db to file-db via extRemap")
	}
}

// TestMergeGraph_ExistingNodeIDCollision covers L494: an existing node whose ID
// also appears in the incremental graph is dropped from keptNodes.
func TestMergeGraph_ExistingNodeIDCollision(t *testing.T) {
	existing := buildIR(
		[]api.Node{
			newNode("file-a", []string{"File"}, "filePath", "/repo/a.go"),
			// fn-shared is in existing AND in incremental with the same ID
			newNode("fn-shared", []string{"Function"}, "filePath", "/repo/a.go", "name", "SharedFn"),
		},
		nil,
	)
	incremental := buildIR(
		[]api.Node{
			newNode("file-a-new", []string{"File"}, "filePath", "/repo/a.go"),
			// Same ID as the existing function → newNodeIDs["fn-shared"] = true
			newNode("fn-shared", []string{"Function"}, "filePath", "/repo/a.go", "name", "SharedFn"),
		},
		nil,
	)
	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/a.go"})
	// Should not panic or duplicate fn-shared
	result := d.GetIR()
	count := 0
	for _, n := range result.Graph.Nodes {
		if n.ID == "fn-shared" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("fn-shared should appear exactly once; got %d times", count)
	}
}

// TestMergeGraph_NodeWithPathProperty covers L469: existing nodes with "path"
// property (not "filePath") are still recognized when matching against changedSet.
func TestMergeGraph_NodeWithPathProperty(t *testing.T) {
	existing := buildIR(
		[]api.Node{
			// Uses "path" instead of "filePath" — covers the `fp = n.Prop("path")` fallback
			newNode("file-old", []string{"File"}, "path", "/repo/a.go"),
		},
		nil,
	)
	incremental := buildIR(
		[]api.Node{
			newNode("file-new", []string{"File"}, "filePath", "/repo/a.go"),
		},
		nil,
	)
	d := NewTestDaemon(existing)
	d.MergeGraph(incremental, []string{"/repo/a.go"})
	// Should not panic; node with "path" property in existing gets recognized
	result := d.GetIR()
	if result == nil {
		t.Fatal("MergeGraph with path-property node returned nil")
	}
}

// TestMergeGraph_ExistingNodeIDInUnchangedFile covers L494-495: when an
// existing node's ID appears in the incremental update but its file is NOT in
// changedFiles, the old copy is discarded so the incremental version wins.
func TestMergeGraph_ExistingNodeIDInUnchangedFile(t *testing.T) {
	existing := buildIR(
		[]api.Node{
			newNode("file-lib", []string{"File"}, "filePath", "/repo/lib.go"),
			// fn-lib exists in unchanged lib.go; same ID appears in incremental
			newNode("fn-lib", []string{"Function"}, "filePath", "/repo/lib.go", "name", "LibFn"),
			newNode("file-a", []string{"File"}, "filePath", "/repo/a.go"),
			newNode("fn-a", []string{"Function"}, "filePath", "/repo/a.go", "name", "AFn"),
		},
		nil,
	)
	// Incremental contains fn-lib (same ID) despite lib.go not being changed.
	incremental := buildIR(
		[]api.Node{
			newNode("file-a-new", []string{"File"}, "filePath", "/repo/a.go"),
			newNode("fn-a-new", []string{"Function"}, "filePath", "/repo/a.go", "name", "AFn"),
			newNode("fn-lib", []string{"Function"}, "filePath", "/repo/lib.go", "name", "LibFn"),
		},
		nil,
	)
	d := NewTestDaemon(existing)
	// Only a.go changed; lib.go is unchanged.
	d.MergeGraph(incremental, []string{"/repo/a.go"})
	result := d.GetIR()
	// fn-lib should appear exactly once (the incremental version).
	count := 0
	for _, n := range result.Graph.Nodes {
		if n.ID == "fn-lib" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("fn-lib should appear exactly once; got %d", count)
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
// ── assignNewFilesToDomains tests ────────────────────────────────────────────

func TestAssignNewFilesToDomains_EmptyDomains(t *testing.T) {
	// d.ir.Domains is nil → early return, no panic
	d := &Daemon{
		ir:   buildIR(nil, nil),
		logf: func(string, ...interface{}) {},
	}
	nodes := []api.Node{newNode("f1", []string{"File"}, "filePath", "/repo/new.go")}
	d.assignNewFilesToDomains(nodes) // must not panic
}

func TestAssignNewFilesToDomains_NonFileNodeSkipped(t *testing.T) {
	d := &Daemon{
		ir: &api.ShardIR{
			Domains: []api.ShardDomain{{Name: "Auth", KeyFiles: []string{"/repo/auth/login.go"}}},
		},
		logf: func(string, ...interface{}) {},
	}
	nodes := []api.Node{newNode("fn1", []string{"Function"}, "filePath", "/repo/auth/handler.go")}
	d.assignNewFilesToDomains(nodes)
	// Non-File node → domain KeyFiles unchanged
	if len(d.ir.Domains[0].KeyFiles) != 1 {
		t.Errorf("non-File node should not be added to domain; got %v", d.ir.Domains[0].KeyFiles)
	}
}

func TestAssignNewFilesToDomains_EmptyFilePathSkipped(t *testing.T) {
	d := &Daemon{
		ir: &api.ShardIR{
			Domains: []api.ShardDomain{{Name: "Core", KeyFiles: []string{"/repo/core/db.go"}}},
		},
		logf: func(string, ...interface{}) {},
	}
	// File node with no filePath property
	nodes := []api.Node{{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{}}}
	d.assignNewFilesToDomains(nodes)
	if len(d.ir.Domains[0].KeyFiles) != 1 {
		t.Errorf("File node without filePath should not be added; got %v", d.ir.Domains[0].KeyFiles)
	}
}

func TestAssignNewFilesToDomains_MatchesBestDomain(t *testing.T) {
	d := &Daemon{
		ir: &api.ShardIR{
			Domains: []api.ShardDomain{
				{Name: "Auth", KeyFiles: []string{"/repo/auth/login.go"}},
				{Name: "Web", KeyFiles: []string{"/repo/web/handler.go"}},
			},
		},
		logf: func(string, ...interface{}) {},
	}
	nodes := []api.Node{newNode("f1", []string{"File"}, "filePath", "/repo/auth/session.go")}
	d.assignNewFilesToDomains(nodes)
	// /repo/auth/session.go → prefix "/repo/auth" matches Auth domain
	if len(d.ir.Domains[0].KeyFiles) != 2 {
		t.Errorf("expected Auth domain to gain one file, got %v", d.ir.Domains[0].KeyFiles)
	}
	if len(d.ir.Domains[1].KeyFiles) != 1 {
		t.Errorf("Web domain should be unchanged, got %v", d.ir.Domains[1].KeyFiles)
	}
}

func TestAssignNewFilesToDomains_NoMatchingDomain(t *testing.T) {
	d := &Daemon{
		ir: &api.ShardIR{
			Domains: []api.ShardDomain{
				{Name: "Auth", KeyFiles: []string{"/repo/auth/login.go"}},
			},
		},
		logf: func(string, ...interface{}) {},
	}
	nodes := []api.Node{newNode("f1", []string{"File"}, "filePath", "/repo/other/service.go")}
	d.assignNewFilesToDomains(nodes)
	// /repo/other/ does not match /repo/auth/ prefix → no file added
	if len(d.ir.Domains[0].KeyFiles) != 1 {
		t.Errorf("unmatched file should not be added to domain, got %v", d.ir.Domains[0].KeyFiles)
	}
}

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

func TestComputeAffectedFiles_ImporterAndImportLoopBodies(t *testing.T) {
	// a.go imports b.go; c.go imports a.go.
	// Changing a.go should pull in both b.go (via Imports) and c.go (via Importers).
	ir := buildIR(
		[]api.Node{
			newNode("file-a", []string{"File"}, "filePath", "a.go"),
			newNode("file-b", []string{"File"}, "filePath", "b.go"),
			newNode("file-c", []string{"File"}, "filePath", "c.go"),
		},
		[]api.Relationship{
			// a.go imports b.go
			newRel("imp-ab", "imports", "file-a", "file-b"),
			// c.go imports a.go
			newRel("imp-ca", "imports", "file-c", "file-a"),
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
	if !affectedSet["b.go"] {
		t.Error("expected b.go (imported by a.go) in affected set")
	}
	if !affectedSet["c.go"] {
		t.Error("expected c.go (importer of a.go) in affected set")
	}
}

func TestComputeAffectedFiles_OldImportsIncluded(t *testing.T) {
	// a.go used to import b.go but no longer does; b.go must still be re-rendered.
	ir := buildIR(
		[]api.Node{
			newNode("file-a", []string{"File"}, "filePath", "a.go"),
		},
		nil,
	)
	d := NewTestDaemon(ir)
	d.cache = NewCache()
	d.cache.Build(ir)

	oldImports := map[string][]string{
		"a.go": {"b.go"},
	}
	affected := d.computeAffectedFiles([]string{"a.go"}, oldImports, nil)

	affectedSet := make(map[string]bool, len(affected))
	for _, f := range affected {
		affectedSet[f] = true
	}
	if !affectedSet["b.go"] {
		t.Error("expected b.go (old import) in affected set")
	}
}

func TestComputeAffectedFiles_OldCalleeFilesIncluded(t *testing.T) {
	// fn-a is in a.go; it used to call fn-d in d.go (captured in oldCalleeFiles).
	// Changing a.go should mark d.go as affected so stale back-references are
	// re-rendered.
	ir := buildIR(
		[]api.Node{
			newNode("file-a", []string{"File"}, "filePath", "a.go"),
			newNode("fn-a", []string{"Function"}, "filePath", "a.go", "name", "FuncA"),
		},
		nil,
	)
	d := NewTestDaemon(ir)
	d.cache = NewCache()
	d.cache.Build(ir)

	oldCalleeFiles := map[string][]string{
		"fn-a": {"d.go"},
	}
	affected := d.computeAffectedFiles([]string{"a.go"}, nil, oldCalleeFiles)

	affectedSet := make(map[string]bool, len(affected))
	for _, f := range affected {
		affectedSet[f] = true
	}

	if !affectedSet["d.go"] {
		t.Error("expected d.go (old callee file) in affected set")
	}
}

// ── newUUID ───────────────────────────────────────────────────────────────────

func TestNewUUID_Format(t *testing.T) {
	id := newUUID()
	// UUID v4 format: 8-4-4-4-12 hex chars separated by hyphens.
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 hyphen-separated parts, got %d: %q", len(parts), id)
	}
	want := []int{8, 4, 4, 4, 12}
	for i, p := range parts {
		if len(p) != want[i] {
			t.Errorf("part %d: expected %d hex chars, got %d: %q", i, want[i], len(p), p)
		}
	}
}

func TestNewUUID_Unique(t *testing.T) {
	ids := make(map[string]bool, 10)
	for i := 0; i < 10; i++ {
		id := newUUID()
		if ids[id] {
			t.Errorf("duplicate UUID produced: %q", id)
		}
		ids[id] = true
	}
}

func TestNewUUID_Version4Bits(t *testing.T) {
	id := newUUID()
	// UUID v4: bits 12-15 of time_hi_and_version = 0100 (i.e., 4th hex char of 3rd group is '4')
	parts := strings.Split(id, "-")
	if parts[2][0] != '4' {
		t.Errorf("expected version nibble '4' at start of 3rd group, got %q", parts[2][0])
	}
}

// ── OnSyncing callback ────────────────────────────────────────────────────────

// mockAnalyzeClient is a minimal analyzeClient that returns a fixed ShardIR.
type mockAnalyzeClient struct {
	result *api.ShardIR
	err    error
	called int
}

func (m *mockAnalyzeClient) AnalyzeShards(_ context.Context, _, _ string, _ []api.PreviousDomain) (*api.ShardIR, error) {
	m.called++
	return m.result, m.err
}

// orderClient records the call order of OnSyncing vs AnalyzeShards.
type orderClient struct {
	inner  analyzeClient
	onCall func()
}

func (o *orderClient) AnalyzeShards(ctx context.Context, zipPath, key string, prev []api.PreviousDomain) (*api.ShardIR, error) {
	if o.onCall != nil {
		o.onCall()
	}
	return o.inner.AnalyzeShards(ctx, zipPath, key, prev)
}

func TestOnSyncing_CalledWithCorrectFileCount(t *testing.T) {
	var gotN int
	d := &Daemon{
		cfg: DaemonConfig{
			RepoDir: t.TempDir(),
			OnSyncing: func(n int) {
				gotN = n
			},
		},
		client: &mockAnalyzeClient{result: buildIR(nil, nil)},
		cache:  NewCache(),
		ir:     buildIR(nil, nil),
		logf:   func(string, ...interface{}) {},
	}
	d.incrementalUpdate(context.Background(), []string{"a.go", "b.go", "c.go"})
	if gotN != 3 {
		t.Errorf("OnSyncing got n=%d, want 3", gotN)
	}
}

func TestOnSyncing_CalledBeforeAPIResponse(t *testing.T) {
	seq := 0
	syncingAt := 0
	apiAt := 0

	mock := &mockAnalyzeClient{result: buildIR(nil, nil)}
	d := &Daemon{
		cfg: DaemonConfig{
			RepoDir: t.TempDir(),
			OnSyncing: func(int) {
				seq++
				syncingAt = seq
			},
		},
		client: &orderClient{
			inner: mock,
			onCall: func() {
				seq++
				apiAt = seq
			},
		},
		cache: NewCache(),
		ir:    buildIR(nil, nil),
		logf:  func(string, ...interface{}) {},
	}
	d.incrementalUpdate(context.Background(), []string{"a.go"})

	if syncingAt == 0 {
		t.Fatal("OnSyncing was never called")
	}
	if apiAt == 0 {
		t.Fatal("AnalyzeShards was never called")
	}
	if syncingAt >= apiAt {
		t.Errorf("OnSyncing (seq %d) should fire before AnalyzeShards (seq %d)", syncingAt, apiAt)
	}
}

func TestOnSyncing_NilSafe(t *testing.T) {
	// OnSyncing=nil should not panic.
	d := &Daemon{
		cfg: DaemonConfig{
			RepoDir:   t.TempDir(),
			OnSyncing: nil,
		},
		client: &mockAnalyzeClient{result: buildIR(nil, nil)},
		cache:  NewCache(),
		ir:     buildIR(nil, nil),
		logf:   func(string, ...interface{}) {},
	}
	d.incrementalUpdate(context.Background(), []string{"a.go"})
}

func TestLoadOrGenerate_RegeneratesStaleFingerprintCache(t *testing.T) {
	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shardsRunGit(t, repoDir, "init")
	shardsRunGit(t, repoDir, "config", "user.email", "test@example.com")
	shardsRunGit(t, repoDir, "config", "user.name", "Test")
	shardsRunGit(t, repoDir, "add", "main.go")
	shardsRunGit(t, repoDir, "commit", "-m", "init")

	cacheFile := filepath.Join(repoDir, ".supermodel", "shards.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := buildIR(
		[]api.Node{newNode("old-file", []string{"File"}, "filePath", "old.go")},
		nil,
	)
	stale.Summary = map[string]any{shardCacheFingerprintKey: "stale"}
	staleJSON, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, staleJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	fresh := buildIR(
		[]api.Node{newNode("file-main", []string{"File"}, "filePath", "main.go")},
		nil,
	)
	client := &mockAnalyzeClient{result: fresh}
	d := &Daemon{
		cfg: DaemonConfig{
			RepoDir:   repoDir,
			CacheFile: cacheFile,
			LogFunc:   func(string, ...interface{}) {},
		},
		client:   client,
		cache:    NewCache(),
		logf:     func(string, ...interface{}) {},
		notifyCh: make(chan string, 256),
	}
	if err := d.loadOrGenerate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.called == 0 {
		t.Fatal("stale fingerprint cache should trigger API regeneration")
	}
	current, err := repocache.RepoFingerprint(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	var saved api.ShardIR
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if got, _ := saved.Summary[shardCacheFingerprintKey].(string); got != current {
		t.Fatalf("saved fingerprint = %q, want %q", got, current)
	}
}

func shardsRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// ── Port-conflict UX ─────────────────────────────────────────────────────────

// TestPortConflict_FriendlyMessage verifies that when the daemon cannot bind
// its UDP notify port (because another supermodel instance is already running),
// the returned error message is friendly and informative rather than a raw
// OS error. It should tell the user their graph is already being watched and
// NOT just say "already in use".
func TestPortConflict_FriendlyMessage(t *testing.T) {
	// Bind the notify port first to simulate a running instance.
	blocker, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not bind blocker socket: %v", err)
	}
	defer blocker.Close()
	port := blocker.LocalAddr().(*net.UDPAddr).Port

	// Write a minimal valid cache file so loadOrGenerate succeeds without an API call.
	repoDir := t.TempDir()
	cacheDir := repoDir + "/.supermodel"
	if mkErr := os.MkdirAll(cacheDir, 0o755); mkErr != nil {
		t.Fatalf("mkdir: %v", mkErr)
	}
	cacheFile := cacheDir + "/cache.json"
	minimalIR := `{"graph":{"nodes":[{"id":"n1","labels":["File"],"properties":{"filePath":"/fake/file.go"}}],"relationships":[]}}`
	if writeErr := os.WriteFile(cacheFile, []byte(minimalIR), 0o644); writeErr != nil {
		t.Fatalf("write cache: %v", writeErr)
	}

	cfg := DaemonConfig{
		RepoDir:    repoDir,
		CacheFile:  cacheFile,
		NotifyPort: port,
		FSWatch:    false, // FSWatch=false means EADDRINUSE is fatal
		LogFunc:    func(string, ...interface{}) {},
	}
	d := &Daemon{
		cfg:      cfg,
		client:   &mockAnalyzeClient{result: buildIR(nil, nil)},
		cache:    NewCache(),
		logf:     func(string, ...interface{}) {},
		notifyCh: make(chan string, 256),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := d.Run(ctx)
	if runErr == nil {
		t.Fatal("expected an error when port is already bound, got nil")
	}

	msg := runErr.Error()

	// The message must NOT be just a raw "already in use" OS error — it should be
	// friendly and tell the user what's happening.
	if !strings.Contains(msg, "already watching") && !strings.Contains(msg, "another terminal") {
		t.Errorf("error message is not user-friendly; got: %q\n"+
			"want: message containing \"already watching\" or \"another terminal\"", msg)
	}
}
