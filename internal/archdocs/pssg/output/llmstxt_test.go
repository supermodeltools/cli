package output

import (
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
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
