package affiliate

import (
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
)

// ── Provider.GenerateLink ─────────────────────────────────────────────────────

func TestGenerateLink_Basic(t *testing.T) {
	p := &Provider{
		Name:        "amazon",
		URLTemplate: "https://amazon.com/s?k={{term}}&tag={{tag}}",
		Tag:         "mytag-20",
	}
	url := p.GenerateLink("cast iron pan")
	if !strings.Contains(url, "cast+iron+pan") {
		t.Errorf("expected + encoding for spaces, got: %s", url)
	}
	if !strings.Contains(url, "mytag-20") {
		t.Errorf("expected tag in URL, got: %s", url)
	}
}

func TestGenerateLink_SpecialChars(t *testing.T) {
	p := &Provider{
		Name:        "amazon",
		URLTemplate: "https://amazon.com/s?k={{term}}",
		Tag:         "",
	}
	url := p.GenerateLink("bread & butter")
	if url == "" {
		t.Error("expected non-empty URL")
	}
}

// ── NewRegistry ───────────────────────────────────────────────────────────────

func TestNewRegistry_SkipsNoTag(t *testing.T) {
	cfg := config.AffiliatesConfig{
		Providers: []config.AffiliateProviderConfig{
			{Name: "amazon", URLTemplate: "...", EnvVar: "SUPERMODEL_TEST_NONEXISTENT_VAR_XYZ"},
		},
	}
	r := NewRegistry(cfg)
	if len(r.Providers) != 0 {
		t.Errorf("expected 0 providers (no env var set), got %d", len(r.Providers))
	}
}

func TestNewRegistry_AlwaysInclude(t *testing.T) {
	cfg := config.AffiliatesConfig{
		Providers: []config.AffiliateProviderConfig{
			{Name: "amazon", URLTemplate: "...", AlwaysInclude: true},
		},
	}
	r := NewRegistry(cfg)
	if len(r.Providers) != 1 {
		t.Errorf("expected 1 provider (always include), got %d", len(r.Providers))
	}
}

func TestNewRegistry_WithEnvVar(t *testing.T) {
	t.Setenv("SUPERMODEL_TEST_TAG_VAR", "testtag-20")
	cfg := config.AffiliatesConfig{
		Providers: []config.AffiliateProviderConfig{
			{Name: "amazon", URLTemplate: "https://example.com?tag={{tag}}", EnvVar: "SUPERMODEL_TEST_TAG_VAR"},
		},
	}
	r := NewRegistry(cfg)
	if len(r.Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(r.Providers))
	}
	if r.Providers[0].Tag != "testtag-20" {
		t.Errorf("expected tag 'testtag-20', got %q", r.Providers[0].Tag)
	}
}

func TestNewRegistry_EmptyProviders(t *testing.T) {
	r := NewRegistry(config.AffiliatesConfig{})
	if len(r.Providers) != 0 {
		t.Errorf("expected 0 providers, got %d", len(r.Providers))
	}
}

// ── Registry.GenerateLinks ────────────────────────────────────────────────────

func TestGenerateLinks_NoProviders(t *testing.T) {
	r := &Registry{}
	links := r.GenerateLinks(map[string]interface{}{"term": "flour"}, []string{"term"})
	if links != nil {
		t.Errorf("expected nil with no providers, got %v", links)
	}
}

func TestGenerateLinks_NilEnrichment(t *testing.T) {
	r := &Registry{Providers: []Provider{{Name: "amazon", URLTemplate: "..."}}}
	links := r.GenerateLinks(nil, []string{"term"})
	if links != nil {
		t.Errorf("expected nil with nil enrichment, got %v", links)
	}
}

func TestGenerateLinks_SimpleField(t *testing.T) {
	r := &Registry{Providers: []Provider{
		{Name: "amazon", URLTemplate: "https://example.com?k={{term}}", Tag: "tag"},
	}}
	data := map[string]interface{}{"term": "flour"}
	links := r.GenerateLinks(data, []string{"term"})
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].Term != "flour" {
		t.Errorf("expected term 'flour', got %q", links[0].Term)
	}
}

func TestGenerateLinks_ArrayField(t *testing.T) {
	r := &Registry{Providers: []Provider{
		{Name: "amazon", URLTemplate: "https://example.com?k={{term}}"},
	}}
	data := map[string]interface{}{
		"ingredients": []interface{}{
			map[string]interface{}{"searchTerm": "flour"},
			map[string]interface{}{"searchTerm": "sugar"},
		},
	}
	links := r.GenerateLinks(data, []string{"ingredients[].searchTerm"})
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
}

// ── extractTerms ──────────────────────────────────────────────────────────────

func TestExtractTerms_SimpleField(t *testing.T) {
	data := map[string]interface{}{"keyword": "cast iron"}
	terms := extractTerms(data, "keyword")
	if len(terms) != 1 || terms[0] != "cast iron" {
		t.Errorf("got %v", terms)
	}
}

func TestExtractTerms_SimpleFieldMissing(t *testing.T) {
	terms := extractTerms(map[string]interface{}{}, "keyword")
	if len(terms) != 0 {
		t.Errorf("missing key: got %v", terms)
	}
}

func TestExtractTerms_SimpleFieldNonString(t *testing.T) {
	data := map[string]interface{}{"count": 42}
	terms := extractTerms(data, "count")
	if len(terms) != 0 {
		t.Errorf("non-string: got %v", terms)
	}
}

func TestExtractTerms_ArrayPath(t *testing.T) {
	data := map[string]interface{}{
		"ingredients": []interface{}{
			map[string]interface{}{"searchTerm": "flour"},
			map[string]interface{}{"searchTerm": ""},      // empty term skipped
			map[string]interface{}{"other": "no term"},    // missing field skipped
			"not a map",                                   // non-map skipped
		},
	}
	terms := extractTerms(data, "ingredients[].searchTerm")
	if len(terms) != 1 || terms[0] != "flour" {
		t.Errorf("expected ['flour'], got %v", terms)
	}
}

func TestExtractTerms_ArrayFieldMissing(t *testing.T) {
	terms := extractTerms(map[string]interface{}{}, "gear[].searchTerm")
	if len(terms) != 0 {
		t.Errorf("missing array field: got %v", terms)
	}
}

func TestExtractTerms_ArrayFieldNotSlice(t *testing.T) {
	data := map[string]interface{}{"ingredients": "not a slice"}
	terms := extractTerms(data, "ingredients[].searchTerm")
	if len(terms) != 0 {
		t.Errorf("non-slice: got %v", terms)
	}
}
