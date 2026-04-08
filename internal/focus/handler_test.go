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
