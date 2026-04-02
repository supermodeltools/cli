package blastradius

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/ui"
)

func sampleImpact() *api.ImpactResult {
	return &api.ImpactResult{
		Metadata: api.ImpactMetadata{
			TotalFiles:      100,
			TotalFunctions:  500,
			TargetsAnalyzed: 1,
			AnalysisMethod:  "call_graph + dependency_graph",
		},
		Impacts: []api.ImpactTarget{
			{
				Target: api.ImpactTargetInfo{File: "src/auth/login.ts", Type: "file"},
				BlastRadius: api.BlastRadius{
					DirectDependents:     3,
					TransitiveDependents: 7,
					AffectedFiles:        5,
					RiskScore:            "high",
					RiskFactors:          []string{"Affects authentication flow"},
				},
				AffectedFiles: []api.AffectedFile{
					{File: "src/api/routes.ts", DirectDependencies: 2, TransitiveDependencies: 0},
					{File: "src/middleware/auth.ts", DirectDependencies: 1, TransitiveDependencies: 3},
				},
				EntryPointsAffected: []api.AffectedEntryPoint{
					{File: "src/api/routes.ts", Name: "/api/login", Type: "route_handler"},
				},
			},
		},
	}
}

func TestPrintResults_Human(t *testing.T) {
	var buf bytes.Buffer
	if err := printResults(&buf, sampleImpact(), ui.FormatHuman); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"src/auth/login.ts",
		"high",
		"Direct: 3",
		"Transitive: 7",
		"Affects authentication flow",
		"src/api/routes.ts",
		"/api/login",
		"route_handler",
		"1 target(s) analyzed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestPrintResults_Empty(t *testing.T) {
	result := &api.ImpactResult{
		Metadata: api.ImpactMetadata{TotalFiles: 100},
	}
	var buf bytes.Buffer
	if err := printResults(&buf, result, ui.FormatHuman); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No impact detected") {
		t.Errorf("expected 'No impact detected', got:\n%s", buf.String())
	}
}

func TestPrintResults_JSON(t *testing.T) {
	var buf bytes.Buffer
	if err := printResults(&buf, sampleImpact(), ui.FormatJSON); err != nil {
		t.Fatal(err)
	}
	var decoded api.ImpactResult
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(decoded.Impacts) != 1 {
		t.Errorf("expected 1 impact, got %d", len(decoded.Impacts))
	}
	if decoded.Impacts[0].BlastRadius.RiskScore != "high" {
		t.Errorf("expected risk=high, got %q", decoded.Impacts[0].BlastRadius.RiskScore)
	}
}

func TestPrintResults_GlobalCouplingMap(t *testing.T) {
	result := &api.ImpactResult{
		Metadata: api.ImpactMetadata{TotalFiles: 100},
		GlobalMetrics: api.ImpactGlobalMetrics{
			MostCriticalFiles: []api.CriticalFileMetric{
				{File: "src/core/db.ts", DependentCount: 42},
				{File: "src/core/auth.ts", DependentCount: 31},
			},
		},
	}
	var buf bytes.Buffer
	if err := printResults(&buf, result, ui.FormatHuman); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Most critical files") {
		t.Errorf("expected global coupling header, got:\n%s", out)
	}
	if !strings.Contains(out, "42") {
		t.Errorf("expected dependent count 42, got:\n%s", out)
	}
}
