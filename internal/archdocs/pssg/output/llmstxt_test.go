package output

import (
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/taxonomy"
)

func minimalCfg(entityType string) *config.Config {
	return &config.Config{
		Site:    config.SiteConfig{Name: "TestSite", BaseURL: "https://example.com"},
		LlmsTxt: config.LlmsTxtConfig{Enabled: true},
		Data:    config.DataConfig{EntityType: entityType},
	}
}

// TestGenerateLlmsTxt_DefaultEntityType verifies that when entity_type is
// not configured the section header reads "## Items" (not "## Itemss").
func TestGenerateLlmsTxt_DefaultEntityType(t *testing.T) {
	cfg := minimalCfg("") // empty → should default to "Item" → "Items"
	result := GenerateLlmsTxt(cfg, nil, nil)

	if strings.Contains(result, "## Itemss") {
		t.Error(`section header contains "## Itemss" — double-plural bug not fixed`)
	}
	if !strings.Contains(result, "## Items") {
		t.Errorf("expected section header \"## Items\", got:\n%s", result)
	}
}

// TestGenerateLlmsTxt_ConfiguredEntityType verifies that a configured entity
// type like "recipe" produces "## Recipes" (not "## Recipess").
func TestGenerateLlmsTxt_ConfiguredEntityType(t *testing.T) {
	cfg := minimalCfg("recipe")
	result := GenerateLlmsTxt(cfg, nil, nil)

	if strings.Contains(result, "## Recipess") {
		t.Error(`section header contains "## Recipess" — unexpected double-plural`)
	}
	if !strings.Contains(result, "## Recipes") {
		t.Errorf("expected section header \"## Recipes\", got:\n%s", result)
	}
}

func TestGenerateLlmsTxt_WithTagline(t *testing.T) {
	cfg := &config.Config{
		Site:    config.SiteConfig{Name: "MySite", BaseURL: "https://example.com"},
		LlmsTxt: config.LlmsTxtConfig{Enabled: true, Tagline: "The best recipes online"},
		Data:    config.DataConfig{EntityType: "recipe"},
	}
	result := GenerateLlmsTxt(cfg, nil, nil)
	if !strings.Contains(result, "> The best recipes online") {
		t.Errorf("expected tagline in output:\n%s", result)
	}
}

func TestGenerateLlmsTxt_WithTaxonomies(t *testing.T) {
	cfg := &config.Config{
		Site:    config.SiteConfig{Name: "MySite", BaseURL: "https://example.com"},
		LlmsTxt: config.LlmsTxtConfig{Enabled: true, Taxonomies: []string{"cuisine"}},
		Data:    config.DataConfig{EntityType: "recipe"},
	}
	taxList := []taxonomy.Taxonomy{{
		Name:  "cuisine",
		Label: "Cuisines",
		Entries: []taxonomy.Entry{
			{Name: "Italian", Slug: "italian"},
		},
	}}
	result := GenerateLlmsTxt(cfg, nil, taxList)
	if !strings.Contains(result, "## Cuisines") {
		t.Errorf("expected taxonomy header in output:\n%s", result)
	}
	if !strings.Contains(result, "[Italian](https://example.com/cuisine/italian.html)") {
		t.Errorf("expected taxonomy entry link in output:\n%s", result)
	}
}

// TestGenerateLlmsTxt_SortsByTitle verifies that the sort comparator fires when
// 2+ entities are present, covering the sort.Slice comparator lambda.
func TestGenerateLlmsTxt_SortsByTitle(t *testing.T) {
	cfg := minimalCfg("recipe")
	entities := []*entity.Entity{
		{Slug: "z-cake", Fields: map[string]interface{}{"title": "Z Cake", "description": "last"}},
		{Slug: "a-soup", Fields: map[string]interface{}{"title": "A Soup", "description": "first"}},
	}
	result := GenerateLlmsTxt(cfg, entities, nil)
	aIdx := strings.Index(result, "A Soup")
	zIdx := strings.Index(result, "Z Cake")
	if aIdx == -1 || zIdx == -1 {
		t.Fatalf("both entities should appear in output:\n%s", result)
	}
	if aIdx > zIdx {
		t.Errorf("A Soup should appear before Z Cake (sorted by title)")
	}
}

// TestGenerateLlmsTxt_EntityLinks verifies entity URLs are rendered correctly.
func TestGenerateLlmsTxt_EntityLinks(t *testing.T) {
	cfg := minimalCfg("recipe")
	entities := []*entity.Entity{
		{
			Slug: "chocolate-cake",
			Fields: map[string]interface{}{
				"title":       "Chocolate Cake",
				"description": "A rich, moist chocolate cake.",
			},
		},
	}

	result := GenerateLlmsTxt(cfg, entities, nil)

	want := "- [Chocolate Cake](https://example.com/chocolate-cake.html): A rich, moist chocolate cake."
	if !strings.Contains(result, want) {
		t.Errorf("expected entity link line:\n  %s\ngot:\n%s", want, result)
	}
}
