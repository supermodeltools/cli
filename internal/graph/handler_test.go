package graph

import (
	"bytes"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

// ── writeHuman ────────────────────────────────────────────────────────────────

func TestWriteHuman_ContainsHeaders(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "abc123", Labels: []string{"Function"}, Properties: map[string]any{"name": "main"}},
		},
		Relationships: []api.Relationship{},
	}
	var buf bytes.Buffer
	if err := writeHuman(&buf, g, ""); err != nil {
		t.Fatalf("writeHuman: %v", err)
	}
	out := buf.String()
	for _, header := range []string{"ID", "LABEL", "NAME"} {
		if !strings.Contains(out, header) {
			t.Errorf("should contain header %q:\n%s", header, out)
		}
	}
}

func TestWriteHuman_ContainsNodeData(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "abc123456789", Labels: []string{"Function"}, Properties: map[string]any{"name": "handleRequest"}},
		},
	}
	var buf bytes.Buffer
	if err := writeHuman(&buf, g, ""); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Function") {
		t.Errorf("should contain label:\n%s", out)
	}
	if !strings.Contains(out, "handleRequest") {
		t.Errorf("should contain node name:\n%s", out)
	}
}

func TestWriteHuman_ShowsCountLine(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "n1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "n2", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "IMPORTS", StartNode: "n1", EndNode: "n2"},
		},
	}
	var buf bytes.Buffer
	if err := writeHuman(&buf, g, ""); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "2 nodes") {
		t.Errorf("should show node count:\n%s", out)
	}
	if !strings.Contains(out, "1 relationship") {
		t.Errorf("should show relationship count:\n%s", out)
	}
}

func TestWriteHuman_Filter(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "doWork"}},
		},
	}
	var buf bytes.Buffer
	if err := writeHuman(&buf, g, "File"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "a.go") {
		t.Errorf("should contain filtered File node:\n%s", out)
	}
	// Count should say filtered
	if !strings.Contains(out, "filtered by label: File") {
		t.Errorf("should say filter info:\n%s", out)
	}
}

func TestWriteHuman_FilterRelationshipCount(t *testing.T) {
	// File nodes connected to each other, plus a Function→Function call.
	// When filtering by File, only the file→file relationship should be counted.
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "doWork"}},
			{ID: "fn2", Labels: []string{"Function"}, Properties: map[string]any{"name": "helper"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "f1", EndNode: "f2"},
			{ID: "r2", Type: "calls", StartNode: "fn1", EndNode: "fn2"},
		},
	}
	var buf bytes.Buffer
	if err := writeHuman(&buf, g, "File"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// Only r1 connects two File nodes — r2 connects Functions and must not be counted.
	if !strings.Contains(out, "1 relationship") {
		t.Errorf("filtered relationship count should be 1, got:\n%s", out)
	}
	if strings.Contains(out, "2 relationship") {
		t.Errorf("should not show 2 relationships when filter excludes one:\n%s", out)
	}
}

func TestWriteHuman_FilterExcludes(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "doWork"}},
		},
	}
	var buf bytes.Buffer
	if err := writeHuman(&buf, g, "File"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// "doWork" is a Function, not a File — should not appear
	if strings.Contains(out, "doWork") {
		t.Errorf("filtered output should not contain excluded label nodes:\n%s", out)
	}
}

func TestWriteHuman_IDTruncated(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "abcdefghijklmnopqrstuvwxyz", Labels: []string{"File"}, Properties: map[string]any{"path": "x.go"}},
		},
	}
	var buf bytes.Buffer
	if err := writeHuman(&buf, g, ""); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// ID should be truncated to 12 chars
	if strings.Contains(out, "abcdefghijklmnopqrstuvwxyz") {
		t.Errorf("long ID should be truncated:\n%s", out)
	}
	if !strings.Contains(out, "abcdefghijkl") {
		t.Errorf("should contain first 12 chars of ID:\n%s", out)
	}
}

// ── writeDOT ──────────────────────────────────────────────────────────────────

func TestWriteDOT_ValidDigraph(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "n1", Labels: []string{"Function"}, Properties: map[string]any{"name": "main"}},
			{ID: "n2", Labels: []string{"Function"}, Properties: map[string]any{"name": "helper"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "CALLS", StartNode: "n1", EndNode: "n2"},
		},
	}
	var buf bytes.Buffer
	if err := writeDOT(&buf, g, ""); err != nil {
		t.Fatalf("writeDOT: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(strings.TrimSpace(out), "digraph") {
		t.Errorf("should start with 'digraph':\n%s", out)
	}
	if !strings.Contains(out, "}") {
		t.Errorf("should end with closing brace:\n%s", out)
	}
}

func TestWriteDOT_ContainsNodes(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "n1", Labels: []string{"Function"}, Properties: map[string]any{"name": "main"}},
		},
	}
	var buf bytes.Buffer
	if err := writeDOT(&buf, g, ""); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "n1") {
		t.Errorf("should contain node ID:\n%s", out)
	}
}

func TestWriteDOT_ContainsEdges(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "n1", Labels: []string{"Function"}, Properties: map[string]any{"name": "foo"}},
			{ID: "n2", Labels: []string{"Function"}, Properties: map[string]any{"name": "bar"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "CALLS", StartNode: "n1", EndNode: "n2"},
		},
	}
	var buf bytes.Buffer
	if err := writeDOT(&buf, g, ""); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "->") {
		t.Errorf("should contain edge arrow:\n%s", out)
	}
	if !strings.Contains(out, "CALLS") {
		t.Errorf("should contain edge type:\n%s", out)
	}
}

func TestWriteDOT_FilterExcludesEdgesToFilteredNodes(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "file1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "doWork"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "DEFINES_FUNCTION", StartNode: "file1", EndNode: "fn1"},
		},
	}
	var buf bytes.Buffer
	if err := writeDOT(&buf, g, "File"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// fn1 is filtered out, so the edge should not appear
	if strings.Contains(out, "->") {
		t.Errorf("edge to filtered node should not appear:\n%s", out)
	}
}

func TestWriteDOT_LongNameTruncated(t *testing.T) {
	longName := strings.Repeat("a", 50)
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "n1", Labels: []string{"Function"}, Properties: map[string]any{"name": longName}},
		},
	}
	var buf bytes.Buffer
	if err := writeDOT(&buf, g, ""); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, longName) {
		t.Errorf("long name should be truncated:\n%s", out)
	}
}

// ── printGraph ────────────────────────────────────────────────────────────────

func TestPrintGraph_JSONOutput(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "n1", Labels: []string{"Function"}, Properties: map[string]any{"name": "foo"}},
		},
	}
	var buf bytes.Buffer
	if err := printGraph(&buf, g, Options{Output: "json"}); err != nil {
		t.Fatalf("printGraph json: %v", err)
	}
	if !strings.Contains(buf.String(), "{") {
		t.Errorf("json output should contain JSON:\n%s", buf.String())
	}
}

func TestPrintGraph_DOTOutput(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "n1", Labels: []string{"Function"}, Properties: map[string]any{"name": "foo"}},
		},
	}
	var buf bytes.Buffer
	if err := printGraph(&buf, g, Options{Output: "dot"}); err != nil {
		t.Fatalf("printGraph dot: %v", err)
	}
	if !strings.Contains(buf.String(), "digraph") {
		t.Errorf("dot output should contain 'digraph':\n%s", buf.String())
	}
}

func TestPrintGraph_HumanDefault(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "n1", Labels: []string{"Function"}, Properties: map[string]any{"name": "foo"}},
		},
	}
	var buf bytes.Buffer
	if err := printGraph(&buf, g, Options{}); err != nil {
		t.Fatalf("printGraph human: %v", err)
	}
	if !strings.Contains(buf.String(), "ID") {
		t.Errorf("human output should contain table headers:\n%s", buf.String())
	}
}
