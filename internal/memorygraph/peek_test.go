package memorygraph

import (
	"strings"
	"testing"
)

// seedGraph writes a small graph to a temp dir and returns the rootDir.
func seedGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	nodes := []struct {
		typ     NodeType
		label   string
		content string
	}{
		{NodeTypeFact, "Go is compiled", "Go compiles to native machine code."},
		{NodeTypeConcept, "Dependency Injection", "A technique where dependencies are provided externally."},
		{NodeTypeEntity, "supermodel-cli", "The CLI tool for Supermodel analysis."},
	}
	for _, n := range nodes {
		if _, err := UpsertNode(dir, n.typ, n.label, n.content, nil); err != nil {
			t.Fatalf("UpsertNode: %v", err)
		}
	}

	factID := nodeID(NodeTypeFact, "Go is compiled")
	entityID := nodeID(NodeTypeEntity, "supermodel-cli")
	if _, err := CreateRelation(dir, factID, entityID, RelationRelatedTo, 0.9, nil); err != nil {
		t.Fatalf("CreateRelation: %v", err)
	}
	return dir
}

// ── Peek ──────────────────────────────────────────────────────────────────────

func TestPeek_ByID(t *testing.T) {
	dir := seedGraph(t)
	id := nodeID(NodeTypeFact, "Go is compiled")

	p, err := Peek(PeekOptions{RootDir: dir, NodeID: id})
	if err != nil {
		t.Fatalf("Peek by ID: %v", err)
	}
	if p == nil {
		t.Fatal("Peek by ID: want non-nil, got nil")
		return
	}
	if p.Node.ID != id {
		t.Errorf("Peek by ID: want ID %q, got %q", id, p.Node.ID)
	}
}

func TestPeek_ByLabel(t *testing.T) {
	dir := seedGraph(t)

	p, err := Peek(PeekOptions{RootDir: dir, Label: "Go is compiled"})
	if err != nil {
		t.Fatalf("Peek by label: %v", err)
	}
	if p == nil {
		t.Fatal("Peek by label: want non-nil, got nil")
		return
	}
	if p.Node.Label != "Go is compiled" {
		t.Errorf("Peek by label: want label %q, got %q", "Go is compiled", p.Node.Label)
	}
}

func TestPeek_ByLabel_CaseInsensitive(t *testing.T) {
	dir := seedGraph(t)

	p, err := Peek(PeekOptions{RootDir: dir, Label: "GO IS COMPILED"})
	if err != nil {
		t.Fatalf("Peek case-insensitive: %v", err)
	}
	if p == nil {
		t.Fatal("Peek case-insensitive: want non-nil, got nil")
	}
}

func TestPeek_NotFound(t *testing.T) {
	dir := seedGraph(t)

	p, err := Peek(PeekOptions{RootDir: dir, NodeID: "nonexistent:id"})
	if err != nil {
		t.Fatalf("Peek not-found: unexpected error: %v", err)
	}
	if p != nil {
		t.Errorf("Peek not-found: want nil, got %+v", p)
	}
}

func TestPeek_EdgesPopulated(t *testing.T) {
	dir := seedGraph(t)
	factID := nodeID(NodeTypeFact, "Go is compiled")

	p, err := Peek(PeekOptions{RootDir: dir, NodeID: factID})
	if err != nil {
		t.Fatalf("Peek edges: %v", err)
	}
	if len(p.EdgesOut) != 1 {
		t.Errorf("EdgesOut: want 1, got %d", len(p.EdgesOut))
	}
	if len(p.EdgesIn) != 0 {
		t.Errorf("EdgesIn: want 0, got %d", len(p.EdgesIn))
	}
	if p.EdgesOut[0].Edge.Relation != RelationRelatedTo {
		t.Errorf("edge relation: want %q, got %q", RelationRelatedTo, p.EdgesOut[0].Edge.Relation)
	}
}

func TestPeek_IDPriorityOverLabel(t *testing.T) {
	dir := seedGraph(t)
	id := nodeID(NodeTypeFact, "Go is compiled")

	// Pass a valid NodeID and a Label that matches a different node; ID wins.
	p, err := Peek(PeekOptions{RootDir: dir, NodeID: id, Label: "Dependency Injection"})
	if err != nil {
		t.Fatalf("Peek ID priority: %v", err)
	}
	if p == nil {
		t.Fatal("Peek ID priority: want non-nil, got nil")
		return
	}
	if p.Node.ID != id {
		t.Errorf("Peek ID priority: want ID %q, got %q", id, p.Node.ID)
	}
}

// ── PeekList ──────────────────────────────────────────────────────────────────

func TestPeekList_ReturnsAllNodes(t *testing.T) {
	dir := seedGraph(t)

	peeks, err := PeekList(dir)
	if err != nil {
		t.Fatalf("PeekList: %v", err)
	}
	if len(peeks) != 3 {
		t.Errorf("PeekList: want 3 nodes, got %d", len(peeks))
	}
}

func TestPeekList_EmptyGraph(t *testing.T) {
	dir := t.TempDir()

	peeks, err := PeekList(dir)
	if err != nil {
		t.Fatalf("PeekList empty: %v", err)
	}
	if len(peeks) != 0 {
		t.Errorf("PeekList empty: want 0, got %d", len(peeks))
	}
}

func TestPeekList_SortedByAccessCountDesc(t *testing.T) {
	dir := t.TempDir()

	_, _ = UpsertNode(dir, NodeTypeFact, "low", "low access", nil)
	// Bump "high" access count by upserting it multiple times.
	for i := 0; i < 5; i++ {
		_, _ = UpsertNode(dir, NodeTypeFact, "high", "high access", nil)
	}

	peeks, err := PeekList(dir)
	if err != nil {
		t.Fatalf("PeekList sort: %v", err)
	}
	if len(peeks) < 2 {
		t.Fatalf("PeekList sort: want ≥2 nodes, got %d", len(peeks))
	}
	if peeks[0].Node.Label != "high" {
		t.Errorf("PeekList sort: want first node %q, got %q", "high", peeks[0].Node.Label)
	}
}

// ── FormatPeek ────────────────────────────────────────────────────────────────

func TestFormatPeek_NilReturnsNotFound(t *testing.T) {
	out := FormatPeek(nil)
	if !strings.Contains(out, "not found") {
		t.Errorf("FormatPeek(nil): want 'not found', got %q", out)
	}
}

func TestFormatPeek_ContainsNodeInfo(t *testing.T) {
	dir := seedGraph(t)
	id := nodeID(NodeTypeFact, "Go is compiled")

	p, _ := Peek(PeekOptions{RootDir: dir, NodeID: id})
	out := FormatPeek(p)

	for _, want := range []string{"Go is compiled", string(NodeTypeFact), id} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatPeek: want output to contain %q\ngot:\n%s", want, out)
		}
	}
}

func TestFormatPeek_ShowsOutEdges(t *testing.T) {
	dir := seedGraph(t)
	factID := nodeID(NodeTypeFact, "Go is compiled")

	p, _ := Peek(PeekOptions{RootDir: dir, NodeID: factID})
	out := FormatPeek(p)

	if !strings.Contains(out, "Out") {
		t.Errorf("FormatPeek: want 'Out' section for node with outbound edges\ngot:\n%s", out)
	}
}

// ── FormatPeekList ────────────────────────────────────────────────────────────

func TestFormatPeekList_EmptyGraph(t *testing.T) {
	out := FormatPeekList(nil)
	if !strings.Contains(out, "empty") {
		t.Errorf("FormatPeekList(nil): want 'empty', got %q", out)
	}
}

func TestFormatPeekList_ContainsHeaders(t *testing.T) {
	dir := seedGraph(t)

	peeks, _ := PeekList(dir)
	out := FormatPeekList(peeks)

	for _, header := range []string{"TYPE", "LABEL", "ACCESSED"} {
		if !strings.Contains(out, header) {
			t.Errorf("FormatPeekList: want header %q\ngot:\n%s", header, out)
		}
	}
}

func TestFormatPeekList_ContainsNodeLabels(t *testing.T) {
	dir := seedGraph(t)

	peeks, _ := PeekList(dir)
	out := FormatPeekList(peeks)

	for _, label := range []string{"Go is compiled", "Dependency Injection", "supermodel-cli"} {
		if !strings.Contains(out, label) {
			t.Errorf("FormatPeekList: want label %q in output\ngot:\n%s", label, out)
		}
	}
}
