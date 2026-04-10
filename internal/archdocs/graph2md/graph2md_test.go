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
		ID string `json:"id"`
		LC int    `json:"lc"`
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

// ── getStr ────────────────────────────────────────────────────────────────────

func TestGetStr(t *testing.T) {
	m := map[string]interface{}{"name": "foo", "num": 42, "empty": ""}
	if got := getStr(m, "name"); got != "foo" {
		t.Errorf("got %q, want %q", got, "foo")
	}
	if got := getStr(m, "num"); got != "" {
		t.Errorf("non-string: got %q, want empty", got)
	}
	if got := getStr(m, "missing"); got != "" {
		t.Errorf("missing key: got %q, want empty", got)
	}
	if got := getStr(m, "empty"); got != "" {
		t.Errorf("empty string: got %q, want empty", got)
	}
}

// ── getNum ────────────────────────────────────────────────────────────────────

func TestGetNum(t *testing.T) {
	m := map[string]interface{}{"f64": float64(7), "i": 9, "str": "x"}
	if got := getNum(m, "f64"); got != 7 {
		t.Errorf("float64: got %d, want 7", got)
	}
	if got := getNum(m, "i"); got != 9 {
		t.Errorf("int: got %d, want 9", got)
	}
	if got := getNum(m, "str"); got != 0 {
		t.Errorf("wrong type: got %d, want 0", got)
	}
	if got := getNum(m, "missing"); got != 0 {
		t.Errorf("missing key: got %d, want 0", got)
	}
}

// ── mermaidID ─────────────────────────────────────────────────────────────────

func TestMermaidID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"fn:src/foo.go:bar", "fn_src_foo_go_bar"},
		{"hello_world", "hello_world"},
		{"ABC123", "ABC123"},
		{"", "node"},
		{"---", "___"},
	}
	for _, tc := range cases {
		got := mermaidID(tc.in)
		if got != tc.want {
			t.Errorf("mermaidID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ── generateSlug ─────────────────────────────────────────────────────────────

func TestGenerateSlug_File(t *testing.T) {
	n := Node{Properties: map[string]interface{}{"path": "src/main.go"}}
	got := generateSlug(n, "File")
	if !strings.HasPrefix(got, "file-") {
		t.Errorf("File slug: got %q, want prefix 'file-'", got)
	}
	// empty path → empty slug
	n2 := Node{Properties: map[string]interface{}{}}
	if got2 := generateSlug(n2, "File"); got2 != "" {
		t.Errorf("empty path File slug: got %q, want empty", got2)
	}
}

func TestGenerateSlug_Function(t *testing.T) {
	n := Node{Properties: map[string]interface{}{"name": "run", "filePath": "internal/api/handler.go"}}
	got := generateSlug(n, "Function")
	if !strings.HasPrefix(got, "fn-") {
		t.Errorf("Function slug with path: got %q, want prefix 'fn-'", got)
	}
	n2 := Node{Properties: map[string]interface{}{"name": "run"}}
	got2 := generateSlug(n2, "Function")
	if !strings.HasPrefix(got2, "fn-") {
		t.Errorf("Function slug without path: got %q, want prefix 'fn-'", got2)
	}
	n3 := Node{Properties: map[string]interface{}{}}
	if got3 := generateSlug(n3, "Function"); got3 != "" {
		t.Errorf("empty name: got %q, want empty", got3)
	}
}

func TestGenerateSlug_ClassTypeLabels(t *testing.T) {
	for _, label := range []string{"Class", "Type"} {
		prefix := strings.ToLower(label) + "-"
		n := Node{Properties: map[string]interface{}{"name": "MyEntity", "filePath": "src/foo.go"}}
		got := generateSlug(n, label)
		if !strings.HasPrefix(got, prefix) {
			t.Errorf("%s slug: got %q, want prefix %q", label, got, prefix)
		}
		n2 := Node{Properties: map[string]interface{}{"name": "MyEntity"}}
		got2 := generateSlug(n2, label)
		if !strings.HasPrefix(got2, prefix) {
			t.Errorf("%s slug without path: got %q, want prefix %q", label, got2, prefix)
		}
		n3 := Node{Properties: map[string]interface{}{}}
		if got3 := generateSlug(n3, label); got3 != "" {
			t.Errorf("%s empty name: got %q, want empty", label, got3)
		}
	}
}

func TestGenerateSlug_DomainSubdomain(t *testing.T) {
	dn := Node{Properties: map[string]interface{}{"name": "auth"}}
	if got := generateSlug(dn, "Domain"); !strings.HasPrefix(got, "domain-") {
		t.Errorf("Domain: got %q, want prefix 'domain-'", got)
	}
	sn := Node{Properties: map[string]interface{}{"name": "users"}}
	if got := generateSlug(sn, "Subdomain"); !strings.HasPrefix(got, "subdomain-") {
		t.Errorf("Subdomain: got %q, want prefix 'subdomain-'", got)
	}
	empty := Node{Properties: map[string]interface{}{}}
	if got := generateSlug(empty, "Domain"); got != "" {
		t.Errorf("Domain empty name: got %q, want empty", got)
	}
	if got := generateSlug(empty, "Subdomain"); got != "" {
		t.Errorf("Subdomain empty name: got %q, want empty", got)
	}
}

func TestGenerateSlug_Directory(t *testing.T) {
	n := Node{Properties: map[string]interface{}{"path": "internal/api"}}
	if got := generateSlug(n, "Directory"); !strings.HasPrefix(got, "dir-") {
		t.Errorf("Directory: got %q, want prefix 'dir-'", got)
	}
	// path containing /app/repo-root/ → empty
	n2 := Node{Properties: map[string]interface{}{"path": "/app/repo-root/internal"}}
	if got := generateSlug(n2, "Directory"); got != "" {
		t.Errorf("repo-root path: got %q, want empty", got)
	}
	// empty path → empty
	n3 := Node{Properties: map[string]interface{}{}}
	if got := generateSlug(n3, "Directory"); got != "" {
		t.Errorf("empty path: got %q, want empty", got)
	}
}

func TestGenerateSlug_Unknown(t *testing.T) {
	n := Node{Properties: map[string]interface{}{"name": "foo"}}
	if got := generateSlug(n, "Unknown"); got != "" {
		t.Errorf("unknown label: got %q, want empty", got)
	}
}

// ── node-type rendering ───────────────────────────────────────────────────────

// TestRunClassNode verifies that a Class node generates a markdown file
// containing class-specific frontmatter fields.
func TestRunClassNode(t *testing.T) {
	nodes := []Node{
		{
			ID:     "class:src/auth.go:UserAuth",
			Labels: []string{"Class"},
			Properties: map[string]interface{}{
				"name":      "UserAuth",
				"filePath":  "src/auth.go",
				"startLine": float64(10),
				"endLine":   float64(50),
				"language":  "go",
			},
		},
	}
	graphFile := buildGraphJSON(t, nodes, nil)
	outDir := t.TempDir()
	if err := Run(graphFile, outDir, "myrepo", "https://github.com/example/myrepo", 0); err != nil {
		t.Fatalf("Run: %v", err)
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 output file, got %d", len(entries))
	}
	content, err := os.ReadFile(filepath.Join(outDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	body := string(content)
	for _, want := range []string{`node_type: "Class"`, `class_name: "UserAuth"`, `language: "go"`, `start_line: 10`, `end_line: 50`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in class output:\n%s", want, body)
		}
	}
}

// TestRunTypeNode verifies that a Type node generates type-specific frontmatter.
func TestRunTypeNode(t *testing.T) {
	nodes := []Node{
		{
			ID:     "type:src/types.go:UserID",
			Labels: []string{"Type"},
			Properties: map[string]interface{}{
				"name":     "UserID",
				"filePath": "src/types.go",
			},
		},
	}
	graphFile := buildGraphJSON(t, nodes, nil)
	outDir := t.TempDir()
	if err := Run(graphFile, outDir, "myrepo", "", 0); err != nil {
		t.Fatalf("Run: %v", err)
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 output file, got %d", len(entries))
	}
	content, _ := os.ReadFile(filepath.Join(outDir, entries[0].Name()))
	body := string(content)
	for _, want := range []string{`node_type: "Type"`, `type_name: "UserID"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in type output:\n%s", want, body)
		}
	}
}

// TestRunDomainNode verifies that a Domain node generates domain-specific frontmatter.
func TestRunDomainNode(t *testing.T) {
	nodes := []Node{
		{
			ID:     "domain:auth",
			Labels: []string{"Domain"},
			Properties: map[string]interface{}{
				"name":        "auth",
				"description": "Authentication domain",
			},
		},
	}
	graphFile := buildGraphJSON(t, nodes, nil)
	outDir := t.TempDir()
	if err := Run(graphFile, outDir, "myrepo", "", 0); err != nil {
		t.Fatalf("Run: %v", err)
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 output file, got %d", len(entries))
	}
	content, _ := os.ReadFile(filepath.Join(outDir, entries[0].Name()))
	body := string(content)
	for _, want := range []string{`node_type: "Domain"`, `domain: "auth"`, `summary: "Authentication domain"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in domain output:\n%s", want, body)
		}
	}
}

// TestRunSubdomainNode verifies that a Subdomain node generates subdomain frontmatter.
func TestRunSubdomainNode(t *testing.T) {
	nodes := []Node{
		{
			ID:     "subdomain:users",
			Labels: []string{"Subdomain"},
			Properties: map[string]interface{}{
				"name": "users",
			},
		},
	}
	graphFile := buildGraphJSON(t, nodes, nil)
	outDir := t.TempDir()
	if err := Run(graphFile, outDir, "myrepo", "", 0); err != nil {
		t.Fatalf("Run: %v", err)
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 output file, got %d", len(entries))
	}
	content, _ := os.ReadFile(filepath.Join(outDir, entries[0].Name()))
	body := string(content)
	for _, want := range []string{`node_type: "Subdomain"`, `subdomain: "users"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in subdomain output:\n%s", want, body)
		}
	}
}

// TestRunDirectoryNode verifies that a Directory node generates directory frontmatter.
func TestRunDirectoryNode(t *testing.T) {
	nodes := []Node{
		{
			ID:     "dir:internal/api",
			Labels: []string{"Directory"},
			Properties: map[string]interface{}{
				"name": "api",
				"path": "internal/api",
			},
		},
	}
	graphFile := buildGraphJSON(t, nodes, nil)
	outDir := t.TempDir()
	if err := Run(graphFile, outDir, "myrepo", "", 0); err != nil {
		t.Fatalf("Run: %v", err)
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 output file, got %d", len(entries))
	}
	content, _ := os.ReadFile(filepath.Join(outDir, entries[0].Name()))
	body := string(content)
	for _, want := range []string{`node_type: "Directory"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in directory output:\n%s", want, body)
		}
	}
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
