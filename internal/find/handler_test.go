package find

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/ui"
)

// ── search ────────────────────────────────────────────────────────────────────

func TestSearch_BasicMatch(t *testing.T) {
	g := makeGraph()
	matches := search(g, "handler", "")
	if len(matches) == 0 {
		t.Fatal("expected matches for 'handler'")
	}
	for _, m := range matches {
		if !strings.Contains(strings.ToLower(m.Name), "handler") {
			t.Errorf("match %q does not contain 'handler'", m.Name)
		}
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	g := makeGraph()
	lower := search(g, "handler", "")
	upper := search(g, "HANDLER", "")
	if len(lower) != len(upper) {
		t.Errorf("case-insensitive: lower=%d upper=%d", len(lower), len(upper))
	}
}

func TestSearch_KindFilter(t *testing.T) {
	g := makeGraph()
	// Filter to Function only
	matches := search(g, "handler", "Function")
	for _, m := range matches {
		if m.Kind != "Function" {
			t.Errorf("kind filter: expected Function, got %q", m.Kind)
		}
	}
}

func TestSearch_KindFilterExcludesOtherKinds(t *testing.T) {
	g := makeGraph()
	// Only File nodes matching "handler"
	matches := search(g, "handler", "File")
	for _, m := range matches {
		if m.Kind != "File" {
			t.Errorf("kind filter File: got %q", m.Kind)
		}
	}
}

func TestSearch_NoMatch(t *testing.T) {
	g := makeGraph()
	matches := search(g, "nonexistent_xyz_qrs", "")
	if len(matches) != 0 {
		t.Errorf("no-match: want 0, got %d", len(matches))
	}
}

func TestSearch_SortedByKindThenName(t *testing.T) {
	g := makeGraph()
	matches := search(g, "a", "") // broad match
	for i := 1; i < len(matches); i++ {
		if matches[i-1].Kind > matches[i].Kind {
			t.Errorf("not sorted by kind: %q > %q", matches[i-1].Kind, matches[i].Kind)
		}
		if matches[i-1].Kind == matches[i].Kind && matches[i-1].Name > matches[i].Name {
			t.Errorf("not sorted by name within kind: %q > %q", matches[i-1].Name, matches[i].Name)
		}
	}
}

func TestSearch_SharedFixtureCallers(t *testing.T) {
	// makeGraph() has fn3("main") --calls--> fn1("handleRequest").
	// Searching for "handleRequest" should show "main" as a caller.
	// This test guards against the makeGraph() fixture using uppercase CALLS.
	g := makeGraph()
	matches := search(g, "handleRequest", "")
	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	m := matches[0]
	if len(m.Callers) != 1 || m.Callers[0] != "main" {
		t.Errorf("callers: want [main], got %v", m.Callers)
	}
}

func TestSearch_CallersAndCallees(t *testing.T) {
	// caller calls target calls callee
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "caller", Labels: []string{"Function"}, Properties: map[string]any{"name": "caller"}},
			{ID: "target", Labels: []string{"Function"}, Properties: map[string]any{"name": "target"}},
			{ID: "callee", Labels: []string{"Function"}, Properties: map[string]any{"name": "callee"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "calls", StartNode: "caller", EndNode: "target"},
			{ID: "r2", Type: "calls", StartNode: "target", EndNode: "callee"},
		},
	}
	matches := search(g, "target", "")
	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	m := matches[0]
	if len(m.Callers) != 1 || m.Callers[0] != "caller" {
		t.Errorf("callers: want [caller], got %v", m.Callers)
	}
	if len(m.Callees) != 1 || m.Callees[0] != "callee" {
		t.Errorf("callees: want [callee], got %v", m.Callees)
	}
}

func TestSearch_CallersDeduplicatedWhenSameCallerCallsMultipleTimes(t *testing.T) {
	// "main" calls "target" via two separate call edges (two different call sites).
	// The callers list should show "main" only once.
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "main", Labels: []string{"Function"}, Properties: map[string]any{"name": "main"}},
			{ID: "target", Labels: []string{"Function"}, Properties: map[string]any{"name": "target"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "calls", StartNode: "main", EndNode: "target"},
			{ID: "r2", Type: "calls", StartNode: "main", EndNode: "target"}, // second call site
		},
	}
	matches := search(g, "target", "")
	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if len(matches[0].Callers) != 1 || matches[0].Callers[0] != "main" {
		t.Errorf("callers: want [main] (deduped), got %v", matches[0].Callers)
	}
}

func TestSearch_DefinesFunction(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "file1", Labels: []string{"File"}, Properties: map[string]any{"path": "auth/handler.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "authenticate"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "defines_function", StartNode: "file1", EndNode: "fn1"},
		},
	}
	matches := search(g, "authenticate", "Function")
	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if matches[0].DefinedIn != "auth/handler.go" {
		t.Errorf("defined_in: want 'auth/handler.go', got %q", matches[0].DefinedIn)
	}
}

// ── printMatches ──────────────────────────────────────────────────────────────

func TestPrintMatches_JSON(t *testing.T) {
	matches := []Match{
		{ID: "n1", Kind: "Function", Name: "handleAuth", File: "auth/handler.go"},
		{ID: "n2", Kind: "File", Name: "main.go", File: "main.go"},
	}
	var buf bytes.Buffer
	if err := printMatches(&buf, matches, "handle", ui.FormatJSON); err != nil {
		t.Fatalf("printMatches JSON: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if len(decoded) != 2 {
		t.Errorf("want 2 items, got %d", len(decoded))
	}
}

func TestPrintMatches_Human(t *testing.T) {
	matches := []Match{
		{ID: "n1", Kind: "Function", Name: "handleAuth", File: "auth/handler.go", Callers: []string{"main"}},
	}
	var buf bytes.Buffer
	if err := printMatches(&buf, matches, "handle", ui.FormatHuman); err != nil {
		t.Fatalf("printMatches human: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Function", "handleAuth", "auth/handler.go", "main"} {
		if !strings.Contains(out, want) {
			t.Errorf("should contain %q:\n%s", want, out)
		}
	}
}

func TestPrintMatches_HumanNoFile(t *testing.T) {
	matches := []Match{
		{ID: "n1", Kind: "Function", Name: "doThing"},
	}
	var buf bytes.Buffer
	if err := printMatches(&buf, matches, "doThing", ui.FormatHuman); err != nil {
		t.Fatalf("printMatches: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "doThing") {
		t.Errorf("should contain function name:\n%s", out)
	}
}

func TestPrintMatches_HumanShowsMatchCount(t *testing.T) {
	matches := []Match{
		{ID: "n1", Kind: "Function", Name: "foo"},
	}
	var buf bytes.Buffer
	if err := printMatches(&buf, matches, "fo", ui.FormatHuman); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "1 match") {
		t.Errorf("should show match count:\n%s", out)
	}
	// Verify the query (not the first match name) is shown in the summary.
	if !strings.Contains(out, `"fo"`) {
		t.Errorf("should show original query 'fo' in summary, not match name:\n%s", out)
	}
}

// ── dedupSorted ───────────────────────────────────────────────────────────────

func TestDedupSorted_Basic(t *testing.T) {
	got := dedupSorted([]string{"c", "a", "b", "a"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestDedupSorted_Empty(t *testing.T) {
	if got := dedupSorted(nil); got != nil {
		t.Errorf("nil input: want nil, got %v", got)
	}
	if got := dedupSorted([]string{}); got != nil {
		t.Errorf("empty input: want nil, got %v", got)
	}
}

func TestDedupSorted_DoesNotMutateInput(t *testing.T) {
	// Prior bug: out := ss[:1] shared the backing array, so appends overwrote
	// the original slice. Verify the input is unchanged after dedupSorted.
	input := []string{"b", "a", "c", "a"}
	snapshot := make([]string, len(input))
	copy(snapshot, input)
	dedupSorted(input)
	for i, v := range snapshot {
		if input[i] != v {
			t.Errorf("input mutated at index %d: was %q, now %q", i, v, input[i])
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func makeGraph() *api.Graph {
	return &api.Graph{
		Nodes: []api.Node{
			{ID: "file1", Labels: []string{"File"}, Properties: map[string]any{"path": "auth/handler.go"}},
			{ID: "file2", Labels: []string{"File"}, Properties: map[string]any{"path": "main.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "handleRequest"}},
			{ID: "fn2", Labels: []string{"Function"}, Properties: map[string]any{"name": "parseToken"}},
			{ID: "fn3", Labels: []string{"Function"}, Properties: map[string]any{"name": "main"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "calls", StartNode: "fn3", EndNode: "fn1"},
		},
	}
}
