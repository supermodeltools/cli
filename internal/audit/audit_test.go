package audit

import (
	"strings"
	"testing"
	"time"

	"github.com/supermodeltools/cli/internal/api"
)

// ── CouplingStatus ────────────────────────────────────────────────────────────

func TestCouplingStatus_OK(t *testing.T) {
	d := &DomainHealth{IncomingDeps: []string{"A", "B"}}
	if got := d.CouplingStatus(); got != "✅ OK" {
		t.Errorf("2 deps: want '✅ OK', got %q", got)
	}
}

func TestCouplingStatus_Warn(t *testing.T) {
	d := &DomainHealth{IncomingDeps: []string{"A", "B", "C"}}
	if !strings.Contains(d.CouplingStatus(), "WARN") {
		t.Errorf("3 deps: expected WARN, got %q", d.CouplingStatus())
	}
}

func TestCouplingStatus_High(t *testing.T) {
	d := &DomainHealth{IncomingDeps: []string{"A", "B", "C", "D", "E"}}
	if !strings.Contains(d.CouplingStatus(), "HIGH") {
		t.Errorf("5 deps: expected HIGH, got %q", d.CouplingStatus())
	}
}

func TestCouplingStatus_Zero(t *testing.T) {
	d := &DomainHealth{}
	if got := d.CouplingStatus(); got != "✅ OK" {
		t.Errorf("0 deps: want '✅ OK', got %q", got)
	}
}

// ── pluralf ───────────────────────────────────────────────────────────────────

func TestPluralf_Singular(t *testing.T) {
	got := pluralf("Resolve %d cycle%s.", 1)
	if !strings.Contains(got, "1 cycle.") {
		t.Errorf("singular: want '1 cycle.', got %q", got)
	}
}

func TestPluralf_Plural(t *testing.T) {
	got := pluralf("Resolve %d cycle%s.", 3)
	if !strings.Contains(got, "3 cycles.") {
		t.Errorf("plural: want '3 cycles.', got %q", got)
	}
}

// ── scoreStatus ───────────────────────────────────────────────────────────────

func TestScoreStatus_Healthy(t *testing.T) {
	r := &HealthReport{Status: StatusHealthy}
	if got := scoreStatus(r); got != StatusHealthy {
		t.Errorf("empty report: want HEALTHY, got %q", got)
	}
}

func TestScoreStatus_CriticalWhenCircularDeps(t *testing.T) {
	r := &HealthReport{CircularDeps: 2}
	if got := scoreStatus(r); got != StatusCritical {
		t.Errorf("circular deps: want CRITICAL, got %q", got)
	}
}

func TestScoreStatus_DegradedOnCriticalImpact(t *testing.T) {
	r := &HealthReport{
		ImpactFiles: []ImpactFile{{Path: "src/db.go", RiskScore: "critical"}},
	}
	if got := scoreStatus(r); got != StatusDegraded {
		t.Errorf("critical impact file: want DEGRADED, got %q", got)
	}
}

func TestScoreStatus_DegradedOnHighCoupling(t *testing.T) {
	r := &HealthReport{
		Domains: []DomainHealth{
			{Name: "Core", IncomingDeps: []string{"A", "B", "C", "D", "E"}},
		},
	}
	if got := scoreStatus(r); got != StatusDegraded {
		t.Errorf("5 incoming deps: want DEGRADED, got %q", got)
	}
}

// ── detectCircularDeps ────────────────────────────────────────────────────────

func TestDetectCircularDeps_None(t *testing.T) {
	ir := &api.SupermodelIR{}
	count, cycles := detectCircularDeps(ir)
	if count != 0 || len(cycles) != 0 {
		t.Errorf("no circular deps: want 0, got count=%d cycles=%v", count, cycles)
	}
}

func TestDetectCircularDeps_Found(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{
			Relationships: []api.IRRelationship{
				{Type: "CIRCULAR_DEPENDENCY", Source: "A", Target: "B"},
				{Type: "CIRCULAR_DEP", Source: "C", Target: "D"},
				{Type: "other", Source: "X", Target: "Y"}, // ignored
			},
		},
	}
	count, cycles := detectCircularDeps(ir)
	if count != 2 {
		t.Errorf("want 2 circular deps, got %d", count)
	}
	if len(cycles) != 2 {
		t.Errorf("want 2 cycles, got %d", len(cycles))
	}
}

func TestDetectCircularDeps_Deduplication(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{
			Relationships: []api.IRRelationship{
				{Type: "CIRCULAR_DEPENDENCY", Source: "A", Target: "B"},
				{Type: "CIRCULAR_DEPENDENCY", Source: "A", Target: "B"}, // duplicate
			},
		},
	}
	count, _ := detectCircularDeps(ir)
	if count != 1 {
		t.Errorf("deduplication: want 1 cycle, got %d", count)
	}
}

// ── buildExternalDeps ─────────────────────────────────────────────────────────

func TestBuildExternalDeps_Empty(t *testing.T) {
	ir := &api.SupermodelIR{}
	deps := buildExternalDeps(ir)
	if len(deps) != 0 {
		t.Errorf("empty IR: want no deps, got %v", deps)
	}
}

func TestBuildExternalDeps_Sorted(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{
			Nodes: []api.IRNode{
				{Type: "ExternalDependency", Name: "zlib"},
				{Type: "ExternalDependency", Name: "axios"},
				{Type: "Function", Name: "foo"}, // ignored
				{Type: "ExternalDependency", Name: ""},  // empty name, ignored
			},
		},
	}
	deps := buildExternalDeps(ir)
	if len(deps) != 2 {
		t.Errorf("want 2 deps, got %v", deps)
	}
	if deps[0] != "axios" || deps[1] != "zlib" {
		t.Errorf("want sorted [axios, zlib], got %v", deps)
	}
}

func TestBuildExternalDeps_Deduplicated(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{
			Nodes: []api.IRNode{
				{Type: "ExternalDependency", Name: "axios"},
				{Type: "ExternalDependency", Name: "axios"},
			},
		},
	}
	deps := buildExternalDeps(ir)
	if len(deps) != 1 {
		t.Errorf("want 1 unique dep, got %v", deps)
	}
}

// ── buildCriticalFiles ────────────────────────────────────────────────────────

func TestBuildCriticalFiles_None(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{Name: "Auth", KeyFiles: []string{"a.go"}},
		},
	}
	files := buildCriticalFiles(ir)
	if len(files) != 0 {
		t.Errorf("no file shared across >1 domain: want 0, got %v", files)
	}
}

func TestBuildCriticalFiles_SharedFile(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{Name: "Auth", KeyFiles: []string{"shared.go", "auth.go"}},
			{Name: "API", KeyFiles: []string{"shared.go", "api.go"}},
		},
	}
	files := buildCriticalFiles(ir)
	if len(files) != 1 {
		t.Fatalf("want 1 critical file, got %v", files)
	}
	if files[0].Path != "shared.go" {
		t.Errorf("want shared.go, got %q", files[0].Path)
	}
	if files[0].RelationshipCount != 2 {
		t.Errorf("want relationship count 2, got %d", files[0].RelationshipCount)
	}
}

func TestBuildCriticalFiles_DedupWithinDomain(t *testing.T) {
	// Same file listed twice in one domain should only count once per domain.
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{Name: "Auth", KeyFiles: []string{"shared.go", "shared.go"}},
			{Name: "API", KeyFiles: []string{"shared.go"}},
		},
	}
	files := buildCriticalFiles(ir)
	if len(files) != 1 || files[0].RelationshipCount != 2 {
		t.Errorf("want 1 file with count 2, got %v", files)
	}
}

func TestBuildCriticalFiles_MoreThanTen(t *testing.T) {
	// Build an IR with 12 files each shared across 2 domains → must cap at 10.
	domains := make([]api.IRDomain, 0, 2)
	var keys1, keys2 []string
	for i := 0; i < 12; i++ {
		f := "file%02d.go"
		k := "file" + string(rune('0'+i/10)) + string(rune('0'+i%10)) + ".go"
		_ = f
		keys1 = append(keys1, k)
		keys2 = append(keys2, k)
	}
	domains = append(domains,
		api.IRDomain{Name: "D1", KeyFiles: keys1},
		api.IRDomain{Name: "D2", KeyFiles: keys2},
	)
	ir := &api.SupermodelIR{Domains: domains}
	files := buildCriticalFiles(ir)
	if len(files) > 10 {
		t.Errorf("should cap at 10, got %d", len(files))
	}
}

func TestBuildCriticalFiles_EqualCountsSortedByPath(t *testing.T) {
	// Two files both shared across 2 domains → sorted by path when counts equal.
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{Name: "D1", KeyFiles: []string{"b.go", "a.go"}},
			{Name: "D2", KeyFiles: []string{"b.go", "a.go"}},
		},
	}
	files := buildCriticalFiles(ir)
	if len(files) != 2 {
		t.Fatalf("want 2 critical files, got %d", len(files))
	}
	// Equal counts → sorted lexicographically.
	if files[0].Path != "a.go" {
		t.Errorf("equal counts: expected a.go first, got %q", files[0].Path)
	}
}

// ── buildDomainHealthList ─────────────────────────────────────────────────────

func TestBuildDomainHealthList_Empty(t *testing.T) {
	ir := &api.SupermodelIR{}
	domains := buildDomainHealthList(ir, map[string][]string{}, map[string][]string{})
	if len(domains) != 0 {
		t.Errorf("empty IR: want 0 domains, got %d", len(domains))
	}
}

func TestBuildDomainHealthList_WithDomains(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{
				Name:               "Auth",
				DescriptionSummary: "Authentication",
				KeyFiles:           []string{"auth.go", "login.go"},
				Responsibilities:   []string{"verify tokens"},
				Subdomains:         []api.IRSubdomain{{Name: "OAuth"}},
			},
		},
	}
	incoming := map[string][]string{"Auth": {"API", "Web"}}
	outgoing := map[string][]string{"Auth": {"DB"}}
	domains := buildDomainHealthList(ir, incoming, outgoing)
	if len(domains) != 1 {
		t.Fatalf("want 1 domain, got %d", len(domains))
	}
	d := domains[0]
	if d.Name != "Auth" {
		t.Errorf("want 'Auth', got %q", d.Name)
	}
	if d.Description != "Authentication" {
		t.Errorf("want 'Authentication', got %q", d.Description)
	}
	if d.KeyFileCount != 2 {
		t.Errorf("want 2 key files, got %d", d.KeyFileCount)
	}
	if d.Responsibilities != 1 {
		t.Errorf("want 1 responsibility, got %d", d.Responsibilities)
	}
	if d.Subdomains != 1 {
		t.Errorf("want 1 subdomain, got %d", d.Subdomains)
	}
	if len(d.IncomingDeps) != 2 {
		t.Errorf("want 2 incoming deps, got %v", d.IncomingDeps)
	}
	if len(d.OutgoingDeps) != 1 {
		t.Errorf("want 1 outgoing dep, got %v", d.OutgoingDeps)
	}
	// Incoming deps should be sorted.
	if d.IncomingDeps[0] != "API" {
		t.Errorf("sorted incoming: want 'API' first, got %q", d.IncomingDeps[0])
	}
}

func TestBuildDomainHealthList_NoCouplingData(t *testing.T) {
	// Domain with no entries in the coupling maps → empty dep slices.
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{Name: "Isolated", KeyFiles: []string{"iso.go"}},
		},
	}
	domains := buildDomainHealthList(ir, map[string][]string{}, map[string][]string{})
	if len(domains) != 1 {
		t.Fatalf("want 1 domain, got %d", len(domains))
	}
	if len(domains[0].IncomingDeps) != 0 {
		t.Errorf("want 0 incoming deps, got %v", domains[0].IncomingDeps)
	}
}

// ── buildCouplingMaps ─────────────────────────────────────────────────────────

func TestBuildCouplingMaps_Empty(t *testing.T) {
	ir := &api.SupermodelIR{}
	inc, out := buildCouplingMaps(ir)
	if len(inc) != 0 || len(out) != 0 {
		t.Error("empty IR: want empty coupling maps")
	}
}

func TestBuildCouplingMaps_DomainRelates(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{
			Relationships: []api.IRRelationship{
				{Type: "DOMAIN_RELATES", Source: "Auth", Target: "DB"},
				{Type: "other_type", Source: "X", Target: "Y"}, // ignored
			},
		},
	}
	inc, out := buildCouplingMaps(ir)
	if len(out["Auth"]) != 1 || out["Auth"][0] != "DB" {
		t.Errorf("outgoing: want Auth→[DB], got %v", out)
	}
	if len(inc["DB"]) != 1 || inc["DB"][0] != "Auth" {
		t.Errorf("incoming: want DB←[Auth], got %v", inc)
	}
}

func TestBuildCouplingMaps_Deduplication(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{
			Relationships: []api.IRRelationship{
				{Type: "DOMAIN_RELATES", Source: "Auth", Target: "DB"},
				{Type: "DOMAIN_RELATES", Source: "Auth", Target: "DB"}, // duplicate
			},
		},
	}
	_, out := buildCouplingMaps(ir)
	if len(out["Auth"]) != 1 {
		t.Errorf("deduplication: want 1 outgoing, got %v", out["Auth"])
	}
}

func TestBuildCouplingMaps_EmptySourceTarget(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{
			Relationships: []api.IRRelationship{
				{Type: "DOMAIN_RELATES", Source: "", Target: "DB"}, // empty source, ignored
				{Type: "DOMAIN_RELATES", Source: "Auth", Target: ""}, // empty target, ignored
			},
		},
	}
	inc, out := buildCouplingMaps(ir)
	if len(inc) != 0 || len(out) != 0 {
		t.Error("empty source/target: want empty coupling maps")
	}
}

// ── generateRecommendations ───────────────────────────────────────────────────

func TestGenerateRecommendations_Empty(t *testing.T) {
	r := &HealthReport{}
	recs := generateRecommendations(r)
	if len(recs) != 0 {
		t.Errorf("empty report: want no recs, got %v", recs)
	}
}

func TestGenerateRecommendations_CircularDeps(t *testing.T) {
	r := &HealthReport{CircularDeps: 2}
	recs := generateRecommendations(r)
	if len(recs) == 0 {
		t.Fatal("circular deps: want recommendation, got none")
	}
	if recs[0].Priority != 1 {
		t.Errorf("circular dep rec should be priority 1, got %d", recs[0].Priority)
	}
	if !strings.Contains(recs[0].Message, "circular") {
		t.Errorf("expected circular dep message, got %q", recs[0].Message)
	}
}

func TestGenerateRecommendations_HighCoupling(t *testing.T) {
	r := &HealthReport{
		Domains: []DomainHealth{
			{Name: "Core", IncomingDeps: []string{"A", "B", "C"}},
		},
	}
	recs := generateRecommendations(r)
	if len(recs) == 0 {
		t.Fatal("high coupling: want recommendation, got none")
	}
	found := false
	for _, rec := range recs {
		if strings.Contains(rec.Message, "Core") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected recommendation mentioning 'Core', got %v", recs)
	}
}

func TestGenerateRecommendations_NoKeyFiles(t *testing.T) {
	r := &HealthReport{
		Domains: []DomainHealth{
			{Name: "Orphan", KeyFileCount: 0},
		},
	}
	recs := generateRecommendations(r)
	found := false
	for _, rec := range recs {
		if strings.Contains(rec.Message, "Orphan") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected recommendation for domain with no key files, got %v", recs)
	}
}

func TestGenerateRecommendations_HighBlastRadius(t *testing.T) {
	r := &HealthReport{
		CriticalFiles: []CriticalFile{
			{Path: "core/db.go", RelationshipCount: 4},
		},
	}
	recs := generateRecommendations(r)
	found := false
	for _, rec := range recs {
		if strings.Contains(rec.Message, "core/db.go") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected recommendation for high blast radius file, got %v", recs)
	}
}

func TestGenerateRecommendations_CriticalImpactFile(t *testing.T) {
	r := &HealthReport{
		ImpactFiles: []ImpactFile{
			{Path: "api/auth.go", RiskScore: "critical", Direct: 10, Transitive: 30, Files: 5},
		},
	}
	recs := generateRecommendations(r)
	found := false
	for _, rec := range recs {
		if strings.Contains(rec.Message, "api/auth.go") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected recommendation for critical impact file, got %v", recs)
	}
}

// ── summaryInt ────────────────────────────────────────────────────────────────

func TestSummaryInt_Present(t *testing.T) {
	summary := map[string]any{"filesProcessed": float64(42)}
	if got := summaryInt(summary, "filesProcessed"); got != 42 {
		t.Errorf("want 42, got %d", got)
	}
}

func TestSummaryInt_Missing(t *testing.T) {
	if got := summaryInt(map[string]any{}, "missing"); got != 0 {
		t.Errorf("missing key: want 0, got %d", got)
	}
}

func TestSummaryInt_WrongType(t *testing.T) {
	summary := map[string]any{"count": "not a number"}
	if got := summaryInt(summary, "count"); got != 0 {
		t.Errorf("wrong type: want 0, got %d", got)
	}
}

// ── Analyze ───────────────────────────────────────────────────────────────────

func TestAnalyze_EmptyIR(t *testing.T) {
	ir := &api.SupermodelIR{}
	r := Analyze(ir, "testproject")
	if r.ProjectName != "testproject" {
		t.Errorf("want 'testproject', got %q", r.ProjectName)
	}
	if r.Status != StatusHealthy {
		t.Errorf("empty IR: want HEALTHY, got %q", r.Status)
	}
}

func TestAnalyze_SetsLanguage(t *testing.T) {
	ir := &api.SupermodelIR{
		Summary:  map[string]any{"primaryLanguage": "Go"},
		Metadata: api.IRMetadata{Languages: []string{"Go", "TypeScript"}},
	}
	r := Analyze(ir, "myproject")
	if r.Language != "Go" {
		t.Errorf("want 'Go', got %q", r.Language)
	}
}

func TestAnalyze_LanguageFallsBackToMetadata(t *testing.T) {
	// No primaryLanguage in summary; falls back to first metadata language.
	ir := &api.SupermodelIR{
		Metadata: api.IRMetadata{Languages: []string{"TypeScript"}},
	}
	r := Analyze(ir, "proj")
	if r.Language != "TypeScript" {
		t.Errorf("fallback: want 'TypeScript', got %q", r.Language)
	}
}

func TestAnalyze_CircularDepsCritical(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{
			Relationships: []api.IRRelationship{
				{Type: "CIRCULAR_DEPENDENCY", Source: "A", Target: "B"},
			},
		},
	}
	r := Analyze(ir, "proj")
	if r.Status != StatusCritical {
		t.Errorf("circular dep: want CRITICAL, got %q", r.Status)
	}
	if r.CircularDeps != 1 {
		t.Errorf("want 1 circular dep, got %d", r.CircularDeps)
	}
}

// ── EnrichWithImpact ──────────────────────────────────────────────────────────

func TestEnrichWithImpact_Nil(t *testing.T) {
	r := &HealthReport{Status: StatusHealthy}
	EnrichWithImpact(r, nil) // should not panic
	if r.Status != StatusHealthy {
		t.Error("nil impact: status should not change")
	}
}

func TestEnrichWithImpact_AddsImpactFiles(t *testing.T) {
	r := &HealthReport{Status: StatusHealthy}
	impact := &api.ImpactResult{
		Impacts: []api.ImpactTarget{
			{
				Target:      api.ImpactTargetInfo{File: "src/auth.go"},
				BlastRadius: api.BlastRadius{RiskScore: "high", DirectDependents: 5, TransitiveDependents: 20, AffectedFiles: 3},
			},
		},
	}
	EnrichWithImpact(r, impact)
	if len(r.ImpactFiles) == 0 {
		t.Fatal("expected impact files after enrichment")
	}
	if r.ImpactFiles[0].Path != "src/auth.go" {
		t.Errorf("want 'src/auth.go', got %q", r.ImpactFiles[0].Path)
	}
}

func TestEnrichWithImpact_GlobalMetrics(t *testing.T) {
	r := &HealthReport{}
	impact := &api.ImpactResult{
		GlobalMetrics: api.ImpactGlobalMetrics{
			MostCriticalFiles: []api.CriticalFileMetric{
				{File: "core/main.go", DependentCount: 10},
			},
		},
	}
	EnrichWithImpact(r, impact)
	if len(r.ImpactFiles) == 0 {
		t.Fatal("expected impact files from global metrics")
	}
}

func TestEnrichWithImpact_CapsAtTen(t *testing.T) {
	r := &HealthReport{}
	var impacts []api.ImpactTarget
	for i := 0; i < 15; i++ {
		impacts = append(impacts, api.ImpactTarget{
			Target:      api.ImpactTargetInfo{File: "file.go"},
			BlastRadius: api.BlastRadius{DirectDependents: i},
		})
	}
	EnrichWithImpact(r, &api.ImpactResult{Impacts: impacts})
	if len(r.ImpactFiles) > 10 {
		t.Errorf("should cap at 10, got %d", len(r.ImpactFiles))
	}
}

// ── RenderHealth ──────────────────────────────────────────────────────────────

func makeHealthReport() *HealthReport {
	return &HealthReport{
		ProjectName:    "myproject",
		AnalyzedAt:     time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
		Status:         StatusHealthy,
		TotalFiles:     100,
		TotalFunctions: 500,
	}
}

func TestRenderHealth_BasicFields(t *testing.T) {
	r := makeHealthReport()
	var w strings.Builder
	RenderHealth(&w, r)
	output := w.String()

	for _, want := range []string{"myproject", "HEALTHY", "100", "500"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in health report output", want)
		}
	}
}

func TestRenderHealth_CircularDeps(t *testing.T) {
	r := makeHealthReport()
	r.Status = StatusCritical
	r.CircularDeps = 2
	r.CircularCycles = [][]string{{"A", "B"}, {"C", "D"}}
	var w strings.Builder
	RenderHealth(&w, r)
	output := w.String()

	if !strings.Contains(output, "Circular Dependencies") {
		t.Error("expected Circular Dependencies section")
	}
	if !strings.Contains(output, "A → B") {
		t.Errorf("expected 'A → B' in output, got:\n%s", output)
	}
}

func TestRenderHealth_CriticalFiles(t *testing.T) {
	r := makeHealthReport()
	r.CriticalFiles = []CriticalFile{
		{Path: "core/db.go", RelationshipCount: 3},
	}
	var w strings.Builder
	RenderHealth(&w, r)
	output := w.String()

	if !strings.Contains(output, "Critical Files") {
		t.Error("expected Critical Files section")
	}
	if !strings.Contains(output, "core/db.go") {
		t.Error("expected critical file path in output")
	}
}

func TestRenderHealth_ImpactFiles(t *testing.T) {
	r := makeHealthReport()
	r.ImpactFiles = []ImpactFile{
		{Path: "api/auth.go", RiskScore: "high", Direct: 5, Transitive: 15, Files: 3},
		{Path: "core/util.go"}, // empty RiskScore → should render as "-"
	}
	var w strings.Builder
	RenderHealth(&w, r)
	output := w.String()

	if !strings.Contains(output, "Impact Analysis") {
		t.Error("expected Impact Analysis section")
	}
	if !strings.Contains(output, "api/auth.go") {
		t.Error("expected impact file path in output")
	}
	if !strings.Contains(output, "-") {
		t.Error("expected '-' for empty risk score")
	}
}

func TestRenderHealth_NoImpactSection(t *testing.T) {
	r := makeHealthReport()
	// No ImpactFiles → section should be absent.
	var w strings.Builder
	RenderHealth(&w, r)
	if strings.Contains(w.String(), "Impact Analysis") {
		t.Error("no impact files: should not render Impact Analysis section")
	}
}

func TestRenderHealth_Domains(t *testing.T) {
	r := makeHealthReport()
	r.Domains = []DomainHealth{
		{
			Name:         "Auth",
			Description:  "Handles authentication",
			KeyFileCount: 5,
			IncomingDeps: []string{"API", "Web"},
			OutgoingDeps: []string{"DB"},
		},
	}
	var w strings.Builder
	RenderHealth(&w, r)
	output := w.String()

	if !strings.Contains(output, "Domain Health") {
		t.Error("expected Domain Health section")
	}
	if !strings.Contains(output, "Auth") {
		t.Error("expected domain name in output")
	}
	if !strings.Contains(output, "Depended on by") {
		t.Error("expected 'Depended on by' for incoming deps")
	}
	if !strings.Contains(output, "Depends on") {
		t.Error("expected 'Depends on' for outgoing deps")
	}
}

func TestRenderHealth_DomainNoDescription(t *testing.T) {
	// Domain with no description should not produce an empty line.
	r := makeHealthReport()
	r.Domains = []DomainHealth{{Name: "Simple"}}
	var w strings.Builder
	RenderHealth(&w, r) // should not panic
}

func TestRenderHealth_ExternalDeps(t *testing.T) {
	r := makeHealthReport()
	r.ExternalDeps = []string{"axios", "zlib"}
	var w strings.Builder
	RenderHealth(&w, r)
	if !strings.Contains(w.String(), "Tech Stack") {
		t.Error("expected Tech Stack section with external deps")
	}
}

func TestRenderHealth_Languages(t *testing.T) {
	r := makeHealthReport()
	r.Languages = []string{"Go", "TypeScript"}
	var w strings.Builder
	RenderHealth(&w, r)
	if !strings.Contains(w.String(), "Go") {
		t.Error("expected languages in metrics table")
	}
}

func TestRenderHealth_RecommendationsPresent(t *testing.T) {
	r := makeHealthReport()
	r.Recommendations = []Recommendation{
		{Priority: 1, Message: "Fix circular deps now."},
		{Priority: 2, Message: "Reduce coupling."},
		{Priority: 4, Message: "Low priority suggestion."}, // unknown priority → falls through to Info
	}
	var w strings.Builder
	RenderHealth(&w, r)
	output := w.String()

	if !strings.Contains(output, "Fix circular deps now.") {
		t.Error("expected critical recommendation message")
	}
}

func TestRenderHealth_NoRecommendations(t *testing.T) {
	r := makeHealthReport()
	r.Recommendations = nil
	var w strings.Builder
	RenderHealth(&w, r)
	if !strings.Contains(w.String(), "No issues found") {
		t.Error("no recommendations: expected 'No issues found' message")
	}
}

func TestRenderHealth_HighCouplingDomain(t *testing.T) {
	// Domain with ≥3 incoming deps triggers high-coupling counter in metrics table.
	r := makeHealthReport()
	r.Domains = []DomainHealth{
		{Name: "HeavyCore", IncomingDeps: []string{"A", "B", "C"}},
	}
	var w strings.Builder
	RenderHealth(&w, r)
	if !strings.Contains(w.String(), "WARN") {
		t.Error("high coupling domain: expected WARN in coupling status")
	}
}

// ── RenderRunPrompt ───────────────────────────────────────────────────────────

func makeSDLCData() *SDLCPromptData {
	return &SDLCPromptData{
		ProjectName:    "myproject",
		Language:       "Go",
		TotalFiles:     100,
		TotalFunctions: 500,
		GeneratedAt:    "2025-01-15",
	}
}

func TestRenderRunPrompt_BasicFields(t *testing.T) {
	d := makeSDLCData()
	d.Goal = "Add rate limiting to the API"
	var w strings.Builder
	RenderRunPrompt(&w, d)
	output := w.String()

	for _, want := range []string{"myproject", "Go", "Add rate limiting", "Phase 1", "Phase 8"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in run prompt output", want)
		}
	}
}

func TestRenderRunPrompt_WithCircularDeps(t *testing.T) {
	d := makeSDLCData()
	d.Goal = "Refactor auth"
	d.CircularDeps = 3
	var w strings.Builder
	RenderRunPrompt(&w, d)
	output := w.String()

	if !strings.Contains(output, "circular") || !strings.Contains(output, "3") {
		t.Errorf("expected circular dep warning with count, got:\n%s", output)
	}
}

func TestRenderRunPrompt_WithDomains(t *testing.T) {
	d := makeSDLCData()
	d.Goal = "Add feature"
	d.Domains = []DomainHealth{
		{Name: "Auth", Description: "Authentication layer", KeyFileCount: 3},
	}
	var w strings.Builder
	RenderRunPrompt(&w, d)
	if !strings.Contains(w.String(), "Auth") {
		t.Error("expected domain name in run prompt")
	}
}

func TestRenderRunPrompt_WithDomainNoDescription(t *testing.T) {
	d := makeSDLCData()
	d.Goal = "Add feature"
	d.Domains = []DomainHealth{
		{Name: "Auth", KeyFileCount: 3}, // no description
	}
	var w strings.Builder
	RenderRunPrompt(&w, d) // should not panic; KeyFileCount printed
}

func TestRenderRunPrompt_WithExternalDeps(t *testing.T) {
	d := makeSDLCData()
	d.Goal = "Fix bug"
	d.ExternalDeps = []string{"axios", "pg"}
	var w strings.Builder
	RenderRunPrompt(&w, d)
	if !strings.Contains(w.String(), "axios") {
		t.Error("expected external deps in output")
	}
}

func TestRenderRunPrompt_WithCriticalFiles(t *testing.T) {
	d := makeSDLCData()
	d.Goal = "Fix bug"
	d.CriticalFiles = []CriticalFile{
		{Path: "core/db.go", RelationshipCount: 4},
	}
	var w strings.Builder
	RenderRunPrompt(&w, d)
	if !strings.Contains(w.String(), "core/db.go") {
		t.Error("expected critical file in output")
	}
}

// ── RenderImprovePrompt ───────────────────────────────────────────────────────

func TestRenderImprovePrompt_NoHealthReport(t *testing.T) {
	d := makeSDLCData()
	var w strings.Builder
	RenderImprovePrompt(&w, d)
	output := w.String()

	for _, want := range []string{"myproject", "Improvement", "Step 1", "Step 4"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in improve prompt output", want)
		}
	}
	// Should not have a Health section when HealthReport is nil.
	if strings.Contains(output, "Current Health") {
		t.Error("no health report: should not render Current Health section")
	}
}

func TestRenderImprovePrompt_WithHealthReport(t *testing.T) {
	d := makeSDLCData()
	d.HealthReport = &HealthReport{
		Status:          StatusDegraded,
		Recommendations: []Recommendation{{Priority: 2, Message: "Reduce coupling."}},
	}
	var w strings.Builder
	RenderImprovePrompt(&w, d)
	output := w.String()

	if !strings.Contains(output, "Current Health") {
		t.Error("expected Current Health section")
	}
	if !strings.Contains(output, "Reduce coupling.") {
		t.Error("expected recommendation in output")
	}
}

func TestRenderImprovePrompt_WithHealthReportNoRecs(t *testing.T) {
	d := makeSDLCData()
	d.HealthReport = &HealthReport{Status: StatusHealthy}
	var w strings.Builder
	RenderImprovePrompt(&w, d) // should not panic; no recommendations section
	if !strings.Contains(w.String(), "Current Health") {
		t.Error("expected Current Health section even with no recommendations")
	}
}
