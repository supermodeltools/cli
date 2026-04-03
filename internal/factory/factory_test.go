package factory

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

// ── summaryInt ────────────────────────────────────────────────────────────────

func TestSummaryInt_MissingKey(t *testing.T) {
	if got := summaryInt(map[string]any{}, "files"); got != 0 {
		t.Errorf("missing key: want 0, got %d", got)
	}
}

func TestSummaryInt_Float64(t *testing.T) {
	m := map[string]any{"files": float64(42)}
	if got := summaryInt(m, "files"); got != 42 {
		t.Errorf("float64: want 42, got %d", got)
	}
}

func TestSummaryInt_WrongType(t *testing.T) {
	m := map[string]any{"files": "42"} // string, not float64
	if got := summaryInt(m, "files"); got != 0 {
		t.Errorf("wrong type: want 0, got %d", got)
	}
}

func TestSummaryInt_NilMap(t *testing.T) {
	if got := summaryInt(nil, "files"); got != 0 {
		t.Errorf("nil map: want 0, got %d", got)
	}
}

// ── pluralf ───────────────────────────────────────────────────────────────────

func TestPluralf_One(t *testing.T) {
	got := pluralf("Resolve %d cycle%s.", 1)
	if got != "Resolve 1 cycle." {
		t.Errorf("n=1: got %q", got)
	}
}

func TestPluralf_Many(t *testing.T) {
	got := pluralf("Resolve %d cycle%s.", 3)
	if got != "Resolve 3 cycles." {
		t.Errorf("n=3: got %q", got)
	}
}

// ── buildExternalDeps ─────────────────────────────────────────────────────────

func TestBuildExternalDeps_Empty(t *testing.T) {
	ir := &api.SupermodelIR{}
	if deps := buildExternalDeps(ir); len(deps) != 0 {
		t.Errorf("empty IR: want [], got %v", deps)
	}
}

func TestBuildExternalDeps_Sorted(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Nodes: []api.IRNode{
			{Type: "ExternalDependency", Name: "zlib"},
			{Type: "ExternalDependency", Name: "axios"},
			{Type: "ExternalDependency", Name: "cobra"},
		}},
	}
	deps := buildExternalDeps(ir)
	if len(deps) != 3 {
		t.Fatalf("want 3, got %d: %v", len(deps), deps)
	}
	if deps[0] != "axios" || deps[1] != "cobra" || deps[2] != "zlib" {
		t.Errorf("not sorted: %v", deps)
	}
}

func TestBuildExternalDeps_Dedup(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Nodes: []api.IRNode{
			{Type: "ExternalDependency", Name: "cobra"},
			{Type: "ExternalDependency", Name: "cobra"},
		}},
	}
	deps := buildExternalDeps(ir)
	if len(deps) != 1 {
		t.Errorf("dedup: want 1, got %d: %v", len(deps), deps)
	}
}

func TestBuildExternalDeps_IgnoresNonExternal(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Nodes: []api.IRNode{
			{Type: "Domain", Name: "auth"},
			{Type: "Function", Name: "handleLogin"},
			{Type: "ExternalDependency", Name: "cobra"},
		}},
	}
	deps := buildExternalDeps(ir)
	if len(deps) != 1 || deps[0] != "cobra" {
		t.Errorf("want [cobra], got %v", deps)
	}
}

func TestBuildExternalDeps_IgnoresEmptyName(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Nodes: []api.IRNode{
			{Type: "ExternalDependency", Name: ""},
			{Type: "ExternalDependency", Name: "cobra"},
		}},
	}
	deps := buildExternalDeps(ir)
	if len(deps) != 1 || deps[0] != "cobra" {
		t.Errorf("want [cobra], got %v", deps)
	}
}

// ── buildCouplingMaps ─────────────────────────────────────────────────────────

func TestBuildCouplingMaps_Empty(t *testing.T) {
	ir := &api.SupermodelIR{}
	incoming, outgoing := buildCouplingMaps(ir)
	if len(incoming) != 0 || len(outgoing) != 0 {
		t.Error("empty IR: expected empty maps")
	}
}

func TestBuildCouplingMaps_IgnoresNonDomainRelates(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Relationships: []api.IRRelationship{
			{Type: "IMPORTS", Source: "a", Target: "b"},
			{Type: "CIRCULAR_DEPENDENCY", Source: "c", Target: "d"},
		}},
	}
	incoming, outgoing := buildCouplingMaps(ir)
	if len(incoming) != 0 || len(outgoing) != 0 {
		t.Error("non-DOMAIN_RELATES edges should be ignored")
	}
}

func TestBuildCouplingMaps_SingleEdge(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Relationships: []api.IRRelationship{
			{Type: "DOMAIN_RELATES", Source: "auth", Target: "api"},
		}},
	}
	incoming, outgoing := buildCouplingMaps(ir)
	if len(outgoing["auth"]) != 1 || outgoing["auth"][0] != "api" {
		t.Errorf("outgoing auth: want [api], got %v", outgoing["auth"])
	}
	if len(incoming["api"]) != 1 || incoming["api"][0] != "auth" {
		t.Errorf("incoming api: want [auth], got %v", incoming["api"])
	}
}

func TestBuildCouplingMaps_DedupDuplicateEdges(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Relationships: []api.IRRelationship{
			{Type: "DOMAIN_RELATES", Source: "auth", Target: "api"},
			{Type: "DOMAIN_RELATES", Source: "auth", Target: "api"},
			{Type: "DOMAIN_RELATES", Source: "auth", Target: "api"},
		}},
	}
	incoming, outgoing := buildCouplingMaps(ir)
	if len(outgoing["auth"]) != 1 {
		t.Errorf("dedup failed: outgoing auth %v", outgoing["auth"])
	}
	if len(incoming["api"]) != 1 {
		t.Errorf("dedup failed: incoming api %v", incoming["api"])
	}
}

func TestBuildCouplingMaps_MultipleEdges(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Relationships: []api.IRRelationship{
			{Type: "DOMAIN_RELATES", Source: "auth", Target: "api"},
			{Type: "DOMAIN_RELATES", Source: "billing", Target: "api"},
			{Type: "DOMAIN_RELATES", Source: "auth", Target: "storage"},
		}},
	}
	incoming, outgoing := buildCouplingMaps(ir)
	if len(incoming["api"]) != 2 {
		t.Errorf("api should have 2 incoming, got %v", incoming["api"])
	}
	if len(outgoing["auth"]) != 2 {
		t.Errorf("auth should have 2 outgoing, got %v", outgoing["auth"])
	}
}

func TestBuildCouplingMaps_IgnoresEmptySourceTarget(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Relationships: []api.IRRelationship{
			{Type: "DOMAIN_RELATES", Source: "", Target: "api"},
			{Type: "DOMAIN_RELATES", Source: "auth", Target: ""},
		}},
	}
	incoming, outgoing := buildCouplingMaps(ir)
	if len(incoming) != 0 || len(outgoing) != 0 {
		t.Error("empty source/target should be ignored")
	}
}

// ── buildCriticalFiles ────────────────────────────────────────────────────────

func TestBuildCriticalFiles_Empty(t *testing.T) {
	ir := &api.SupermodelIR{}
	if files := buildCriticalFiles(ir); len(files) != 0 {
		t.Errorf("empty IR: want [], got %v", files)
	}
}

func TestBuildCriticalFiles_SingleDomainNoCritical(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{Name: "auth", KeyFiles: []string{"auth/handler.go", "auth/service.go"}},
		},
	}
	if files := buildCriticalFiles(ir); len(files) != 0 {
		t.Errorf("single domain should produce no critical files, got %v", files)
	}
}

func TestBuildCriticalFiles_CrossDomain(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{Name: "auth", KeyFiles: []string{"internal/api/client.go", "auth/handler.go"}},
			{Name: "billing", KeyFiles: []string{"internal/api/client.go", "billing/service.go"}},
		},
	}
	files := buildCriticalFiles(ir)
	if len(files) != 1 {
		t.Fatalf("want 1 critical file, got %d: %v", len(files), files)
	}
	if files[0].Path != "internal/api/client.go" {
		t.Errorf("want internal/api/client.go, got %q", files[0].Path)
	}
	if files[0].RelationshipCount != 2 {
		t.Errorf("want count=2, got %d", files[0].RelationshipCount)
	}
}

func TestBuildCriticalFiles_SortedByCountDescThenPathAsc(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{Name: "d1", KeyFiles: []string{"shared.go", "common.go"}},
			{Name: "d2", KeyFiles: []string{"shared.go", "common.go"}},
			{Name: "d3", KeyFiles: []string{"shared.go"}},
		},
	}
	files := buildCriticalFiles(ir)
	if len(files) != 2 {
		t.Fatalf("want 2, got %d: %v", len(files), files)
	}
	// shared.go in 3 domains, common.go in 2
	if files[0].Path != "shared.go" || files[0].RelationshipCount != 3 {
		t.Errorf("first should be shared.go×3, got %+v", files[0])
	}
	if files[1].Path != "common.go" || files[1].RelationshipCount != 2 {
		t.Errorf("second should be common.go×2, got %+v", files[1])
	}
}

func TestBuildCriticalFiles_SortedAlphaOnTie(t *testing.T) {
	// Both files referenced by exactly 2 domains — sort by path asc
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{Name: "d1", KeyFiles: []string{"z_file.go", "a_file.go"}},
			{Name: "d2", KeyFiles: []string{"z_file.go", "a_file.go"}},
		},
	}
	files := buildCriticalFiles(ir)
	if len(files) != 2 {
		t.Fatalf("want 2, got %d", len(files))
	}
	if files[0].Path != "a_file.go" {
		t.Errorf("alpha tie: first should be a_file.go, got %q", files[0].Path)
	}
}

func TestBuildCriticalFiles_CapAt10(t *testing.T) {
	// 12 files each referenced by exactly 2 domains
	doms := make([]api.IRDomain, 24)
	for i := 0; i < 12; i++ {
		f := fmt.Sprintf("shared%02d.go", i)
		doms[i*2] = api.IRDomain{Name: fmt.Sprintf("a%d", i), KeyFiles: []string{f}}
		doms[i*2+1] = api.IRDomain{Name: fmt.Sprintf("b%d", i), KeyFiles: []string{f}}
	}
	files := buildCriticalFiles(&api.SupermodelIR{Domains: doms})
	if len(files) != 10 {
		t.Errorf("cap at 10: want 10, got %d", len(files))
	}
}

func TestBuildCriticalFiles_DedupWithinDomain(t *testing.T) {
	// Same file listed twice in one domain — should count as 1
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{Name: "auth", KeyFiles: []string{"shared.go", "shared.go"}},
			{Name: "billing", KeyFiles: []string{"shared.go"}},
		},
	}
	files := buildCriticalFiles(ir)
	if len(files) != 1 {
		t.Fatalf("want 1 critical file, got %d: %v", len(files), files)
	}
	if files[0].RelationshipCount != 2 {
		t.Errorf("dedup within domain: want count=2, got %d", files[0].RelationshipCount)
	}
}

// ── buildDomainHealthList ─────────────────────────────────────────────────────

func TestBuildDomainHealthList_Empty(t *testing.T) {
	ir := &api.SupermodelIR{}
	domains := buildDomainHealthList(ir, map[string][]string{}, map[string][]string{})
	if len(domains) != 0 {
		t.Errorf("want [], got %v", domains)
	}
}

func TestBuildDomainHealthList_Basic(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{
				Name:               "Authentication",
				DescriptionSummary: "Handles auth flows",
				KeyFiles:           []string{"auth/handler.go", "auth/service.go"},
				Responsibilities:   []string{"login", "logout", "refresh"},
				Subdomains: []api.IRSubdomain{
					{Name: "OAuth"},
				},
			},
		},
	}
	incoming := map[string][]string{"Authentication": {"billing", "api"}}
	outgoing := map[string][]string{"Authentication": {"storage"}}

	domains := buildDomainHealthList(ir, incoming, outgoing)
	if len(domains) != 1 {
		t.Fatalf("want 1 domain, got %d", len(domains))
	}
	d := domains[0]
	if d.Name != "Authentication" {
		t.Errorf("name: got %q", d.Name)
	}
	if d.Description != "Handles auth flows" {
		t.Errorf("description: got %q", d.Description)
	}
	if d.KeyFileCount != 2 {
		t.Errorf("key file count: want 2, got %d", d.KeyFileCount)
	}
	if d.Responsibilities != 3 {
		t.Errorf("responsibilities: want 3, got %d", d.Responsibilities)
	}
	if d.Subdomains != 1 {
		t.Errorf("subdomains: want 1, got %d", d.Subdomains)
	}
}

func TestBuildDomainHealthList_IncomingOutgoingSorted(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{{Name: "api"}},
	}
	incoming := map[string][]string{"api": {"zebra", "alpha", "mango"}}
	outgoing := map[string][]string{"api": {"zz", "aa"}}

	domains := buildDomainHealthList(ir, incoming, outgoing)
	d := domains[0]
	if d.IncomingDeps[0] != "alpha" || d.IncomingDeps[1] != "mango" || d.IncomingDeps[2] != "zebra" {
		t.Errorf("incoming not sorted: %v", d.IncomingDeps)
	}
	if d.OutgoingDeps[0] != "aa" || d.OutgoingDeps[1] != "zz" {
		t.Errorf("outgoing not sorted: %v", d.OutgoingDeps)
	}
}

func TestBuildDomainHealthList_NoDepsForDomain(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{{Name: "isolated"}},
	}
	domains := buildDomainHealthList(ir, map[string][]string{}, map[string][]string{})
	if len(domains[0].IncomingDeps) != 0 || len(domains[0].OutgoingDeps) != 0 {
		t.Errorf("isolated domain should have no deps, got in=%v out=%v",
			domains[0].IncomingDeps, domains[0].OutgoingDeps)
	}
}

// ── detectCircularDeps ────────────────────────────────────────────────────────

func TestDetectCircularDeps_Empty(t *testing.T) {
	count, cycles := detectCircularDeps(&api.SupermodelIR{})
	if count != 0 || len(cycles) != 0 {
		t.Errorf("empty: want count=0, got count=%d cycles=%v", count, cycles)
	}
}

func TestDetectCircularDeps_TypeCIRCULAR_DEPENDENCY(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Relationships: []api.IRRelationship{
			{Type: "CIRCULAR_DEPENDENCY", Source: "auth", Target: "billing"},
		}},
	}
	count, cycles := detectCircularDeps(ir)
	if count != 1 {
		t.Errorf("want 1, got %d", count)
	}
	if cycles[0][0] != "auth" || cycles[0][1] != "billing" {
		t.Errorf("unexpected cycle: %v", cycles[0])
	}
}

func TestDetectCircularDeps_TypeCIRCULAR_DEP(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Relationships: []api.IRRelationship{
			{Type: "CIRCULAR_DEP", Source: "a", Target: "b"},
		}},
	}
	count, _ := detectCircularDeps(ir)
	if count != 1 {
		t.Errorf("CIRCULAR_DEP should be detected, got count=%d", count)
	}
}

func TestDetectCircularDeps_BothTypes(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Relationships: []api.IRRelationship{
			{Type: "CIRCULAR_DEPENDENCY", Source: "a", Target: "b"},
			{Type: "CIRCULAR_DEP", Source: "c", Target: "d"},
			{Type: "IMPORTS", Source: "e", Target: "f"},
		}},
	}
	count, _ := detectCircularDeps(ir)
	if count != 2 {
		t.Errorf("want 2, got %d", count)
	}
}

func TestDetectCircularDeps_Dedup(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Relationships: []api.IRRelationship{
			{Type: "CIRCULAR_DEPENDENCY", Source: "auth", Target: "billing"},
			{Type: "CIRCULAR_DEPENDENCY", Source: "auth", Target: "billing"},
		}},
	}
	count, _ := detectCircularDeps(ir)
	if count != 1 {
		t.Errorf("dedup: want 1, got %d", count)
	}
}

func TestDetectCircularDeps_IgnoresNonCircular(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Relationships: []api.IRRelationship{
			{Type: "DOMAIN_RELATES", Source: "auth", Target: "api"},
			{Type: "IMPORTS", Source: "x", Target: "y"},
		}},
	}
	count, _ := detectCircularDeps(ir)
	if count != 0 {
		t.Errorf("non-circular types should be ignored, got %d", count)
	}
}

// ── scoreStatus ───────────────────────────────────────────────────────────────

func TestScoreStatus_Healthy(t *testing.T) {
	r := &HealthReport{
		CircularDeps: 0,
		Domains:      []DomainHealth{{IncomingDeps: []string{"a", "b"}}},
	}
	if got := scoreStatus(r); got != StatusHealthy {
		t.Errorf("want HEALTHY, got %s", got)
	}
}

func TestScoreStatus_CriticalOnCircularDeps(t *testing.T) {
	r := &HealthReport{CircularDeps: 1}
	if got := scoreStatus(r); got != StatusCritical {
		t.Errorf("want CRITICAL, got %s", got)
	}
}

func TestScoreStatus_DegradedOnHighIncoming(t *testing.T) {
	r := &HealthReport{
		CircularDeps: 0,
		Domains:      []DomainHealth{{IncomingDeps: []string{"a", "b", "c", "d", "e"}}},
	}
	if got := scoreStatus(r); got != StatusDegraded {
		t.Errorf("want DEGRADED for 5 incoming, got %s", got)
	}
}

func TestScoreStatus_FourIncomingIsHealthy(t *testing.T) {
	r := &HealthReport{
		Domains: []DomainHealth{{IncomingDeps: []string{"a", "b", "c", "d"}}},
	}
	if got := scoreStatus(r); got != StatusHealthy {
		t.Errorf("4 incoming should still be HEALTHY, got %s", got)
	}
}

func TestScoreStatus_CriticalBeatsDegraded(t *testing.T) {
	r := &HealthReport{
		CircularDeps: 1,
		Domains:      []DomainHealth{{IncomingDeps: []string{"a", "b", "c", "d", "e"}}},
	}
	if got := scoreStatus(r); got != StatusCritical {
		t.Errorf("CRITICAL should beat DEGRADED, got %s", got)
	}
}

// ── generateRecommendations ───────────────────────────────────────────────────

func TestGenerateRecommendations_Empty(t *testing.T) {
	if recs := generateRecommendations(&HealthReport{}); len(recs) != 0 {
		t.Errorf("clean report: want no recs, got %v", recs)
	}
}

func TestGenerateRecommendations_CircularDeps(t *testing.T) {
	r := &HealthReport{CircularDeps: 3}
	recs := generateRecommendations(r)
	if len(recs) != 1 {
		t.Fatalf("want 1 rec, got %d", len(recs))
	}
	if recs[0].Priority != 1 {
		t.Errorf("circular dep rec should be priority 1, got %d", recs[0].Priority)
	}
	if !strings.Contains(recs[0].Message, "3") {
		t.Errorf("message should mention count 3: %q", recs[0].Message)
	}
}

func TestGenerateRecommendations_CircularDepsSingular(t *testing.T) {
	r := &HealthReport{CircularDeps: 1}
	recs := generateRecommendations(r)
	// Pluralisation: "cycle" not "cycles"
	if !strings.Contains(recs[0].Message, "cycle") {
		t.Errorf("singular message should say 'cycle': %q", recs[0].Message)
	}
	if strings.Contains(recs[0].Message, "cycles") {
		t.Errorf("singular message should not say 'cycles': %q", recs[0].Message)
	}
}

func TestGenerateRecommendations_HighCouplingThreshold(t *testing.T) {
	// 2 incoming → below threshold, no rec
	r := &HealthReport{
		Domains: []DomainHealth{{Name: "api", KeyFileCount: 1, IncomingDeps: []string{"a", "b"}}},
	}
	if recs := generateRecommendations(r); len(recs) != 0 {
		t.Errorf("2 incoming should not trigger rec, got %v", recs)
	}
}

func TestGenerateRecommendations_HighCoupling(t *testing.T) {
	r := &HealthReport{
		Domains: []DomainHealth{
			{Name: "api", KeyFileCount: 1, IncomingDeps: []string{"a", "b", "c"}}, // 3 >= threshold
		},
	}
	recs := generateRecommendations(r)
	if len(recs) != 1 || recs[0].Priority != 2 {
		t.Fatalf("want 1 priority-2 rec, got %v", recs)
	}
	if !strings.Contains(recs[0].Message, "api") {
		t.Errorf("should mention domain name: %q", recs[0].Message)
	}
}

func TestGenerateRecommendations_NoKeyFiles(t *testing.T) {
	r := &HealthReport{
		Domains: []DomainHealth{{Name: "orphan", KeyFileCount: 0}},
	}
	recs := generateRecommendations(r)
	if len(recs) != 1 || recs[0].Priority != 3 {
		t.Fatalf("want 1 priority-3 rec, got %v", recs)
	}
	if !strings.Contains(recs[0].Message, "orphan") {
		t.Errorf("should mention domain name: %q", recs[0].Message)
	}
}

func TestGenerateRecommendations_BlastRadius(t *testing.T) {
	r := &HealthReport{
		CriticalFiles: []CriticalFile{
			{Path: "shared.go", RelationshipCount: 4},
		},
	}
	recs := generateRecommendations(r)
	if len(recs) != 1 || recs[0].Priority != 2 {
		t.Fatalf("want 1 priority-2 rec for blast radius, got %v", recs)
	}
	if !strings.Contains(recs[0].Message, "shared.go") {
		t.Errorf("should mention file: %q", recs[0].Message)
	}
}

func TestGenerateRecommendations_BlastRadiusThreshold(t *testing.T) {
	// 3 relationships → below threshold
	r := &HealthReport{
		CriticalFiles: []CriticalFile{
			{Path: "shared.go", RelationshipCount: 3},
		},
	}
	if recs := generateRecommendations(r); len(recs) != 0 {
		t.Errorf("3 relationships should not trigger rec, got %v", recs)
	}
}

func TestGenerateRecommendations_SortedByPriority(t *testing.T) {
	r := &HealthReport{
		CircularDeps: 1, // priority 1
		Domains: []DomainHealth{
			{Name: "api", IncomingDeps: []string{"a", "b", "c"}}, // priority 2
			{Name: "orphan", KeyFileCount: 0},                    // priority 3
		},
	}
	recs := generateRecommendations(r)
	if len(recs) < 3 {
		t.Fatalf("want >=3 recs, got %d", len(recs))
	}
	for i := 1; i < len(recs); i++ {
		if recs[i].Priority < recs[i-1].Priority {
			t.Errorf("recs not sorted by priority at index %d: %v", i, recs)
			break
		}
	}
	if recs[0].Priority != 1 {
		t.Errorf("first rec should be priority 1, got %d", recs[0].Priority)
	}
}

// ── CouplingStatus ────────────────────────────────────────────────────────────

func TestCouplingStatus(t *testing.T) {
	tests := []struct {
		incoming []string
		want     string
	}{
		{nil, "✅ OK"},
		{[]string{"a"}, "✅ OK"},
		{[]string{"a", "b"}, "✅ OK"},
		{[]string{"a", "b", "c"}, "⚠️  WARN"},
		{[]string{"a", "b", "c", "d"}, "⚠️  WARN"},
		{[]string{"a", "b", "c", "d", "e"}, "⛔ HIGH"},
		{[]string{"a", "b", "c", "d", "e", "f"}, "⛔ HIGH"},
	}
	for _, tt := range tests {
		d := &DomainHealth{IncomingDeps: tt.incoming}
		if got := d.CouplingStatus(); got != tt.want {
			t.Errorf("CouplingStatus(%d incoming) = %q, want %q",
				len(tt.incoming), got, tt.want)
		}
	}
}

// ── Analyze ───────────────────────────────────────────────────────────────────

func TestAnalyze_Basic(t *testing.T) {
	ir := &api.SupermodelIR{
		Summary: map[string]any{
			"filesProcessed": float64(50),
			"functions":      float64(200),
		},
		Metadata: api.IRMetadata{Languages: []string{"Go", "JavaScript"}},
		Domains: []api.IRDomain{
			{
				Name:               "Authentication",
				DescriptionSummary: "Handles auth",
				KeyFiles:           []string{"auth/handler.go"},
				Responsibilities:   []string{"Login", "Logout"},
			},
		},
		Graph: api.IRGraph{
			Nodes: []api.IRNode{{Type: "ExternalDependency", Name: "cobra"}},
		},
	}

	r := Analyze(ir, "myproject")

	if r.ProjectName != "myproject" {
		t.Errorf("project name: got %q", r.ProjectName)
	}
	if r.TotalFiles != 50 {
		t.Errorf("total files: got %d", r.TotalFiles)
	}
	if r.TotalFunctions != 200 {
		t.Errorf("total functions: got %d", r.TotalFunctions)
	}
	if r.Language != "Go" {
		t.Errorf("language: got %q", r.Language)
	}
	if len(r.ExternalDeps) != 1 || r.ExternalDeps[0] != "cobra" {
		t.Errorf("external deps: got %v", r.ExternalDeps)
	}
	if len(r.Domains) != 1 || r.Domains[0].Name != "Authentication" {
		t.Errorf("domains: got %v", r.Domains)
	}
	if r.Status != StatusHealthy {
		t.Errorf("status: got %s", r.Status)
	}
}

func TestAnalyze_PrimaryLanguageFromSummaryOverridesMetadata(t *testing.T) {
	ir := &api.SupermodelIR{
		Summary:  map[string]any{"primaryLanguage": "TypeScript"},
		Metadata: api.IRMetadata{Languages: []string{"Go"}},
	}
	r := Analyze(ir, "proj")
	if r.Language != "TypeScript" {
		t.Errorf("Summary primaryLanguage should override Metadata: got %q", r.Language)
	}
}

func TestAnalyze_PrimaryLanguageFromMetadataWhenNoSummary(t *testing.T) {
	ir := &api.SupermodelIR{
		Metadata: api.IRMetadata{Languages: []string{"Rust", "C"}},
	}
	r := Analyze(ir, "proj")
	if r.Language != "Rust" {
		t.Errorf("first Metadata language: got %q", r.Language)
	}
}

func TestAnalyze_CircularDepsCauseCritical(t *testing.T) {
	ir := &api.SupermodelIR{
		Graph: api.IRGraph{Relationships: []api.IRRelationship{
			{Type: "CIRCULAR_DEPENDENCY", Source: "auth", Target: "billing"},
		}},
	}
	r := Analyze(ir, "proj")
	if r.Status != StatusCritical {
		t.Errorf("want CRITICAL, got %s", r.Status)
	}
	if r.CircularDeps != 1 {
		t.Errorf("want 1 circular dep, got %d", r.CircularDeps)
	}
	if len(r.Recommendations) == 0 {
		t.Error("should have recommendations for circular deps")
	}
}

func TestAnalyze_EmptyIR(t *testing.T) {
	r := Analyze(&api.SupermodelIR{}, "empty")
	if r == nil {
		t.Fatal("Analyze returned nil")
	}
	if r.Status != StatusHealthy {
		t.Errorf("empty IR should be HEALTHY, got %s", r.Status)
	}
	if r.ProjectName != "empty" {
		t.Errorf("project name: got %q", r.ProjectName)
	}
}

func TestAnalyze_DegradedHighCoupling(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{{Name: "api"}},
		Graph: api.IRGraph{Relationships: []api.IRRelationship{
			{Type: "DOMAIN_RELATES", Source: "a", Target: "api"},
			{Type: "DOMAIN_RELATES", Source: "b", Target: "api"},
			{Type: "DOMAIN_RELATES", Source: "c", Target: "api"},
			{Type: "DOMAIN_RELATES", Source: "d", Target: "api"},
			{Type: "DOMAIN_RELATES", Source: "e", Target: "api"},
		}},
	}
	r := Analyze(ir, "proj")
	if r.Status != StatusDegraded {
		t.Errorf("5 incoming → want DEGRADED, got %s", r.Status)
	}
}

// ── RenderHealth ──────────────────────────────────────────────────────────────

func TestRenderHealth_ContainsProjectName(t *testing.T) {
	r := &HealthReport{ProjectName: "myawesomeapp", Status: StatusHealthy}
	var buf bytes.Buffer
	RenderHealth(&buf, r)
	if !strings.Contains(buf.String(), "myawesomeapp") {
		t.Errorf("output should contain project name, got:\n%s", buf.String())
	}
}

func TestRenderHealth_HealthyStatus(t *testing.T) {
	r := &HealthReport{ProjectName: "proj", Status: StatusHealthy}
	var buf bytes.Buffer
	RenderHealth(&buf, r)
	if !strings.Contains(buf.String(), "HEALTHY") {
		t.Errorf("should contain HEALTHY status, got:\n%s", buf.String())
	}
}

func TestRenderHealth_CriticalStatus(t *testing.T) {
	r := &HealthReport{
		ProjectName:  "proj",
		Status:       StatusCritical,
		CircularDeps: 2,
		CircularCycles: [][]string{
			{"auth", "billing"},
			{"api", "storage"},
		},
	}
	var buf bytes.Buffer
	RenderHealth(&buf, r)
	out := buf.String()
	if !strings.Contains(out, "CRITICAL") {
		t.Error("should contain CRITICAL")
	}
	if !strings.Contains(out, "auth") {
		t.Error("should list circular cycle members")
	}
}

func TestRenderHealth_NoRecommendations(t *testing.T) {
	r := &HealthReport{ProjectName: "clean", Status: StatusHealthy}
	var buf bytes.Buffer
	RenderHealth(&buf, r)
	if !strings.Contains(buf.String(), "No issues found") {
		t.Errorf("should say 'No issues found', got:\n%s", buf.String())
	}
}

func TestRenderHealth_MetricsTable(t *testing.T) {
	r := &HealthReport{
		ProjectName:    "proj",
		TotalFiles:     99,
		TotalFunctions: 333,
	}
	var buf bytes.Buffer
	RenderHealth(&buf, r)
	out := buf.String()
	if !strings.Contains(out, "99") {
		t.Error("should contain file count 99")
	}
	if !strings.Contains(out, "333") {
		t.Error("should contain function count 333")
	}
}

func TestRenderHealth_CriticalFilesSection(t *testing.T) {
	r := &HealthReport{
		ProjectName: "proj",
		CriticalFiles: []CriticalFile{
			{Path: "internal/api/client.go", RelationshipCount: 5},
		},
	}
	var buf bytes.Buffer
	RenderHealth(&buf, r)
	if !strings.Contains(buf.String(), "internal/api/client.go") {
		t.Errorf("should list critical file, got:\n%s", buf.String())
	}
}

func TestRenderHealth_RecommendationPriority(t *testing.T) {
	r := &HealthReport{
		ProjectName:     "proj",
		Recommendations: []Recommendation{{Priority: 1, Message: "Fix cycles NOW"}},
	}
	var buf bytes.Buffer
	RenderHealth(&buf, r)
	if !strings.Contains(buf.String(), "Fix cycles NOW") {
		t.Errorf("should contain recommendation message, got:\n%s", buf.String())
	}
}

// ── RenderRunPrompt ───────────────────────────────────────────────────────────

func TestRenderRunPrompt_ContainsGoalAndProject(t *testing.T) {
	d := &SDLCPromptData{
		ProjectName: "myapp",
		Language:    "Go",
		Goal:        "Add rate limiting to the order API",
		GeneratedAt: "2025-01-01 00:00:00 UTC",
	}
	var buf bytes.Buffer
	RenderRunPrompt(&buf, d)
	out := buf.String()
	if !strings.Contains(out, "Add rate limiting to the order API") {
		t.Error("should contain goal")
	}
	if !strings.Contains(out, "myapp") {
		t.Error("should contain project name")
	}
}

func TestRenderRunPrompt_Contains8Phases(t *testing.T) {
	d := &SDLCPromptData{ProjectName: "app", Goal: "test goal"}
	var buf bytes.Buffer
	RenderRunPrompt(&buf, d)
	out := buf.String()
	for i := 1; i <= 8; i++ {
		phase := fmt.Sprintf("Phase %d", i)
		if !strings.Contains(out, phase) {
			t.Errorf("missing %s in run prompt", phase)
		}
	}
}

func TestRenderRunPrompt_CircularDepWarning(t *testing.T) {
	d := &SDLCPromptData{
		ProjectName:  "app",
		Goal:         "fix stuff",
		CircularDeps: 2,
	}
	var buf bytes.Buffer
	RenderRunPrompt(&buf, d)
	out := buf.String()
	if !strings.Contains(out, "circular") {
		t.Error("should warn about circular deps when present")
	}
}

func TestRenderRunPrompt_ContainsGuardrails(t *testing.T) {
	d := &SDLCPromptData{ProjectName: "app", Goal: "do stuff"}
	var buf bytes.Buffer
	RenderRunPrompt(&buf, d)
	// Guardrails section should exist somewhere in the output
	out := buf.String()
	if !strings.Contains(out, "guardrail") && !strings.Contains(out, "Guardrail") {
		t.Error("should contain guardrails section")
	}
}

// ── RenderImprovePrompt ───────────────────────────────────────────────────────

func TestRenderImprovePrompt_ContainsHealthStatus(t *testing.T) {
	report := &HealthReport{
		Status: StatusDegraded,
		Recommendations: []Recommendation{
			{Priority: 2, Message: "Fix coupling in auth domain"},
		},
	}
	d := &SDLCPromptData{ProjectName: "myapp", HealthReport: report}
	var buf bytes.Buffer
	RenderImprovePrompt(&buf, d)
	out := buf.String()
	if !strings.Contains(out, "DEGRADED") {
		t.Error("should contain DEGRADED status")
	}
	if !strings.Contains(out, "Fix coupling in auth domain") {
		t.Error("should contain recommendation")
	}
}

func TestRenderImprovePrompt_ContainsScoringModel(t *testing.T) {
	d := &SDLCPromptData{
		ProjectName:  "app",
		HealthReport: &HealthReport{Status: StatusHealthy},
	}
	var buf bytes.Buffer
	RenderImprovePrompt(&buf, d)
	out := buf.String()
	// The improve prompt includes a scoring model
	if !strings.Contains(out, "circular") {
		t.Error("improve prompt should reference circular deps in scoring model")
	}
}

func TestRenderImprovePrompt_NoHealthReport(t *testing.T) {
	d := &SDLCPromptData{ProjectName: "app"} // nil HealthReport
	var buf bytes.Buffer
	// Should not panic
	RenderImprovePrompt(&buf, d)
	if buf.Len() == 0 {
		t.Error("should produce output even without HealthReport")
	}
}

// ── EnrichWithImpact ─────────────────────────────────────────────────────────

func TestEnrichWithImpact_AddsFiles(t *testing.T) {
	r := &HealthReport{Status: StatusHealthy}
	impact := &api.ImpactResult{
		Impacts: []api.ImpactTarget{
			{
				Target:      api.ImpactTargetInfo{File: "src/db.ts", Type: "file"},
				BlastRadius: api.BlastRadius{DirectDependents: 50, TransitiveDependents: 100, AffectedFiles: 10, RiskScore: "high"},
			},
		},
	}
	EnrichWithImpact(r, impact)
	if len(r.ImpactFiles) != 1 {
		t.Fatalf("expected 1 impact file, got %d", len(r.ImpactFiles))
	}
	if r.ImpactFiles[0].Path != "src/db.ts" {
		t.Errorf("expected src/db.ts, got %s", r.ImpactFiles[0].Path)
	}
	if r.ImpactFiles[0].Direct != 50 {
		t.Errorf("expected 50 direct, got %d", r.ImpactFiles[0].Direct)
	}
}

func TestEnrichWithImpact_CriticalDegrades(t *testing.T) {
	r := &HealthReport{Status: StatusHealthy}
	impact := &api.ImpactResult{
		Impacts: []api.ImpactTarget{
			{
				Target:      api.ImpactTargetInfo{File: "src/core.ts", Type: "file"},
				BlastRadius: api.BlastRadius{DirectDependents: 200, TransitiveDependents: 500, AffectedFiles: 30, RiskScore: "critical"},
			},
		},
	}
	EnrichWithImpact(r, impact)
	if r.Status != StatusDegraded {
		t.Errorf("expected DEGRADED, got %s", r.Status)
	}
}

func TestEnrichWithImpact_NonCriticalStaysHealthy(t *testing.T) {
	r := &HealthReport{Status: StatusHealthy}
	impact := &api.ImpactResult{
		Impacts: []api.ImpactTarget{
			{
				Target:      api.ImpactTargetInfo{File: "src/util.ts", Type: "file"},
				BlastRadius: api.BlastRadius{DirectDependents: 5, TransitiveDependents: 10, AffectedFiles: 2, RiskScore: "low"},
			},
		},
	}
	EnrichWithImpact(r, impact)
	if r.Status != StatusHealthy {
		t.Errorf("expected HEALTHY, got %s", r.Status)
	}
}

func TestEnrichWithImpact_CapsAt10(t *testing.T) {
	r := &HealthReport{Status: StatusHealthy}
	var impacts []api.ImpactTarget
	for i := 0; i < 15; i++ {
		impacts = append(impacts, api.ImpactTarget{
			Target:      api.ImpactTargetInfo{File: fmt.Sprintf("src/file%d.ts", i), Type: "file"},
			BlastRadius: api.BlastRadius{DirectDependents: 100 - i, RiskScore: "high"},
		})
	}
	EnrichWithImpact(r, &api.ImpactResult{Impacts: impacts})
	if len(r.ImpactFiles) != 10 {
		t.Errorf("expected 10 impact files (capped), got %d", len(r.ImpactFiles))
	}
}

func TestEnrichWithImpact_GeneratesRecommendations(t *testing.T) {
	r := &HealthReport{Status: StatusHealthy}
	impact := &api.ImpactResult{
		Impacts: []api.ImpactTarget{
			{
				Target:      api.ImpactTargetInfo{File: "src/auth.ts", Type: "file"},
				BlastRadius: api.BlastRadius{DirectDependents: 100, TransitiveDependents: 200, AffectedFiles: 20, RiskScore: "critical"},
			},
		},
	}
	EnrichWithImpact(r, impact)
	found := false
	for _, rec := range r.Recommendations {
		if strings.Contains(rec.Message, "src/auth.ts") && rec.Priority == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected critical recommendation for src/auth.ts")
	}
}

func TestEnrichWithImpact_EmptyImpact(t *testing.T) {
	r := &HealthReport{Status: StatusHealthy}
	EnrichWithImpact(r, &api.ImpactResult{})
	if r.Status != StatusHealthy {
		t.Errorf("expected HEALTHY with empty impact, got %s", r.Status)
	}
	if len(r.ImpactFiles) != 0 {
		t.Errorf("expected 0 impact files, got %d", len(r.ImpactFiles))
	}
}

func TestRenderHealth_ImpactSection(t *testing.T) {
	r := &HealthReport{
		ProjectName: "test",
		Status:      StatusDegraded,
		ImpactFiles: []ImpactFile{
			{Path: "src/core.ts", RiskScore: "critical", Direct: 100, Transitive: 200, Files: 15},
		},
	}
	var buf bytes.Buffer
	RenderHealth(&buf, r)
	out := buf.String()
	if !strings.Contains(out, "## Impact Analysis") {
		t.Error("expected Impact Analysis section")
	}
	if !strings.Contains(out, "src/core.ts") {
		t.Error("expected file path in impact table")
	}
	if !strings.Contains(out, "critical") {
		t.Error("expected risk score in impact table")
	}
}

func TestRenderHealth_NoImpactSection(t *testing.T) {
	r := &HealthReport{ProjectName: "test", Status: StatusHealthy}
	var buf bytes.Buffer
	RenderHealth(&buf, r)
	if strings.Contains(buf.String(), "## Impact Analysis") {
		t.Error("should not render Impact Analysis when no impact files")
	}
}
