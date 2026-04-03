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
