package focus

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

// ── pathMatches ────────────────────────────────────────────────────────────────

func TestPathMatches_Exact(t *testing.T) {
	if !pathMatches("auth/handler.go", "auth/handler.go") {
		t.Error("exact path should match")
	}
}

func TestPathMatches_WithDotSlashPrefix(t *testing.T) {
	if !pathMatches("./auth/handler.go", "auth/handler.go") {
		t.Error("leading ./ should be stripped before comparison")
	}
}

func TestPathMatches_SuffixMatch(t *testing.T) {
	if !pathMatches("/repo/auth/handler.go", "auth/handler.go") {
		t.Error("absolute path should match via suffix")
	}
}

func TestPathMatches_NoMatch(t *testing.T) {
	if pathMatches("other/handler.go", "auth/handler.go") {
		t.Error("different dirs should not match")
	}
}

// ── extract ────────────────────────────────────────────────────────────────────

func makeTestGraph() *api.Graph {
	return &api.Graph{
		Nodes: []api.Node{
			{ID: "file-auth", Labels: []string{"File"}, Properties: map[string]any{"path": "auth/handler.go"}},
			{ID: "file-main", Labels: []string{"File"}, Properties: map[string]any{"path": "main.go"}},
			{ID: "file-util", Labels: []string{"File"}, Properties: map[string]any{"path": "util/util.go"}},
			{ID: "fn-auth", Labels: []string{"Function"}, Properties: map[string]any{"name": "Authenticate", "filePath": "auth/handler.go"}},
			{ID: "fn-main", Labels: []string{"Function"}, Properties: map[string]any{"name": "main", "filePath": "main.go"}},
			{ID: "fn-util", Labels: []string{"Function"}, Properties: map[string]any{"name": "Hash", "filePath": "util/util.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "defines_function", StartNode: "file-auth", EndNode: "fn-auth"},
			{ID: "r2", Type: "defines_function", StartNode: "file-main", EndNode: "fn-main"},
			{ID: "r3", Type: "defines_function", StartNode: "file-util", EndNode: "fn-util"},
			{ID: "r4", Type: "imports", StartNode: "file-auth", EndNode: "file-util"},
			{ID: "r5", Type: "calls", StartNode: "fn-main", EndNode: "fn-auth"},
			{ID: "r6", Type: "calls", StartNode: "fn-auth", EndNode: "fn-util"},
		},
	}
}

func TestExtract_FileFound(t *testing.T) {
	g := makeTestGraph()
	sl := extract(g, "auth/handler.go", 1, false)
	if sl == nil {
		t.Fatal("expected non-nil slice for known file")
	}
	if sl.File != "auth/handler.go" {
		t.Errorf("file: want 'auth/handler.go', got %q", sl.File)
	}
}

func TestExtract_FileNotFound(t *testing.T) {
	g := makeTestGraph()
	sl := extract(g, "nonexistent.go", 1, false)
	if sl != nil {
		t.Error("expected nil for unknown file")
	}
}

func TestExtract_ImportsPopulated(t *testing.T) {
	g := makeTestGraph()
	sl := extract(g, "auth/handler.go", 1, false)
	if sl == nil {
		t.Fatal("nil slice")
	}
	if len(sl.Imports) != 1 || sl.Imports[0] != "util/util.go" {
		t.Errorf("imports: want [util/util.go], got %v", sl.Imports)
	}
}

func TestExtract_FunctionsPopulated(t *testing.T) {
	g := makeTestGraph()
	sl := extract(g, "auth/handler.go", 1, false)
	if sl == nil {
		t.Fatal("nil slice")
	}
	if len(sl.Functions) != 1 {
		t.Fatalf("functions: want 1, got %d", len(sl.Functions))
	}
	if sl.Functions[0].Name != "Authenticate" {
		t.Errorf("function name: want 'Authenticate', got %q", sl.Functions[0].Name)
	}
}

func TestExtract_FunctionCallees(t *testing.T) {
	g := makeTestGraph()
	sl := extract(g, "auth/handler.go", 1, false)
	if sl == nil {
		t.Fatal("nil slice")
	}
	fn := sl.Functions[0]
	if len(fn.Callees) != 1 || fn.Callees[0] != "Hash" {
		t.Errorf("callees: want [Hash], got %v", fn.Callees)
	}
}

func TestExtract_CalledByPopulated(t *testing.T) {
	g := makeTestGraph()
	sl := extract(g, "auth/handler.go", 1, false)
	if sl == nil {
		t.Fatal("nil slice")
	}
	if len(sl.CalledBy) != 1 {
		t.Fatalf("called_by: want 1, got %d: %+v", len(sl.CalledBy), sl.CalledBy)
	}
	if sl.CalledBy[0].Caller != "main" {
		t.Errorf("caller name: want 'main', got %q", sl.CalledBy[0].Caller)
	}
}

func TestExtract_CalledBySelfCallsExcluded(t *testing.T) {
	// Self-calls (same file) should not appear in CalledBy.
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "file-a", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "fn-a1", Labels: []string{"Function"}, Properties: map[string]any{"name": "A", "filePath": "a.go"}},
			{ID: "fn-a2", Labels: []string{"Function"}, Properties: map[string]any{"name": "B", "filePath": "a.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "defines_function", StartNode: "file-a", EndNode: "fn-a1"},
			{ID: "r2", Type: "defines_function", StartNode: "file-a", EndNode: "fn-a2"},
			{ID: "r3", Type: "calls", StartNode: "fn-a1", EndNode: "fn-a2"},
		},
	}
	sl := extract(g, "a.go", 1, false)
	if sl == nil {
		t.Fatal("nil slice")
	}
	if len(sl.CalledBy) != 0 {
		t.Errorf("self-calls should be excluded from CalledBy, got: %v", sl.CalledBy)
	}
}

func TestExtract_RelTypesCaseInsensitive(t *testing.T) {
	// Relationship types must be lowercase as returned by the API.
	// Verify that uppercase variants do NOT produce results (i.e. the handler
	// correctly uses lowercase — a regression guard for the bug fixed in #68).
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "file-a", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "fn-a", Labels: []string{"Function"}, Properties: map[string]any{"name": "Foo", "filePath": "a.go"}},
			{ID: "file-b", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
			{ID: "fn-b", Labels: []string{"Function"}, Properties: map[string]any{"name": "Bar", "filePath": "b.go"}},
		},
		Relationships: []api.Relationship{
			// Use uppercase — these should NOT match, confirming lowercase is expected.
			{ID: "r1", Type: "DEFINES_FUNCTION", StartNode: "file-a", EndNode: "fn-a"},
			{ID: "r2", Type: "CALLS", StartNode: "fn-b", EndNode: "fn-a"},
		},
	}
	sl := extract(g, "a.go", 1, false)
	if sl == nil {
		t.Fatal("nil slice — file should still be found even with uppercase rels")
	}
	if len(sl.Functions) != 0 {
		t.Errorf("uppercase DEFINES_FUNCTION should not match: got %v", sl.Functions)
	}
	if len(sl.CalledBy) != 0 {
		t.Errorf("uppercase CALLS should not match: got %v", sl.CalledBy)
	}
}

func TestExtract_LowercaseRelTypesWork(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "file-a", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "fn-a", Labels: []string{"Function"}, Properties: map[string]any{"name": "Foo", "filePath": "a.go"}},
			{ID: "file-b", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
			{ID: "fn-b", Labels: []string{"Function"}, Properties: map[string]any{"name": "Bar", "filePath": "b.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "defines_function", StartNode: "file-a", EndNode: "fn-a"},
			{ID: "r2", Type: "calls", StartNode: "fn-b", EndNode: "fn-a"},
		},
	}
	sl := extract(g, "a.go", 1, false)
	if sl == nil {
		t.Fatal("nil slice")
	}
	if len(sl.Functions) != 1 || sl.Functions[0].Name != "Foo" {
		t.Errorf("lowercase defines_function: want [Foo], got %v", sl.Functions)
	}
	if len(sl.CalledBy) != 1 || sl.CalledBy[0].Caller != "Bar" {
		t.Errorf("lowercase calls: want [Bar], got %v", sl.CalledBy)
	}
}

func TestExtract_DepthZeroDefaultsToOne(t *testing.T) {
	g := makeTestGraph()
	// depth=0 should be treated as depth=1 (imports one hop)
	sl := extract(g, "auth/handler.go", 0, false)
	if sl == nil {
		t.Fatal("nil slice")
	}
	// depth=0 is handled in Run() (converted to 1 before calling extract).
	// extract itself uses the value passed; depth=0 means zero BFS hops = no imports.
	// This test documents the current behaviour.
	if sl.Imports == nil {
		sl.Imports = []string{} // normalize nil vs empty
	}
	// With depth=0, BFS loop doesn't run, so no imports.
	if len(sl.Imports) != 0 {
		t.Errorf("depth=0 should produce no imports, got %v", sl.Imports)
	}
}

func TestExtract_TokenHintPositive(t *testing.T) {
	g := makeTestGraph()
	sl := extract(g, "auth/handler.go", 1, false)
	if sl == nil {
		t.Fatal("nil slice")
	}
	if sl.TokenHint < 0 {
		t.Errorf("token hint should be non-negative, got %d", sl.TokenHint)
	}
}

// ── render (printMarkdown / JSON) ──────────────────────────────────────────────

func TestRender_JSON(t *testing.T) {
	sl := &Slice{
		File:      "auth/handler.go",
		Imports:   []string{"util/util.go"},
		Functions: []Function{{Name: "Authenticate", Callees: []string{"Hash"}}},
		CalledBy:  []Call{{Caller: "main", File: "main.go"}},
	}
	var buf bytes.Buffer
	if err := render(&buf, sl, "json"); err != nil {
		t.Fatalf("render JSON: %v", err)
	}
	var decoded Slice
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if decoded.File != "auth/handler.go" {
		t.Errorf("file: want 'auth/handler.go', got %q", decoded.File)
	}
}

func TestRender_Markdown(t *testing.T) {
	sl := &Slice{
		File:      "auth/handler.go",
		Imports:   []string{"util/util.go"},
		Functions: []Function{{Name: "Authenticate", Callees: []string{"Hash"}}},
		CalledBy:  []Call{{Caller: "main", File: "main.go"}},
		TokenHint: 42,
	}
	var buf bytes.Buffer
	if err := render(&buf, sl, "markdown"); err != nil {
		t.Fatalf("render markdown: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"auth/handler.go", "util/util.go", "Authenticate", "Hash", "main"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}

func TestRender_MarkdownNoCalledBy(t *testing.T) {
	sl := &Slice{
		File:      "main.go",
		Functions: []Function{{Name: "main"}},
	}
	var buf bytes.Buffer
	if err := render(&buf, sl, ""); err != nil {
		t.Fatalf("render markdown: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "Called by") {
		t.Errorf("should not include 'Called by' section when empty:\n%s", out)
	}
}

func TestRender_MarkdownTokenHint(t *testing.T) {
	sl := &Slice{File: "a.go", TokenHint: 123}
	var buf bytes.Buffer
	if err := render(&buf, sl, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "123 tokens") {
		t.Errorf("should show token hint:\n%s", buf.String())
	}
}

func TestRender_MarkdownCalledByNoCallerName(t *testing.T) {
	// CalledBy with empty Caller (only File) covers the else branch in printMarkdown.
	sl := &Slice{
		File:     "util.go",
		CalledBy: []Call{{Caller: "", File: "main.go"}},
	}
	var buf bytes.Buffer
	if err := render(&buf, sl, ""); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Called by") {
		t.Errorf("should have 'Called by' section:\n%s", out)
	}
	if !strings.Contains(out, "main.go") {
		t.Errorf("should contain caller file name:\n%s", out)
	}
}

func TestRender_MarkdownTypes(t *testing.T) {
	sl := &Slice{
		File:  "models.go",
		Types: []Type{{Name: "User", Kind: "class"}, {Name: "ID", Kind: "type"}},
	}
	var buf bytes.Buffer
	if err := render(&buf, sl, ""); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "### Types") {
		t.Errorf("should have 'Types' section:\n%s", out)
	}
	if !strings.Contains(out, "User") || !strings.Contains(out, "class") {
		t.Errorf("should show type name and kind:\n%s", out)
	}
}

// ── extractTypes ──────────────────────────────────────────────────────────────

func TestExtractTypes(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "auth/handler.go"}},
			{ID: "cls1", Labels: []string{"Class"}, Properties: map[string]any{"name": "AuthService", "file": "auth/handler.go"}},
			{ID: "iface1", Labels: []string{"Interface"}, Properties: map[string]any{"name": "Authenticator", "file": "auth/handler.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "declares_class", StartNode: "f1", EndNode: "cls1"},
			{ID: "r2", Type: "defines", StartNode: "f1", EndNode: "iface1"},
		},
	}
	nodeByID := map[string]*api.Node{}
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	types := extractTypes(g, "f1", nodeByID, g.Rels())
	if len(types) != 2 {
		t.Fatalf("want 2 types, got %d: %v", len(types), types)
	}
	// Class should have kind "class"
	var foundClass bool
	for _, typ := range types {
		if typ.Name == "AuthService" && typ.Kind == "class" {
			foundClass = true
		}
	}
	if !foundClass {
		t.Errorf("should have AuthService with kind='class', got %v", types)
	}
}

func TestExtractTypes_OtherFileExcluded(t *testing.T) {
	// Relations from a different file should not appear
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
			{ID: "cls1", Labels: []string{"Class"}, Properties: map[string]any{"name": "Foo"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "declares_class", StartNode: "f2", EndNode: "cls1"}, // from f2, not f1
		},
	}
	nodeByID := map[string]*api.Node{}
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	types := extractTypes(g, "f1", nodeByID, g.Rels())
	if len(types) != 0 {
		t.Errorf("other file's types should not appear, got %v", types)
	}
}

// ── extract with includeTypes ─────────────────────────────────────────────────

func TestExtract_WithTypes(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "auth.go"}},
			{ID: "cls1", Labels: []string{"Class"}, Properties: map[string]any{"name": "AuthService"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "declares_class", StartNode: "f1", EndNode: "cls1"},
		},
	}
	sl := extract(g, "auth.go", 1, true)
	if sl == nil {
		t.Fatal("nil slice")
	}
	if len(sl.Types) != 1 || sl.Types[0].Name != "AuthService" {
		t.Errorf("types: got %v", sl.Types)
	}
}

// ── extract: caller node not in graph ────────────────────────────────────────

func TestExtract_CallerNodeMissing(t *testing.T) {
	// A "calls" relationship whose StartNode doesn't exist in the graph.
	// The callerNode == nil branch should be taken silently.
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "doWork", "filePath": "a.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "defines", StartNode: "f1", EndNode: "fn1"},
			// StartNode "missing-caller" is not in nodeByID
			{ID: "r2", Type: "calls", StartNode: "missing-caller", EndNode: "fn1"},
		},
	}
	sl := extract(g, "a.go", 1, false)
	if sl == nil {
		t.Fatal("nil slice")
	}
	// No CalledBy entries — the missing caller was silently skipped
	if len(sl.CalledBy) != 0 {
		t.Errorf("expected 0 CalledBy (missing caller skipped), got %v", sl.CalledBy)
	}
}

// ── reachableImports: wildcard_imports and empty prop ────────────────────────

func TestReachableImports_WildcardImports(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "wildcard_imports", StartNode: "f1", EndNode: "f2"},
		},
	}
	nodeByID := map[string]*api.Node{}
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	imports := reachableImports(g, "f1", nodeByID, g.Rels(), 1)
	found := false
	for _, imp := range imports {
		if imp == "b.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("wildcard_imports should be followed; got %v", imports)
	}
}

func TestReachableImports_NodeWithEmptyProp(t *testing.T) {
	// Node found but has no path/name/importPath → not added to imports
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "ext1", Labels: []string{"ExternalDependency"}, Properties: map[string]any{}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "f1", EndNode: "ext1"},
		},
	}
	nodeByID := map[string]*api.Node{}
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	imports := reachableImports(g, "f1", nodeByID, g.Rels(), 1)
	if len(imports) != 0 {
		t.Errorf("node with empty prop should not be added to imports; got %v", imports)
	}
}

// ── extractTypes: dangling EndNode ────────────────────────────────────────────

func TestExtractTypes_DanglingEndNode(t *testing.T) {
	// Relationship points to a node not in nodeByID → n == nil → continue
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "defines", StartNode: "f1", EndNode: "missing-type"},
		},
	}
	nodeByID := map[string]*api.Node{}
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	types := extractTypes(g, "f1", nodeByID, g.Rels())
	if len(types) != 0 {
		t.Errorf("dangling endNode should be skipped; got %v", types)
	}
}

func TestExtractTypes_NonClassKind(t *testing.T) {
	// A "defines" rel to a non-Class node → kind stays "type"
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "t1", Labels: []string{"Type"}, Properties: map[string]any{"name": "MyStruct"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "defines", StartNode: "f1", EndNode: "t1"},
		},
	}
	nodeByID := map[string]*api.Node{}
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	types := extractTypes(g, "f1", nodeByID, g.Rels())
	if len(types) != 1 || types[0].Kind != "type" {
		t.Errorf("non-Class node should have kind='type', got %v", types)
	}
}

// ── extract: fn node missing from nodeByID ───────────────────────────────────

func TestExtract_FnNodeMissing(t *testing.T) {
	// defines relationship references a fn ID not in the graph → fn == nil → skip
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "defines", StartNode: "f1", EndNode: "missing-fn"},
		},
	}
	sl := extract(g, "a.go", 1, false)
	if sl == nil {
		t.Fatal("nil slice")
	}
	if len(sl.Functions) != 0 {
		t.Errorf("missing fn node should be skipped; got %v", sl.Functions)
	}
}

// ── reachableImports: cycle detection ────────────────────────────────────────

func TestReachableImports_CycleSkipped(t *testing.T) {
	// f1 imports f2, f2 imports f1 → cycle; f1 is already visited so second visit skipped
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "f1", EndNode: "f2"},
			{ID: "r2", Type: "imports", StartNode: "f2", EndNode: "f1"}, // cycle back to f1
		},
	}
	nodeByID := map[string]*api.Node{}
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	// depth=2 so we traverse both hops; the cycle back to f1 should be skipped
	imports := reachableImports(g, "f1", nodeByID, g.Rels(), 2)
	// Only b.go should appear (a.go is the seed, not imported)
	if len(imports) != 1 || imports[0] != "b.go" {
		t.Errorf("cycle: expected [b.go], got %v", imports)
	}
}

// TestExtract_DuplicateCaller covers L150: seenCallers deduplication.
// When the same external function calls two different functions in the target file,
// the caller should only appear once in CalledBy.
func TestExtract_DuplicateCaller(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "fn-a1", Labels: []string{"Function"}, Properties: map[string]any{"name": "FuncA", "filePath": "a.go"}},
			{ID: "fn-a2", Labels: []string{"Function"}, Properties: map[string]any{"name": "FuncB", "filePath": "a.go"}},
			{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
			{ID: "fn-b", Labels: []string{"Function"}, Properties: map[string]any{"name": "Caller", "filePath": "b.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "defines_function", StartNode: "f1", EndNode: "fn-a1"},
			{ID: "r2", Type: "defines_function", StartNode: "f1", EndNode: "fn-a2"},
			// Same caller calls BOTH functions in the target file.
			{ID: "r3", Type: "calls", StartNode: "fn-b", EndNode: "fn-a1"},
			{ID: "r4", Type: "calls", StartNode: "fn-b", EndNode: "fn-a2"},
		},
	}
	sl := extract(g, "a.go", 1, false)
	if sl == nil {
		t.Fatal("nil slice")
	}
	// Caller should appear exactly once even though they call two functions.
	if len(sl.CalledBy) != 1 {
		t.Errorf("duplicate caller should be deduplicated; got %d callers: %v", len(sl.CalledBy), sl.CalledBy)
	}
}

// TestExtract_MultipleCallersSorted covers L159: sort.Slice on sl.CalledBy
// with multiple callers from different files.
func TestExtract_MultipleCallersSorted(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "fn-target", Labels: []string{"Function"}, Properties: map[string]any{"name": "Target", "filePath": "a.go"}},
			{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"path": "z.go"}},
			{ID: "fn-z", Labels: []string{"Function"}, Properties: map[string]any{"name": "ZCaller", "filePath": "z.go"}},
			{ID: "f3", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
			{ID: "fn-b", Labels: []string{"Function"}, Properties: map[string]any{"name": "BCaller", "filePath": "b.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "defines_function", StartNode: "f1", EndNode: "fn-target"},
			{ID: "r2", Type: "calls", StartNode: "fn-z", EndNode: "fn-target"},
			{ID: "r3", Type: "calls", StartNode: "fn-b", EndNode: "fn-target"},
		},
	}
	sl := extract(g, "a.go", 1, false)
	if sl == nil {
		t.Fatal("nil slice")
	}
	if len(sl.CalledBy) != 2 {
		t.Fatalf("expected 2 callers, got %d: %v", len(sl.CalledBy), sl.CalledBy)
	}
	// Should be sorted by file: b.go before z.go
	if sl.CalledBy[0].File != "b.go" || sl.CalledBy[1].File != "z.go" {
		t.Errorf("callers should be sorted by file; got %v", sl.CalledBy)
	}
}

// TestExtractTypes_NonMatchingRelSkipped covers L237: a non-declares_class/defines
// relationship causes 'continue' in extractTypes.
func TestExtractTypes_NonMatchingRelSkipped(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "cls1", Labels: []string{"Class"}, Properties: map[string]any{"name": "MyClass"}},
		},
		Relationships: []api.Relationship{
			// Non-matching type → skipped (covers L237 continue branch)
			{ID: "r1", Type: "imports", StartNode: "f1", EndNode: "cls1"},
			// Matching type → included
			{ID: "r2", Type: "declares_class", StartNode: "f1", EndNode: "cls1"},
		},
	}
	nodeByID := map[string]*api.Node{}
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	types := extractTypes(g, "f1", nodeByID, g.Rels())
	if len(types) != 1 {
		t.Errorf("only declares_class/defines rels should be processed; got %v", types)
	}
}

// ── reachableImports: endNode not in nodeByID ────────────────────────────────

func TestReachableImports_EndNodeMissingFromGraph(t *testing.T) {
	// import edge points to a node not in nodeByID → n == nil → no prop appended
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "f1", EndNode: "ghost-node"},
		},
	}
	nodeByID := map[string]*api.Node{}
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	imports := reachableImports(g, "f1", nodeByID, g.Rels(), 1)
	if len(imports) != 0 {
		t.Errorf("ghost endNode should produce 0 imports; got %v", imports)
	}
}
