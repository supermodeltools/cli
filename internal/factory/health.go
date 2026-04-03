package factory

import (
	"fmt"
	"sort"
	"time"

	"github.com/supermodeltools/cli/internal/api"
)

// Analyze derives a HealthReport from the raw SupermodelIR returned by the API.
func Analyze(ir *api.SupermodelIR, projectName string) *HealthReport {
	r := &HealthReport{
		ProjectName:    projectName,
		AnalyzedAt:     time.Now().UTC(),
		Status:         StatusHealthy,
		TotalFiles:     summaryInt(ir.Summary, "filesProcessed"),
		TotalFunctions: summaryInt(ir.Summary, "functions"),
		Languages:      ir.Metadata.Languages,
		ExternalDeps:   buildExternalDeps(ir),
	}
	if len(r.Languages) > 0 {
		r.Language = r.Languages[0]
	}
	if v, ok := ir.Summary["primaryLanguage"]; ok {
		if s, ok := v.(string); ok && s != "" {
			r.Language = s
		}
	}

	incoming, outgoing := buildCouplingMaps(ir)
	r.CriticalFiles = buildCriticalFiles(ir)
	r.Domains = buildDomainHealthList(ir, incoming, outgoing)
	r.CircularDeps, r.CircularCycles = detectCircularDeps(ir)
	r.Status = scoreStatus(r)
	r.Recommendations = generateRecommendations(r)
	return r
}

// EnrichWithImpact adds impact analysis results to an existing HealthReport
// and re-scores status and recommendations.
func EnrichWithImpact(r *HealthReport, impact *api.ImpactResult) {
	for i := range impact.Impacts {
		imp := &impact.Impacts[i]
		r.ImpactFiles = append(r.ImpactFiles, ImpactFile{
			Path:       imp.Target.File,
			RiskScore:  imp.BlastRadius.RiskScore,
			Direct:     imp.BlastRadius.DirectDependents,
			Transitive: imp.BlastRadius.TransitiveDependents,
			Files:      imp.BlastRadius.AffectedFiles,
		})
	}
	// Also pull in global critical files if the API returned them.
	for i := range impact.GlobalMetrics.MostCriticalFiles {
		cf := &impact.GlobalMetrics.MostCriticalFiles[i]
		r.ImpactFiles = append(r.ImpactFiles, ImpactFile{
			Path:   cf.File,
			Direct: cf.DependentCount,
		})
	}
	// Cap to top 10 by direct dependents.
	sort.Slice(r.ImpactFiles, func(i, j int) bool {
		return r.ImpactFiles[i].Direct > r.ImpactFiles[j].Direct
	})
	if len(r.ImpactFiles) > 10 {
		r.ImpactFiles = r.ImpactFiles[:10]
	}
	// Re-score and regenerate recommendations with impact data.
	r.Status = scoreStatus(r)
	r.Recommendations = generateRecommendations(r)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func buildExternalDeps(ir *api.SupermodelIR) []string {
	seen := make(map[string]bool)
	var deps []string
	for i := range ir.Graph.Nodes {
		n := &ir.Graph.Nodes[i]
		if n.Type == "ExternalDependency" && n.Name != "" && !seen[n.Name] {
			seen[n.Name] = true
			deps = append(deps, n.Name)
		}
	}
	sort.Strings(deps)
	return deps
}

func buildCouplingMaps(ir *api.SupermodelIR) (incoming, outgoing map[string][]string) {
	incoming = make(map[string][]string)
	outgoing = make(map[string][]string)
	// Deduplicate edges: the graph may emit the same source→target pair multiple
	// times, which would inflate coupling counts and trigger false recommendations.
	seen := make(map[string]bool)
	for i := range ir.Graph.Relationships {
		rel := &ir.Graph.Relationships[i]
		if rel.Type != "DOMAIN_RELATES" || rel.Source == "" || rel.Target == "" {
			continue
		}
		key := rel.Source + "→" + rel.Target
		if seen[key] {
			continue
		}
		seen[key] = true
		outgoing[rel.Source] = append(outgoing[rel.Source], rel.Target)
		incoming[rel.Target] = append(incoming[rel.Target], rel.Source)
	}
	return incoming, outgoing
}

func buildCriticalFiles(ir *api.SupermodelIR) []CriticalFile {
	counts := make(map[string]int)
	for i := range ir.Domains {
		d := &ir.Domains[i]
		seen := make(map[string]bool, len(d.KeyFiles))
		for _, f := range d.KeyFiles {
			if !seen[f] {
				seen[f] = true
				counts[f]++
			}
		}
	}
	var files []CriticalFile
	for path, count := range counts {
		if count > 1 {
			files = append(files, CriticalFile{Path: path, RelationshipCount: count})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].RelationshipCount != files[j].RelationshipCount {
			return files[i].RelationshipCount > files[j].RelationshipCount
		}
		return files[i].Path < files[j].Path
	})
	const maxCritical = 10
	if len(files) > maxCritical {
		files = files[:maxCritical]
	}
	return files
}

func buildDomainHealthList(ir *api.SupermodelIR, incoming, outgoing map[string][]string) []DomainHealth {
	domains := make([]DomainHealth, 0, len(ir.Domains))
	for i := range ir.Domains {
		d := &ir.Domains[i]
		dh := DomainHealth{
			Name:             d.Name,
			Description:      d.DescriptionSummary,
			KeyFileCount:     len(d.KeyFiles),
			Responsibilities: len(d.Responsibilities),
			Subdomains:       len(d.Subdomains),
			IncomingDeps:     append([]string(nil), incoming[d.Name]...),
			OutgoingDeps:     append([]string(nil), outgoing[d.Name]...),
		}
		sort.Strings(dh.IncomingDeps)
		sort.Strings(dh.OutgoingDeps)
		domains = append(domains, dh)
	}
	return domains
}

func detectCircularDeps(ir *api.SupermodelIR) (count int, cycles [][]string) {
	seen := make(map[string]bool)
	for i := range ir.Graph.Relationships {
		rel := &ir.Graph.Relationships[i]
		if rel.Type != "CIRCULAR_DEPENDENCY" && rel.Type != "CIRCULAR_DEP" {
			continue
		}
		key := rel.Source + "→" + rel.Target
		if !seen[key] {
			seen[key] = true
			cycles = append(cycles, []string{rel.Source, rel.Target})
		}
	}
	count = len(cycles)
	return count, cycles
}

func scoreStatus(r *HealthReport) HealthStatus {
	if r.CircularDeps > 0 {
		return StatusCritical
	}
	for i := range r.ImpactFiles {
		if r.ImpactFiles[i].RiskScore == "critical" {
			return StatusDegraded
		}
	}
	for i := range r.Domains {
		if len(r.Domains[i].IncomingDeps) >= 5 {
			return StatusDegraded
		}
	}
	return StatusHealthy
}

func summaryInt(summary map[string]any, key string) int {
	if v, ok := summary[key]; ok {
		if n, ok := v.(float64); ok {
			return int(n)
		}
	}
	return 0
}

func generateRecommendations(r *HealthReport) []Recommendation {
	var recs []Recommendation

	if r.CircularDeps > 0 {
		recs = append(recs, Recommendation{
			Priority: 1,
			Message:  pluralf("Resolve %d circular dependency cycle%s — these block architectural validation (Phase 2).", r.CircularDeps),
		})
	}

	for i := range r.Domains {
		d := &r.Domains[i]
		if len(d.IncomingDeps) >= 3 { // matches CouplingStatus warning threshold
			recs = append(recs, Recommendation{
				Priority: 2,
				Message: fmt.Sprintf(
					"Domain %q has %d dependents — consider extracting a shared kernel to reduce coupling.",
					d.Name, len(d.IncomingDeps),
				),
			})
		}
	}

	for i := range r.Domains {
		d := &r.Domains[i]
		if d.KeyFileCount == 0 {
			recs = append(recs, Recommendation{
				Priority: 3,
				Message:  fmt.Sprintf("Domain %q has no key files recorded — verify domain classification is complete.", d.Name),
			})
		}
	}

	for i := range r.CriticalFiles {
		f := &r.CriticalFiles[i]
		if f.RelationshipCount >= 4 {
			recs = append(recs, Recommendation{
				Priority: 2,
				Message:  fmt.Sprintf("File %q is referenced by %d domains — high blast radius; protect its public interface.", f.Path, f.RelationshipCount),
			})
		}
	}

	for i := range r.ImpactFiles {
		f := &r.ImpactFiles[i]
		if f.RiskScore == "critical" {
			recs = append(recs, Recommendation{
				Priority: 1,
				Message:  fmt.Sprintf("File %q has critical blast radius (%d direct, %d transitive dependents) — changes here affect %d files.", f.Path, f.Direct, f.Transitive, f.Files),
			})
		}
	}

	sort.Slice(recs, func(i, j int) bool { return recs[i].Priority < recs[j].Priority })
	return recs
}

func pluralf(template string, n int) string {
	suffix := "s"
	if n == 1 {
		suffix = ""
	}
	return fmt.Sprintf(template, n, suffix)
}
