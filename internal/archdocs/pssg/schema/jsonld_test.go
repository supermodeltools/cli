package schema

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
)

func TestStepName(t *testing.T) {
	// ASCII truncation: step longer than 80 bytes, no sentence break.
	long := ""
	for i := 0; i < 85; i++ {
		long += "a"
	}
	got := stepName(long)
	if len([]rune(got)) != 80 { // 77 + len("...") = 80
		t.Errorf("ASCII truncation: got %d runes, want 80", len([]rune(got)))
	}

	// Short sentence extraction.
	got = stepName("Mix ingredients. Then bake for 30 minutes.")
	if got != "Mix ingredients." {
		t.Errorf("short sentence: got %q, want %q", got, "Mix ingredients.")
	}

	// Multi-byte truncation: 85 'é' chars (2 bytes each), no period.
	// Byte length > 80 but we must truncate at rune boundary.
	multiLong := ""
	for i := 0; i < 85; i++ {
		multiLong += "é"
	}
	got = stepName(multiLong)
	if !utf8.ValidString(got) {
		t.Errorf("multi-byte truncation produced invalid UTF-8: %q", got)
	}
	if len([]rune(got)) != 80 { // 77 runes + "..."
		t.Errorf("multi-byte truncation: got %d runes, want 80", len([]rune(got)))
	}

	// Multi-byte sentence: 'é' × 79 chars followed by ". rest"
	// Sentence rune count = 80 (79 é + 1 period), which is NOT < 80, so falls through.
	// Resulting truncation: 85-char total → truncate to 77+...
	multiSentence := ""
	for i := 0; i < 79; i++ {
		multiSentence += "é"
	}
	multiSentence += ". rest of step"
	got = stepName(multiSentence)
	if !utf8.ValidString(got) {
		t.Errorf("multi-byte sentence truncation produced invalid UTF-8: %q", got)
	}
}

// ── NewGenerator ──────────────────────────────────────────────────────────────

func TestNewGenerator(t *testing.T) {
	site := config.SiteConfig{Name: "My Site", BaseURL: "https://example.com"}
	g := NewGenerator(site, config.SchemaConfig{})
	if g == nil {
		t.Fatal("NewGenerator returned nil")
	}
	if g.SiteConfig.Name != "My Site" {
		t.Errorf("SiteConfig.Name: got %q", g.SiteConfig.Name)
	}
}

// ── GenerateWebSiteSchema ─────────────────────────────────────────────────────

func TestGenerateWebSiteSchema(t *testing.T) {
	g := NewGenerator(config.SiteConfig{
		Name:        "My Site",
		BaseURL:     "https://example.com",
		Description: "A recipe site",
	}, config.SchemaConfig{})

	s := g.GenerateWebSiteSchema("")
	if s["@type"] != "WebSite" {
		t.Errorf("@type: got %v", s["@type"])
	}
	if s["name"] != "My Site" {
		t.Errorf("name: got %v", s["name"])
	}
	if s["url"] != "https://example.com" {
		t.Errorf("url: got %v", s["url"])
	}
	if _, ok := s["image"]; ok {
		t.Error("image should not be set when imageURL is empty")
	}

	// With image
	s2 := g.GenerateWebSiteSchema("https://example.com/og.png")
	if s2["image"] != "https://example.com/og.png" {
		t.Errorf("image: got %v", s2["image"])
	}
}

// ── GenerateBreadcrumbSchema ──────────────────────────────────────────────────

func TestGenerateBreadcrumbSchema(t *testing.T) {
	g := NewGenerator(config.SiteConfig{}, config.SchemaConfig{})
	items := []BreadcrumbItem{
		{Name: "Home", URL: "https://example.com/"},
		{Name: "Recipes", URL: "https://example.com/recipes/"},
		{Name: "Soup"},
	}
	s := g.GenerateBreadcrumbSchema(items)
	if s["@type"] != "BreadcrumbList" {
		t.Errorf("@type: got %v", s["@type"])
	}
	list := s["itemListElement"].([]map[string]interface{})
	if len(list) != 3 {
		t.Fatalf("want 3 items, got %d", len(list))
	}
	if list[0]["position"] != 1 {
		t.Errorf("position 1: got %v", list[0]["position"])
	}
	if list[0]["item"] != "https://example.com/" {
		t.Errorf("item[0].item: got %v", list[0]["item"])
	}
	// Last item has no URL, so "item" key should not be present
	if _, ok := list[2]["item"]; ok {
		t.Error("item without URL should not have 'item' key")
	}
}

// ── GenerateFAQSchema ─────────────────────────────────────────────────────────

func TestGenerateFAQSchema_WithFAQs(t *testing.T) {
	g := NewGenerator(config.SiteConfig{}, config.SchemaConfig{})
	faqs := []entity.FAQ{
		{Question: "How long does this take?", Answer: "30 minutes."},
		{Question: "Can I freeze it?", Answer: "Yes!"},
	}
	s := g.GenerateFAQSchema(faqs)
	if s["@type"] != "FAQPage" {
		t.Errorf("@type: got %v", s["@type"])
	}
	qs := s["mainEntity"].([]map[string]interface{})
	if len(qs) != 2 {
		t.Fatalf("want 2 FAQs, got %d", len(qs))
	}
	if qs[0]["name"] != "How long does this take?" {
		t.Errorf("first question: got %v", qs[0]["name"])
	}
}

func TestGenerateFAQSchema_Empty(t *testing.T) {
	g := NewGenerator(config.SiteConfig{}, config.SchemaConfig{})
	if got := g.GenerateFAQSchema(nil); got != nil {
		t.Error("empty FAQs should return nil")
	}
}

// ── GenerateItemListSchema ────────────────────────────────────────────────────

func TestGenerateItemListSchema(t *testing.T) {
	g := NewGenerator(config.SiteConfig{}, config.SchemaConfig{})
	items := []ItemListEntry{
		{Name: "Tomato Soup", URL: "https://example.com/tomato-soup"},
		{Name: "Chicken Stew", URL: "https://example.com/chicken-stew"},
	}
	s := g.GenerateItemListSchema("Soups", "A collection of soups", items, "")
	if s["@type"] != "ItemList" {
		t.Errorf("@type: got %v", s["@type"])
	}
	if s["numberOfItems"] != 2 {
		t.Errorf("numberOfItems: got %v", s["numberOfItems"])
	}
	list := s["itemListElement"].([]map[string]interface{})
	if len(list) != 2 {
		t.Fatalf("want 2 items, got %d", len(list))
	}
	if list[0]["position"] != 1 {
		t.Errorf("position: got %v", list[0]["position"])
	}
}

// ── MarshalSchemas ────────────────────────────────────────────────────────────

func TestMarshalSchemas_Basic(t *testing.T) {
	s := map[string]interface{}{"@type": "WebSite", "name": "Test"}
	got := MarshalSchemas(s)
	if !strings.HasPrefix(got, `<script type="application/ld+json">`) {
		t.Errorf("should start with script tag, got: %q", got[:50])
	}
	if !strings.Contains(got, `"@type":"WebSite"`) {
		t.Errorf("should contain @type, got: %q", got)
	}
}

func TestMarshalSchemas_NilSkipped(t *testing.T) {
	s := map[string]interface{}{"@type": "WebSite"}
	got := MarshalSchemas(nil, s, nil)
	if strings.Count(got, "<script") != 1 {
		t.Errorf("nil schemas should be skipped, got %d script tags", strings.Count(got, "<script"))
	}
}

func TestMarshalSchemas_Multiple(t *testing.T) {
	s1 := map[string]interface{}{"@type": "WebSite"}
	s2 := map[string]interface{}{"@type": "BreadcrumbList"}
	got := MarshalSchemas(s1, s2)
	if strings.Count(got, "<script") != 2 {
		t.Errorf("want 2 script tags, got %d", strings.Count(got, "<script"))
	}
}

func TestMarshalSchemas_Empty(t *testing.T) {
	if got := MarshalSchemas(); got != "" {
		t.Errorf("no schemas: want empty string, got %q", got)
	}
}

func TestParseDurationMinutes(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"PT30M", 30},
		{"PT1H", 60},
		{"PT1H30M", 90},
		{"PT90S", 1},        // 90 seconds = 1 minute
		{"PT30S", 0},        // 30 seconds rounds down to 0 minutes
		{"PT2H30M45S", 150}, // 2h + 30m + 45s → 150m (45s/60 = 0 extra)
		{"PT2H30M90S", 151}, // 2h + 30m + 90s → 151m (90s/60 = 1 extra)
		{"PT15M90S", 16},    // 15m + 90s = 16m — this was 15 before the fix
		{"PT0S", 0},
		{"", 0},
		{"invalid", 0},
	}
	for _, c := range cases {
		got := parseDurationMinutes(c.input)
		if got != c.want {
			t.Errorf("parseDurationMinutes(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestComputeTotalTime(t *testing.T) {
	cases := []struct {
		d1, d2 string
		want   string
	}{
		{"PT15M", "PT30M", "PT45M"},
		{"PT1H", "PT30M", "PT1H30M"},
		{"PT30M", "PT30M", "PT1H"},
		{"PT15M90S", "PT30M", "PT46M"}, // 16m + 30m = 46m
		{"PT0S", "PT1H", "PT1H"},
	}
	for _, c := range cases {
		got := computeTotalTime(c.d1, c.d2)
		if got != c.want {
			t.Errorf("computeTotalTime(%q, %q) = %q, want %q", c.d1, c.d2, got, c.want)
		}
	}
}
