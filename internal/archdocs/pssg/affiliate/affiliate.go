package affiliate

import (
	"net/url"
	"os"
	"strings"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
)

// Link represents a single affiliate link.
type Link struct {
	Provider string
	Term     string
	URL      string
}

// Provider generates affiliate links for a given search term.
type Provider struct {
	Name        string
	URLTemplate string // e.g., "https://www.amazon.com/s?k={{term}}&tag={{tag}}"
	Tag         string
}

// GenerateLink creates an affiliate URL for the given search term.
func (p *Provider) GenerateLink(term string) string {
	encoded := url.QueryEscape(term)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	// Also support + encoding like the TS version
	plusEncoded := strings.ReplaceAll(url.QueryEscape(term), "%20", "+")

	result := p.URLTemplate
	result = strings.ReplaceAll(result, "{{term}}", plusEncoded)
	result = strings.ReplaceAll(result, "{{tag}}", p.Tag)
	return result
}

// Registry holds all configured affiliate providers.
type Registry struct {
	Providers []Provider
}

// NewRegistry creates a Registry from config, reading env vars for tags.
func NewRegistry(cfg config.AffiliatesConfig) *Registry {
	var providers []Provider
	for _, pc := range cfg.Providers {
		tag := ""
		if pc.EnvVar != "" {
			tag = os.Getenv(pc.EnvVar)
		}
		// Skip providers that require an env var but don't have one set
		if tag == "" && !pc.AlwaysInclude {
			continue
		}
		providers = append(providers, Provider{
			Name:        pc.Name,
			URLTemplate: pc.URLTemplate,
			Tag:         tag,
		})
	}
	return &Registry{Providers: providers}
}

// GenerateLinks creates affiliate links for all search terms from enrichment data.
func (r *Registry) GenerateLinks(enrichmentData map[string]interface{}, searchTermPaths []string) []Link {
	if len(r.Providers) == 0 || enrichmentData == nil {
		return nil
	}

	// Extract search terms from enrichment data using configured paths
	var terms []string
	for _, path := range searchTermPaths {
		terms = append(terms, extractTerms(enrichmentData, path)...)
	}

	var links []Link
	for _, provider := range r.Providers {
		for _, term := range terms {
			links = append(links, Link{
				Provider: provider.Name,
				Term:     term,
				URL:      provider.GenerateLink(term),
			})
		}
	}
	return links
}

// extractTerms extracts string values from enrichment data at the given path.
// Supports paths like "ingredients[].searchTerm" and "gear[].searchTerm".
func extractTerms(data map[string]interface{}, path string) []string {
	parts := strings.Split(path, "[].")
	if len(parts) != 2 {
		// Simple field
		if v, ok := data[path]; ok {
			if s, ok := v.(string); ok {
				return []string{s}
			}
		}
		return nil
	}

	arrayField := parts[0]
	subField := parts[1]

	arr, ok := data[arrayField]
	if !ok {
		return nil
	}

	items, ok := arr.([]interface{})
	if !ok {
		return nil
	}

	var results []string
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			if v, ok := m[subField]; ok {
				if s, ok := v.(string); ok && s != "" {
					results = append(results, s)
				}
			}
		}
	}
	return results
}
