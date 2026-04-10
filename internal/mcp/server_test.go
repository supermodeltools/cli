package mcp

import (
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

func TestFormatDeadCode_Empty(t *testing.T) {
	result := &api.DeadCodeResult{}
	got := formatDeadCode(result)
	if got != "No dead code detected." {
		t.Errorf("expected 'No dead code detected.', got: %s", got)
	}
}

func TestFormatDeadCode_WithCandidates(t *testing.T) {
	result := &api.DeadCodeResult{
		Metadata: api.DeadCodeMetadata{TotalDeclarations: 100, DeadCodeCandidates: 2},
		DeadCodeCandidates: []api.DeadCodeCandidate{
			{File: "src/a.ts", Line: 10, Name: "unused", Confidence: "high", Reason: "No callers"},
			{File: "src/b.ts", Line: 42, Name: "old", Confidence: "medium", Reason: "Transitively dead"},
		},
	}
	got := formatDeadCode(result)
	for _, want := range []string{
		"2 dead code candidate(s) out of 100 total declarations",
		"[high] src/a.ts:10 unused — No callers",
		"[medium] src/b.ts:42 old — Transitively dead",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestFormatImpact_Empty(t *testing.T) {
	result := &api.ImpactResult{}
	got := formatImpact(result)
	if got != "No impact detected." {
		t.Errorf("expected 'No impact detected.', got: %s", got)
	}
}

func TestFormatImpact_GlobalCouplingMap(t *testing.T) {
	result := &api.ImpactResult{
		GlobalMetrics: api.ImpactGlobalMetrics{
			MostCriticalFiles: []api.CriticalFileMetric{
				{File: "src/db.ts", DependentCount: 42},
			},
		},
	}
	got := formatImpact(result)
	if !strings.Contains(got, "Most critical files") {
		t.Errorf("expected global coupling header, got:\n%s", got)
	}
	if !strings.Contains(got, "src/db.ts (42 dependents)") {
		t.Errorf("expected file with count, got:\n%s", got)
	}
}

func TestFormatImpact_WithTarget(t *testing.T) {
	result := &api.ImpactResult{
		Metadata: api.ImpactMetadata{TargetsAnalyzed: 1, TotalFiles: 100, TotalFunctions: 500},
		Impacts: []api.ImpactTarget{
			{
				Target: api.ImpactTargetInfo{File: "src/auth.ts", Type: "file"},
				BlastRadius: api.BlastRadius{
					DirectDependents: 10, TransitiveDependents: 30, AffectedFiles: 5,
					RiskScore: "high", RiskFactors: []string{"High fan-in"},
				},
				AffectedFiles: []api.AffectedFile{
					{File: "src/routes.ts", DirectDependencies: 3, TransitiveDependencies: 7},
				},
				EntryPointsAffected: []api.AffectedEntryPoint{
					{File: "src/routes.ts", Name: "/login", Type: "route_handler"},
				},
			},
		},
	}
	got := formatImpact(result)
	for _, want := range []string{
		"Target: src/auth.ts",
		"Risk: high",
		"Direct: 10",
		"Transitive: 30",
		"High fan-in",
		"src/routes.ts (direct: 3, transitive: 7)",
		"/login (route_handler)",
		"1 target(s) analyzed across 100 files and 500 functions",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestFilterGraph_NoFilter(t *testing.T) {
	g := makeTestGraph()
	result := filterGraph(g, "", "")
	if len(result.Nodes) != len(g.Nodes) {
		t.Errorf("no filter: want %d nodes, got %d", len(g.Nodes), len(result.Nodes))
	}
	if len(result.Relationships) != len(g.Rels()) {
		t.Errorf("no filter: want %d rels, got %d", len(g.Rels()), len(result.Relationships))
	}
}

func TestFilterGraph_LabelOnly(t *testing.T) {
	g := makeTestGraph()
	result := filterGraph(g, "File", "")
	for _, n := range result.Nodes {
		if !n.HasLabel("File") {
			t.Errorf("label filter: expected only File nodes, got label %v", n.Labels)
		}
	}
	// Relationships must only reference nodes that are in the result.
	visible := make(map[string]bool)
	for _, n := range result.Nodes {
		visible[n.ID] = true
	}
	for _, r := range result.Relationships {
		if !visible[r.StartNode] || !visible[r.EndNode] {
			t.Errorf("label filter: relationship %s→%s references a node not in the filtered set", r.StartNode, r.EndNode)
		}
	}
}

func TestFilterGraph_LabelExcludesCrossLabelRels(t *testing.T) {
	// File nodes + Function nodes; one file→file rel, one function→function rel.
	// Filtering by File should yield only the file→file rel.
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "foo"}},
			{ID: "fn2", Labels: []string{"Function"}, Properties: map[string]any{"name": "bar"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "f1", EndNode: "f2"},
			{ID: "r2", Type: "calls", StartNode: "fn1", EndNode: "fn2"},
		},
	}
	result := filterGraph(g, "File", "")
	if len(result.Relationships) != 1 {
		t.Errorf("want 1 rel (file→file), got %d", len(result.Relationships))
	}
	if len(result.Relationships) > 0 && result.Relationships[0].ID != "r1" {
		t.Errorf("want rel r1 (imports), got %s", result.Relationships[0].ID)
	}
}

func TestFilterGraph_RelTypeOnly(t *testing.T) {
	g := makeTestGraph()
	result := filterGraph(g, "", "calls")
	for _, r := range result.Relationships {
		if r.Type != "calls" {
			t.Errorf("relType filter: expected only 'calls', got %q", r.Type)
		}
	}
	// All nodes should be returned when only relType is filtered.
	if len(result.Nodes) != len(g.Nodes) {
		t.Errorf("relType filter: want all %d nodes, got %d", len(g.Nodes), len(result.Nodes))
	}
}

func TestFilterGraph_BothFilters(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "foo"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "f1", EndNode: "f2"},
			{ID: "r2", Type: "calls", StartNode: "fn1", EndNode: "f2"},        // fn→file, excluded by label filter
			{ID: "r3", Type: "contains_call", StartNode: "f1", EndNode: "f2"}, // file→file, wrong relType
		},
	}
	result := filterGraph(g, "File", "imports")
	if len(result.Nodes) != 2 {
		t.Errorf("both filters: want 2 File nodes, got %d", len(result.Nodes))
	}
	if len(result.Relationships) != 1 || result.Relationships[0].ID != "r1" {
		t.Errorf("both filters: want only r1 (imports between Files), got %v", result.Relationships)
	}
}

// makeTestGraph builds a small mixed graph for filter tests.
func makeTestGraph() *api.Graph {
	return &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "handleReq"}},
			{ID: "fn2", Labels: []string{"Function"}, Properties: map[string]any{"name": "parse"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "f1", EndNode: "f2"},
			{ID: "r2", Type: "calls", StartNode: "fn1", EndNode: "fn2"},
			{ID: "r3", Type: "defines_function", StartNode: "f1", EndNode: "fn1"},
		},
	}
}

// ── boolArg / intArg ──────────────────────────────────────────────────────────

func TestBoolArg(t *testing.T) {
	args := map[string]any{"flag": true, "off": false, "num": 42}
	if !boolArg(args, "flag") {
		t.Error("boolArg(flag=true) should return true")
	}
	if boolArg(args, "off") {
		t.Error("boolArg(off=false) should return false")
	}
	if boolArg(args, "num") {
		t.Error("boolArg(num=42) should return false (wrong type)")
	}
	if boolArg(args, "absent") {
		t.Error("boolArg(absent) should return false")
	}
}

func TestIntArg(t *testing.T) {
	args := map[string]any{"count": float64(5), "zero": float64(0), "str": "hello"}
	if got := intArg(args, "count"); got != 5 {
		t.Errorf("intArg(count=5.0) = %d, want 5", got)
	}
	if got := intArg(args, "zero"); got != 0 {
		t.Errorf("intArg(zero=0.0) = %d, want 0", got)
	}
	if got := intArg(args, "str"); got != 0 {
		t.Errorf("intArg(str='hello') = %d, want 0 (wrong type)", got)
	}
	if got := intArg(args, "absent"); got != 0 {
		t.Errorf("intArg(absent) = %d, want 0", got)
	}
}

func TestFormatImpact_NoEntryPoints(t *testing.T) {
	result := &api.ImpactResult{
		Metadata: api.ImpactMetadata{TargetsAnalyzed: 1, TotalFiles: 50, TotalFunctions: 200},
		Impacts: []api.ImpactTarget{
			{
				Target:      api.ImpactTargetInfo{File: "src/util.ts", Type: "file"},
				BlastRadius: api.BlastRadius{DirectDependents: 2, RiskScore: "low"},
			},
		},
	}
	got := formatImpact(result)
	if strings.Contains(got, "Entry points") {
		t.Error("should not contain entry points section when none affected")
	}
}
