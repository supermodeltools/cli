package output

import (
	"fmt"
	"sort"
	"strings"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/taxonomy"
)

// GenerateLlmsTxt generates an llms.txt file in the llmstxt.org format.
func GenerateLlmsTxt(cfg *config.Config, entities []*entity.Entity, taxonomies []taxonomy.Taxonomy) string {
	var lines []string

	// Header
	lines = append(lines, fmt.Sprintf("# %s", cfg.Site.Name))
	lines = append(lines, "")

	// Tagline
	if cfg.LlmsTxt.Tagline != "" {
		lines = append(lines, fmt.Sprintf("> %s", cfg.LlmsTxt.Tagline))
		lines = append(lines, "")
	}

	// Entities section
	entityLabel := cfg.Data.EntityType
	if entityLabel == "" {
		entityLabel = "Item"
	}
	lines = append(lines, fmt.Sprintf("## %ss", strings.Title(entityLabel)))

	// Sort entities by title
	sorted := make([]*entity.Entity, len(entities))
	copy(sorted, entities)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].GetString("title") < sorted[j].GetString("title")
	})

	for _, e := range sorted {
		title := e.GetString("title")
		desc := e.GetString("description")
		url := fmt.Sprintf("%s/%s.html", cfg.Site.BaseURL, e.Slug)
		lines = append(lines, fmt.Sprintf("- [%s](%s): %s", title, url, desc))
	}
	lines = append(lines, "")

	// Taxonomy sections
	for _, taxName := range cfg.LlmsTxt.Taxonomies {
		for _, tax := range taxonomies {
			if tax.Name == taxName {
				lines = append(lines, fmt.Sprintf("## %s", tax.Label))
				for _, entry := range tax.Entries {
					url := fmt.Sprintf("%s/%s/%s.html", cfg.Site.BaseURL, tax.Name, entry.Slug)
					lines = append(lines, fmt.Sprintf("- [%s](%s)", entry.Name, url))
				}
				lines = append(lines, "")
				break
			}
		}
	}

	return strings.Join(lines, "\n")
}
