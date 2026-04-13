package api

import (
	"testing"
)

// ── Node.Prop ─────────────────────────────────────────────────────────────────

func TestNode_Prop_FirstMatch(t *testing.T) {
	n := Node{Properties: map[string]any{"name": "handler", "path": "auth/handler.go"}}
	if got := n.Prop("name", "path"); got != "handler" {
		t.Errorf("want 'handler', got %q", got)
	}
}

func TestNode_Prop_Fallback(t *testing.T) {
	n := Node{Properties: map[string]any{"path": "auth/handler.go"}}
	if got := n.Prop("name", "path"); got != "auth/handler.go" {
		t.Errorf("want 'auth/handler.go', got %q", got)
	}
}

func TestNode_Prop_EmptyStringSkipped(t *testing.T) {
	n := Node{Properties: map[string]any{"name": "", "path": "auth/handler.go"}}
	if got := n.Prop("name", "path"); got != "auth/handler.go" {
		t.Errorf("empty string should be skipped: want 'auth/handler.go', got %q", got)
	}
}

func TestNode_Prop_NilProperties(t *testing.T) {
	n := Node{}
	if got := n.Prop("name"); got != "" {
		t.Errorf("nil properties: want '', got %q", got)
	}
}

func TestNode_Prop_WrongType(t *testing.T) {
	n := Node{Properties: map[string]any{"count": 42}} // int, not string
	if got := n.Prop("count"); got != "" {
		t.Errorf("non-string value: want '', got %q", got)
	}
}

func TestNode_Prop_NoKeys(t *testing.T) {
	n := Node{Properties: map[string]any{"name": "foo"}}
	if got := n.Prop(); got != "" {
		t.Errorf("no keys: want '', got %q", got)
	}
}

func TestNode_Prop_MissingKey(t *testing.T) {
	n := Node{Properties: map[string]any{"name": "foo"}}
	if got := n.Prop("path"); got != "" {
		t.Errorf("missing key: want '', got %q", got)
	}
}

// ── Node.HasLabel ─────────────────────────────────────────────────────────────

func TestNode_HasLabel_Found(t *testing.T) {
	n := Node{Labels: []string{"Function", "Exported"}}
	if !n.HasLabel("Function") {
		t.Error("should find 'Function'")
	}
}

func TestNode_HasLabel_NotFound(t *testing.T) {
	n := Node{Labels: []string{"Function"}}
	if n.HasLabel("File") {
		t.Error("should not find 'File'")
	}
}

func TestNode_HasLabel_Empty(t *testing.T) {
	n := Node{}
	if n.HasLabel("Function") {
		t.Error("no labels: should return false")
	}
}

func TestNode_HasLabel_CaseSensitive(t *testing.T) {
	n := Node{Labels: []string{"Function"}}
	if n.HasLabel("function") {
		t.Error("label matching should be case-sensitive")
	}
}

// ── Graph.Rels ────────────────────────────────────────────────────────────────

func TestGraph_Rels_PrefersRelationships(t *testing.T) {
	g := &Graph{
		Edges:         []Relationship{{ID: "e1"}},
		Relationships: []Relationship{{ID: "r1"}, {ID: "r2"}},
	}
	rels := g.Rels()
	if len(rels) != 2 || rels[0].ID != "r1" {
		t.Errorf("Rels() should prefer Relationships field, got %v", rels)
	}
}

func TestGraph_Rels_FallsBackToEdges(t *testing.T) {
	g := &Graph{
		Edges: []Relationship{{ID: "e1"}, {ID: "e2"}},
	}
	rels := g.Rels()
	if len(rels) != 2 || rels[0].ID != "e1" {
		t.Errorf("Rels() should fall back to Edges, got %v", rels)
	}
}

func TestGraph_Rels_Empty(t *testing.T) {
	g := &Graph{}
	if rels := g.Rels(); rels != nil {
		t.Errorf("empty graph: want nil, got %v", rels)
	}
}

// ── Graph.RepoID ──────────────────────────────────────────────────────────────

func TestGraph_RepoID_Present(t *testing.T) {
	g := &Graph{Metadata: map[string]any{"repoId": "abc123"}}
	if got := g.RepoID(); got != "abc123" {
		t.Errorf("want 'abc123', got %q", got)
	}
}

func TestGraph_RepoID_Missing(t *testing.T) {
	g := &Graph{Metadata: map[string]any{"other": "value"}}
	if got := g.RepoID(); got != "" {
		t.Errorf("missing repoId: want '', got %q", got)
	}
}

func TestGraph_RepoID_NilMetadata(t *testing.T) {
	g := &Graph{}
	if got := g.RepoID(); got != "" {
		t.Errorf("nil metadata: want '', got %q", got)
	}
}

func TestGraph_RepoID_WrongType(t *testing.T) {
	g := &Graph{Metadata: map[string]any{"repoId": 42}} // int, not string
	if got := g.RepoID(); got != "" {
		t.Errorf("wrong type: want '', got %q", got)
	}
}

// ── Graph.NodesByLabel ────────────────────────────────────────────────────────

func TestGraph_NodesByLabel_Match(t *testing.T) {
	g := &Graph{Nodes: []Node{
		{ID: "1", Labels: []string{"Function"}},
		{ID: "2", Labels: []string{"File"}},
		{ID: "3", Labels: []string{"Function", "Exported"}},
	}}
	fns := g.NodesByLabel("Function")
	if len(fns) != 2 {
		t.Errorf("want 2 Function nodes, got %d", len(fns))
	}
}

func TestGraph_NodesByLabel_NoMatch(t *testing.T) {
	g := &Graph{Nodes: []Node{{ID: "1", Labels: []string{"File"}}}}
	if nodes := g.NodesByLabel("Function"); len(nodes) != 0 {
		t.Errorf("no match: want [], got %v", nodes)
	}
}

func TestGraph_NodesByLabel_Empty(t *testing.T) {
	g := &Graph{}
	if nodes := g.NodesByLabel("File"); nodes != nil {
		t.Errorf("empty graph: want nil, got %v", nodes)
	}
}

// ── Graph.NodeByID ────────────────────────────────────────────────────────────

func TestGraph_NodeByID_Found(t *testing.T) {
	g := &Graph{Nodes: []Node{
		{ID: "abc", Labels: []string{"Function"}},
		{ID: "def", Labels: []string{"File"}},
	}}
	n, ok := g.NodeByID("abc")
	if !ok {
		t.Fatal("expected to find node 'abc'")
	}
	if n.ID != "abc" {
		t.Errorf("wrong node: got %q", n.ID)
	}
}

func TestGraph_NodeByID_NotFound(t *testing.T) {
	g := &Graph{Nodes: []Node{{ID: "abc"}}}
	_, ok := g.NodeByID("xyz")
	if ok {
		t.Error("should not find 'xyz'")
	}
}

func TestGraph_NodeByID_Empty(t *testing.T) {
	g := &Graph{}
	_, ok := g.NodeByID("abc")
	if ok {
		t.Error("empty graph: should return false")
	}
}

// ── Error ─────────────────────────────────────────────────────────────────────

func TestError_Error_WithCode(t *testing.T) {
	e := &Error{StatusCode: 422, Code: "INVALID_INPUT", Message: "bad zip file"}
	got := e.Error()
	if got == "" {
		t.Fatal("Error() should return non-empty string")
	}
	// Should contain status code and message
	for _, want := range []string{"422", "bad zip file"} {
		if !containsStr(got, want) {
			t.Errorf("Error() = %q, should contain %q", got, want)
		}
	}
}

func TestError_Error_WithoutCode(t *testing.T) {
	e := &Error{StatusCode: 500, Message: "internal server error"}
	got := e.Error()
	if got == "" {
		t.Fatal("Error() should return non-empty string")
	}
	if !containsStr(got, "500") {
		t.Errorf("Error() = %q, should contain '500'", got)
	}
}

func TestError_Error_FallsBackToStatus(t *testing.T) {
	// When StatusCode is 0, Error() should use the Status field.
	e := &Error{StatusCode: 0, Status: 404, Message: "not found"}
	got := e.Error()
	if !containsStr(got, "404") {
		t.Errorf("Error() = %q, should contain '404' (from Status field)", got)
	}
}

// ── GraphFromShardIR ──────────────────────────────────────────────────────────

func TestGraphFromShardIR_NodesAndRels(t *testing.T) {
	ir := &ShardIR{
		Repo: "myorg/myrepo",
		Graph: ShardGraph{
			Nodes: []Node{
				{ID: "n1", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/a.go"}},
				{ID: "n2", Labels: []string{"Function"}, Properties: map[string]any{"name": "doThing"}},
			},
			Relationships: []Relationship{
				{ID: "r1", Type: "defines_function", StartNode: "n1", EndNode: "n2"},
			},
		},
	}
	g := GraphFromShardIR(ir)

	if len(g.Nodes) != 2 {
		t.Errorf("nodes: got %d, want 2", len(g.Nodes))
	}
	if len(g.Relationships) != 1 {
		t.Errorf("relationships: got %d, want 1", len(g.Relationships))
	}
	if g.Nodes[0].ID != "n1" {
		t.Errorf("first node ID: got %q", g.Nodes[0].ID)
	}
}

func TestGraphFromShardIR_RepoID(t *testing.T) {
	ir := &ShardIR{Repo: "acme/backend"}
	g := GraphFromShardIR(ir)
	if got := g.RepoID(); got != "acme/backend" {
		t.Errorf("RepoID: got %q, want 'acme/backend'", got)
	}
}

func TestGraphFromShardIR_RelsViaRels(t *testing.T) {
	// Rels() should return the Relationships slice (not Edges)
	ir := &ShardIR{
		Graph: ShardGraph{
			Relationships: []Relationship{
				{ID: "r1", Type: "imports"},
				{ID: "r2", Type: "calls"},
			},
		},
	}
	g := GraphFromShardIR(ir)
	rels := g.Rels()
	if len(rels) != 2 {
		t.Errorf("Rels(): got %d, want 2", len(rels))
	}
}

func TestGraphFromShardIR_Empty(t *testing.T) {
	ir := &ShardIR{}
	g := GraphFromShardIR(ir)
	if g == nil {
		t.Fatal("GraphFromShardIR returned nil")
	}
	if len(g.Nodes) != 0 {
		t.Errorf("empty IR: expected 0 nodes, got %d", len(g.Nodes))
	}
	if g.RepoID() != "" {
		t.Errorf("empty IR: expected empty repoId, got %q", g.RepoID())
	}
}

func TestGraphFromShardIR_NodeByID(t *testing.T) {
	ir := &ShardIR{
		Graph: ShardGraph{
			Nodes: []Node{
				{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "myFunc"}},
			},
		},
	}
	g := GraphFromShardIR(ir)
	n, ok := g.NodeByID("fn1")
	if !ok {
		t.Fatal("NodeByID('fn1') returned false")
	}
	if n.Prop("name") != "myFunc" {
		t.Errorf("name prop: got %q", n.Prop("name"))
	}
}

func TestGraphFromShardIR_NodesByLabel(t *testing.T) {
	ir := &ShardIR{
		Graph: ShardGraph{
			Nodes: []Node{
				{ID: "f1", Labels: []string{"File"}},
				{ID: "fn1", Labels: []string{"Function"}},
				{ID: "f2", Labels: []string{"File"}},
			},
		},
	}
	g := GraphFromShardIR(ir)
	files := g.NodesByLabel("File")
	if len(files) != 2 {
		t.Errorf("NodesByLabel('File'): got %d, want 2", len(files))
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
