package restore

import (
	"sort"
	"time"

	"github.com/supermodeltools/cli/internal/api"
)

// ProjectGraph is the processed project model used for rendering.
// It is derived from either a SupermodelIR API response or a local file scan.
type ProjectGraph struct {
	Name                 string                    `json:"name"`
	Language             string                    `json:"language"`
	Framework            string                    `json:"framework,omitempty"`
	Description          string                    `json:"description,omitempty"`
	Domains              []Domain                  `json:"domains"`
	ExternalDeps         []string                  `json:"external_deps,omitempty"`
	CriticalFiles        []CriticalFile            `json:"critical_files,omitempty"`
	Stats                Stats                     `json:"stats"`
	Cycles               []CircularDependencyCycle `json:"cycles,omitempty"`
	CircularDepsAnalyzed bool                      `json:"circular_deps_analyzed"`
	UpdatedAt            time.Time                 `json:"updated_at"`
}

// Domain is a semantic area of the codebase.
type Domain struct {
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	KeyFiles         []string    `json:"key_files"`
	Responsibilities []string    `json:"responsibilities"`
	Subdomains       []Subdomain `json:"subdomains,omitempty"`
	DependsOn        []string    `json:"depends_on,omitempty"`
}

// Subdomain is a named sub-area within a Domain.
type Subdomain struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CriticalFile is a highly-referenced file derived from domain key file counts.
type CriticalFile struct {
	Path              string `json:"path"`
	RelationshipCount int    `json:"relationship_count"`
}

// Stats holds aggregate codebase statistics.
type Stats struct {
	TotalFiles               int      `json:"total_files"`
	TotalFunctions           int      `json:"total_functions"`
	Languages                []string `json:"languages,omitempty"`
	CircularDependencyCycles int      `json:"circular_dependency_cycles,omitempty"`
}

// CircularDependencyCycle is a single circular import chain.
type CircularDependencyCycle struct {
	Cycle []string `json:"cycle"`
}

// FromSupermodelIR converts the raw API response into a ProjectGraph.
func FromSupermodelIR(ir *api.SupermodelIR, projectName string) *ProjectGraph {
	lang := ""
	if len(ir.Metadata.Languages) > 0 {
		lang = ir.Metadata.Languages[0]
	}
	if v, ok := ir.Summary["primaryLanguage"]; ok {
		if s, ok := v.(string); ok && s != "" {
			lang = s
		}
	}

	summaryInt := func(key string) int {
		if v, ok := ir.Summary[key]; ok {
			if n, ok := v.(float64); ok {
				return int(n)
			}
		}
		return 0
	}

	// Build domain → dependsOn map from DOMAIN_RELATES edges.
	dependsOn := make(map[string][]string)
	for _, rel := range ir.Graph.Relationships {
		if rel.Type == "DOMAIN_RELATES" && rel.Source != "" && rel.Target != "" {
			dependsOn[rel.Source] = append(dependsOn[rel.Source], rel.Target)
		}
	}

	domains := make([]Domain, 0, len(ir.Domains))
	for _, d := range ir.Domains {
		subs := make([]Subdomain, 0, len(d.Subdomains))
		for _, s := range d.Subdomains {
			subs = append(subs, Subdomain{Name: s.Name, Description: s.DescriptionSummary})
		}
		domains = append(domains, Domain{
			Name:             d.Name,
			Description:      d.DescriptionSummary,
			KeyFiles:         d.KeyFiles,
			Responsibilities: d.Responsibilities,
			Subdomains:       subs,
			DependsOn:        dependsOn[d.Name],
		})
	}

	var extDeps []string
	for _, node := range ir.Graph.Nodes {
		if node.Type == "ExternalDependency" && node.Name != "" {
			extDeps = append(extDeps, node.Name)
		}
	}

	g := &ProjectGraph{
		Name:         projectName,
		Language:     lang,
		Domains:      domains,
		ExternalDeps: extDeps,
		Stats: Stats{
			TotalFiles:     summaryInt("filesProcessed"),
			TotalFunctions: summaryInt("functions"),
			Languages:      ir.Metadata.Languages,
		},
		UpdatedAt: time.Now(),
	}
	g.CriticalFiles = computeCriticalFiles(g.Domains, 10)
	return g
}

// computeCriticalFiles derives the most-referenced files by counting how many
// domains list each file as a key file. The top n files are returned.
func computeCriticalFiles(domains []Domain, n int) []CriticalFile {
	if n <= 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, d := range domains {
		seen := make(map[string]struct{}, len(d.KeyFiles))
		for _, f := range d.KeyFiles {
			if _, exists := seen[f]; exists {
				continue
			}
			seen[f] = struct{}{}
			counts[f]++
		}
	}
	files := make([]CriticalFile, 0, len(counts))
	for path, count := range counts {
		files = append(files, CriticalFile{Path: path, RelationshipCount: count})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].RelationshipCount != files[j].RelationshipCount {
			return files[i].RelationshipCount > files[j].RelationshipCount
		}
		return files[i].Path < files[j].Path
	})
	if len(files) > n {
		files = files[:n]
	}
	return files
}
