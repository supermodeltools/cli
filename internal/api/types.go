package api

import (
	"encoding/json"
	"fmt"
)

// Node represents a graph node returned by the Supermodel API.
type Node struct {
	ID         string         `json:"id"`
	Labels     []string       `json:"labels"`
	Properties map[string]any `json:"properties"`
}

// Prop returns the first non-empty string value from the node's properties.
func (n Node) Prop(keys ...string) string {
	for _, k := range keys {
		if v, ok := n.Properties[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// HasLabel reports whether the node carries the given label.
func (n Node) HasLabel(label string) bool {
	for _, l := range n.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// Relationship is a directed edge between two graph nodes.
type Relationship struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	StartNode  string         `json:"startNode"`
	EndNode    string         `json:"endNode"`
	Properties map[string]any `json:"properties,omitempty"`
}

// Graph is the unified response type for /v1/graphs/supermodel and
// /v1/repos/{id}/graph/display. The API serialises relationships as either
// "edges" or "relationships" depending on the endpoint; Rels() unifies both.
type Graph struct {
	Nodes         []Node         `json:"nodes"`
	Edges         []Relationship `json:"edges"`
	Relationships []Relationship `json:"relationships"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// Rels returns all relationships regardless of which JSON field they came from.
func (g *Graph) Rels() []Relationship {
	if len(g.Relationships) > 0 {
		return g.Relationships
	}
	return g.Edges
}

// RepoID returns the repoId from graph metadata, or "".
func (g *Graph) RepoID() string {
	if g.Metadata == nil {
		return ""
	}
	if id, ok := g.Metadata["repoId"].(string); ok {
		return id
	}
	return ""
}

// NodesByLabel returns all nodes that carry the given label.
func (g *Graph) NodesByLabel(label string) []Node {
	var out []Node
	for _, n := range g.Nodes {
		if n.HasLabel(label) {
			out = append(out, n)
		}
	}
	return out
}

// NodeByID returns the node with the given ID, if present.
func (g *Graph) NodeByID(id string) (Node, bool) {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

// SupermodelIR is the full structured response returned inside a completed job
// result from /v1/graphs/supermodel. It contains high-level domain information
// in addition to the raw node/edge graph captured by Graph.
type SupermodelIR struct {
	Repo     string         `json:"repo"`
	Summary  map[string]any `json:"summary"`
	Metadata IRMetadata     `json:"metadata"`
	Domains  []IRDomain     `json:"domains"`
	Graph    IRGraph        `json:"graph"`
}

// IRMetadata holds file-count and language statistics from the API response.
type IRMetadata struct {
	FileCount int      `json:"fileCount"`
	Languages []string `json:"languages"`
}

// IRGraph is the raw node/relationship sub-graph embedded in SupermodelIR.
type IRGraph struct {
	Nodes         []IRNode         `json:"nodes"`
	Relationships []IRRelationship `json:"relationships"`
}

// IRNode is a single node in the IRGraph.
type IRNode struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// IRRelationship is a directed edge in the IRGraph.
type IRRelationship struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// IRDomain is the raw representation of a semantic domain from the API.
type IRDomain struct {
	Name               string        `json:"name"`
	DescriptionSummary string        `json:"descriptionSummary"`
	KeyFiles           []string      `json:"keyFiles"`
	Responsibilities   []string      `json:"responsibilities"`
	Subdomains         []IRSubdomain `json:"subdomains"`
}

// IRSubdomain is a named sub-area within an IRDomain.
type IRSubdomain struct {
	Name               string `json:"name"`
	DescriptionSummary string `json:"descriptionSummary"`
}

// SidecarIR is the full structured response from /v1/graphs/supermodel used
// by the sidecars vertical slice. Unlike SupermodelIR (which uses simplified
// IRNode/IRRelationship stubs), SidecarIR preserves the complete node graph
// with IDs, labels, and properties required for sidecar rendering.
type SidecarIR struct {
	Repo     string          `json:"repo"`
	Summary  map[string]any  `json:"summary"`
	Metadata IRMetadata      `json:"metadata"`
	Domains  []SidecarDomain `json:"domains"`
	Graph    SidecarGraph    `json:"graph"`
}

// SidecarGraph is the full node/relationship graph embedded in SidecarIR.
type SidecarGraph struct {
	Nodes         []Node         `json:"nodes"`
	Relationships []Relationship `json:"relationships"`
}

// SidecarDomain is a semantic domain from the API with file references.
type SidecarDomain struct {
	Name               string             `json:"name"`
	DescriptionSummary string             `json:"descriptionSummary"`
	KeyFiles           []string           `json:"keyFiles"`
	Responsibilities   []string           `json:"responsibilities"`
	Subdomains         []SidecarSubdomain `json:"subdomains"`
}

// SidecarSubdomain is a named sub-area within a SidecarDomain.
type SidecarSubdomain struct {
	Name               string   `json:"name"`
	DescriptionSummary string   `json:"descriptionSummary"`
	Files              []string `json:"files"`
	KeyFiles           []string `json:"keyFiles"`
}

// GraphFromSidecarIR builds a display Graph from a SidecarIR response.
// SidecarIR uses the same Node/Relationship types, so this is a zero-copy
// extraction that also populates the repoId metadata field.
func GraphFromSidecarIR(ir *SidecarIR) *Graph {
	return &Graph{
		Nodes:         ir.Graph.Nodes,
		Relationships: ir.Graph.Relationships,
		Metadata: map[string]any{
			"repoId": ir.Repo,
		},
	}
}

// JobResponse is the async envelope returned by the API for long-running jobs.
type JobResponse struct {
	Status     string          `json:"status"`
	JobID      string          `json:"jobId"`
	RetryAfter int             `json:"retryAfter"`
	Error      *string         `json:"error"`
	Result     json.RawMessage `json:"result"`
}

// jobResult is the inner result object containing the graph.
type jobResult struct {
	Graph Graph `json:"graph"`
}

// DeadCodeResult is the result from /v1/analysis/dead-code.
type DeadCodeResult struct {
	Metadata           DeadCodeMetadata    `json:"metadata"`
	DeadCodeCandidates []DeadCodeCandidate `json:"deadCodeCandidates"`
	AliveCode          []AliveCode         `json:"aliveCode"`
	EntryPoints        []EntryPoint        `json:"entryPoints"`
}

// DeadCodeMetadata holds summary stats for a dead code analysis.
type DeadCodeMetadata struct {
	TotalDeclarations  int    `json:"totalDeclarations"`
	DeadCodeCandidates int    `json:"deadCodeCandidates"`
	AliveCode          int    `json:"aliveCode"`
	AnalysisMethod     string `json:"analysisMethod"`
	AnalysisStartTime  string `json:"analysisStartTime"`
	AnalysisEndTime    string `json:"analysisEndTime"`
}

// DeadCodeCandidate is a function flagged as unreachable.
type DeadCodeCandidate struct {
	File       string `json:"file"`
	Name       string `json:"name"`
	Line       int    `json:"line"`
	Type       string `json:"type"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

// AliveCode is a function confirmed as reachable.
type AliveCode struct {
	File        string `json:"file"`
	Name        string `json:"name"`
	Line        int    `json:"line"`
	Type        string `json:"type"`
	CallerCount int    `json:"callerCount"`
}

// EntryPoint is a detected entry point that should not be flagged as dead.
type EntryPoint struct {
	File string `json:"file"`
}

// ImpactResult is the result from /v1/analysis/impact.
type ImpactResult struct {
	Metadata      ImpactMetadata      `json:"metadata"`
	Impacts       []ImpactTarget      `json:"impacts"`
	GlobalMetrics ImpactGlobalMetrics `json:"globalMetrics"`
}

// ImpactMetadata holds summary stats for an impact analysis.
type ImpactMetadata struct {
	TotalFiles        int    `json:"totalFiles"`
	TotalFunctions    int    `json:"totalFunctions"`
	TargetsAnalyzed   int    `json:"targetsAnalyzed"`
	AnalysisMethod    string `json:"analysisMethod"`
	AnalysisStartTime string `json:"analysisStartTime"`
	AnalysisEndTime   string `json:"analysisEndTime"`
}

// ImpactTarget is the impact analysis result for a single target.
type ImpactTarget struct {
	Target              ImpactTargetInfo     `json:"target"`
	BlastRadius         BlastRadius          `json:"blastRadius"`
	AffectedFunctions   []AffectedFunction   `json:"affectedFunctions"`
	AffectedFiles       []AffectedFile       `json:"affectedFiles"`
	EntryPointsAffected []AffectedEntryPoint `json:"entryPointsAffected"`
}

// ImpactTargetInfo identifies the file or function being analyzed.
type ImpactTargetInfo struct {
	File string `json:"file"`
	Name string `json:"name,omitempty"`
	Line int    `json:"line,omitempty"`
	Type string `json:"type"`
}

// BlastRadius holds blast radius metrics for a target.
type BlastRadius struct {
	DirectDependents     int      `json:"directDependents"`
	TransitiveDependents int      `json:"transitiveDependents"`
	AffectedFiles        int      `json:"affectedFiles"`
	AffectedDomains      []string `json:"affectedDomains,omitempty"`
	RiskScore            string   `json:"riskScore"`
	RiskFactors          []string `json:"riskFactors,omitempty"`
}

// AffectedFunction is a function affected by changes to the target.
type AffectedFunction struct {
	File         string `json:"file"`
	Name         string `json:"name"`
	Line         int    `json:"line,omitempty"`
	Type         string `json:"type"`
	Distance     int    `json:"distance"`
	Relationship string `json:"relationship"`
}

// AffectedFile is a file affected by changes to the target.
type AffectedFile struct {
	File                   string `json:"file"`
	DirectDependencies     int    `json:"directDependencies"`
	TransitiveDependencies int    `json:"transitiveDependencies"`
}

// AffectedEntryPoint is an entry point affected by changes to the target.
type AffectedEntryPoint struct {
	File string `json:"file"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// ImpactGlobalMetrics holds global metrics across all analyzed targets.
type ImpactGlobalMetrics struct {
	MostCriticalFiles       []CriticalFileMetric    `json:"mostCriticalFiles,omitempty"`
	CrossDomainDependencies []CrossDomainDependency `json:"crossDomainDependencies,omitempty"`
}

// CriticalFileMetric identifies a high-dependent-count file.
type CriticalFileMetric struct {
	File           string `json:"file"`
	DependentCount int    `json:"dependentCount"`
}

// CrossDomainDependency identifies a dependency crossing domain boundaries.
type CrossDomainDependency struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceDomain string `json:"sourceDomain"`
	TargetDomain string `json:"targetDomain"`
}

// ShareRequest is the payload for POST /v1/share.
type ShareRequest struct {
	ProjectName string `json:"project_name"`
	Status      string `json:"status"`
	Content     string `json:"content"` // rendered Markdown report
}

// ShareResponse is returned by POST /v1/share.
type ShareResponse struct {
	URL string `json:"url"`
}

// Error represents a non-2xx response from the API.
type Error struct {
	StatusCode int    `json:"-"`
	Status     int    `json:"status"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *Error) Error() string {
	code := e.StatusCode
	if code == 0 {
		code = e.Status
	}
	if e.Code != "" {
		return fmt.Sprintf("API error %d (%s): %s", code, e.Code, e.Message)
	}
	return fmt.Sprintf("API error %d: %s", code, e.Message)
}
