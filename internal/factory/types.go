package factory

import "time"

// HealthStatus is the overall verdict of a health analysis.
type HealthStatus string

const (
	// StatusHealthy means no critical issues detected.
	StatusHealthy HealthStatus = "HEALTHY"
	// StatusDegraded means non-critical issues are present (high coupling, warnings).
	StatusDegraded HealthStatus = "DEGRADED"
	// StatusCritical means blocking issues exist (circular dependencies).
	StatusCritical HealthStatus = "CRITICAL"
)

// HealthReport is the output of a factory health analysis.
type HealthReport struct {
	ProjectName    string
	Language       string
	AnalyzedAt     time.Time
	Status         HealthStatus
	TotalFiles     int
	TotalFunctions int
	Languages      []string
	ExternalDeps   []string

	// Circular dependency data
	CircularDeps   int
	CircularCycles [][]string

	// Per-domain health
	Domains []DomainHealth

	// Highest blast-radius files
	CriticalFiles []CriticalFile

	// Prioritised action items
	Recommendations []Recommendation
}

// DomainHealth holds structural metrics for a single semantic domain.
type DomainHealth struct {
	Name             string
	Description      string
	KeyFileCount     int
	Responsibilities int
	Subdomains       int
	// IncomingDeps are domain names that depend on this domain.
	IncomingDeps []string
	// OutgoingDeps are domain names this domain depends on.
	OutgoingDeps []string
}

// CouplingStatus classifies a domain's coupling level.
func (d *DomainHealth) CouplingStatus() string {
	n := len(d.IncomingDeps)
	switch {
	case n >= 5:
		return "⛔ HIGH"
	case n >= 3:
		return "⚠️  WARN"
	default:
		return "✅ OK"
	}
}

// CriticalFile is a high blast-radius file derived from cross-domain references.
type CriticalFile struct {
	Path              string
	RelationshipCount int
}

// Recommendation is a prioritised actionable finding.
type Recommendation struct {
	// Priority: 1=critical, 2=high, 3=medium.
	Priority int
	Message  string
}

// SDLCPromptData holds the inputs for rendering a factory run/improve prompt.
type SDLCPromptData struct {
	ProjectName    string
	Language       string
	TotalFiles     int
	TotalFunctions int
	ExternalDeps   []string
	Domains        []DomainHealth
	CriticalFiles  []CriticalFile
	CircularDeps   int

	// Goal is non-empty for the "run" command.
	Goal string
	// HealthReport is non-nil for the "improve" command.
	HealthReport *HealthReport

	GeneratedAt string
}
