package graph2md

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parseGraphData extracts the "graph_data" JSON from a rendered markdown file's
// frontmatter and returns the parsed graphData struct.
func parseGraphData(t *testing.T, content string) struct {
	Nodes []struct {
		ID string  `json:"id"`
		LC int     `json:"lc"`
	} `json:"nodes"`
} {
	t.Helper()
	const key = `graph_data: "`
	idx := strings.Index(content, key)
	if idx < 0 {
		t.Fatal("graph_data key not found in output")
	}
	start := idx + len(key)
	// graph_data value is a quoted Go string — find the closing unescaped "
	end := strings.Index(content[start:], "\"\n")
	if end < 0 {
		t.Fatal("graph_data closing quote not found")
	}
	// Unquote the embedded JSON
	raw := strings.ReplaceAll(content[start:start+end], `\"`, `"`)
	var gd struct {
		Nodes []struct {
			ID string `json:"id"`
			LC int    `json:"lc"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(raw), &gd); err != nil {
		t.Fatalf("unmarshal graph_data: %v\nraw: %s", err, raw)
	}
	return gd
}

// buildGraphJSON serialises nodes and relationships into a Graph JSON file
// that loadGraph can parse.
func buildGraphJSON(t *testing.T, nodes []Node, rels []Relationship) string {
	t.Helper()
	g := Graph{Nodes: nodes, Relationships: rels}
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	f, err := os.CreateTemp(t.TempDir(), "graph-*.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

// TestSlugCollisionResolution verifies that when two nodes produce the same
// base slug, the second gets a "-2" suffix, AND that a third node which
// naturally produces that same "-2" slug does not silently collide with it.
func TestSlugCollisionResolution(t *testing.T) {
	// Two Function nodes in different directories but same base-name file (handler.go)
	// both produce slug "fn-handler-go-run".
	// A third Function node whose name is literally "run-2" in handler.go would
	// naturally produce "fn-handler-go-run-2" — the same as the collision-resolved
	// slug for the second node. Without the fix, both get the same output file.
	nodes := []Node{
		{
			ID:     "fn:internal/api/handler.go:run",
			Labels: []string{"Function"},
			Properties: map[string]interface{}{
				"name":     "run",
				"filePath": "internal/api/handler.go",
			},
		},
		{
			ID:     "fn:internal/files/handler.go:run",
			Labels: []string{"Function"},
			Properties: map[string]interface{}{
				"name":     "run",
				"filePath": "internal/files/handler.go",
			},
		},
		{
			ID:     "fn:internal/api/handler.go:run-2",
			Labels: []string{"Function"},
			Properties: map[string]interface{}{
				"name":     "run-2",
				"filePath": "internal/api/handler.go",
			},
		},
	}

	graphFile := buildGraphJSON(t, nodes, nil)
	outDir := t.TempDir()

	if err := Run(graphFile, outDir, "testrepo", "https://github.com/example/repo", 0); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Collect all generated .md files
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var slugs []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			slugs = append(slugs, strings.TrimSuffix(e.Name(), ".md"))
		}
	}

	// Must have exactly 3 files — one per node, all with distinct slugs.
	if len(slugs) != 3 {
		t.Errorf("expected 3 output files, got %d: %v", len(slugs), slugs)
	}

	// Check uniqueness
	seen := make(map[string]bool)
	for _, s := range slugs {
		if seen[s] {
			t.Errorf("duplicate slug %q — slug collision not resolved", s)
		}
		seen[s] = true
	}
}

// TestLineCountMissingStartLine verifies that when a Function node has an
// endLine but no startLine, line_count defaults to endLine (i.e. startLine=1)
// rather than endLine+1 (which would happen if startLine were treated as 0).
func TestLineCountMissingStartLine(t *testing.T) {
	nodes := []Node{
		{
			ID:     "fn:src/foo.go:bar",
			Labels: []string{"Function"},
			Properties: map[string]interface{}{
				"name":    "bar",
				"endLine": float64(50), // startLine intentionally absent
			},
		},
	}

	graphFile := buildGraphJSON(t, nodes, nil)
	outDir := t.TempDir()

	if err := Run(graphFile, outDir, "testrepo", "", 0); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Find the generated file
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 output file, got %d", len(entries))
	}

	content, err := os.ReadFile(filepath.Join(outDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// line_count should be 50 (endLine=50, effectiveStartLine=1 → 50-1+1=50)
	// NOT 51 (which would be 50-0+1).
	if strings.Contains(string(content), "line_count: 51") {
		t.Errorf("line_count is 51 (off-by-one: startLine treated as 0 instead of 1)")
	}
	if !strings.Contains(string(content), "line_count: 50") {
		t.Errorf("expected line_count: 50 in output, got:\n%s", content)
	}
}

// TestGraphDataLineCountMissingStartLine verifies that the graph_data JSON
// embedded in the markdown frontmatter uses the same effectiveStart=1 logic
// as the text line_count field. Before the fix, a node with endLine=50 but
// no startLine would have lc=0 (condition startLine>0 was false), while the
// frontmatter line_count correctly showed 50.
//
// A DEFINES_FUNCTION relationship to a file is included so that the function
// node has at least one neighbor; writeGraphData skips output when len(nodes)<2.
func TestGraphDataLineCountMissingStartLine(t *testing.T) {
	nodes := []Node{
		{
			ID:     "file:src/foo.go",
			Labels: []string{"File"},
			Properties: map[string]interface{}{
				"path":      "src/foo.go",
				"lineCount": float64(100),
			},
		},
		{
			ID:     "fn:src/foo.go:bar",
			Labels: []string{"Function"},
			Properties: map[string]interface{}{
				"name":     "bar",
				"filePath": "src/foo.go",
				"endLine":  float64(50), // startLine intentionally absent
			},
		},
	}
	rels := []Relationship{
		{
			ID:        "r1",
			Type:      "DEFINES_FUNCTION",
			StartNode: "file:src/foo.go",
			EndNode:   "fn:src/foo.go:bar",
		},
	}

	graphFile := buildGraphJSON(t, nodes, rels)
	outDir := t.TempDir()

	if err := Run(graphFile, outDir, "testrepo", "", 0); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Find the function's markdown file
	entries, _ := os.ReadDir(outDir)
	var fnFile string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "fn-") {
			fnFile = filepath.Join(outDir, e.Name())
			break
		}
	}
	if fnFile == "" {
		t.Fatal("function markdown file not found")
	}

	content, err := os.ReadFile(fnFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	gd := parseGraphData(t, string(content))
	// Find the function node in graph_data
	var fnLC int = -1
	for _, n := range gd.Nodes {
		if n.ID == "fn:src/foo.go:bar" {
			fnLC = n.LC
			break
		}
	}
	if fnLC == -1 {
		t.Fatalf("function node not found in graph_data nodes: %v", gd.Nodes)
	}
	if fnLC != 50 {
		t.Errorf("graph_data lc = %d, want 50 (endLine=50, effectiveStart=1)", fnLC)
	}
}
