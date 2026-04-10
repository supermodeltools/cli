package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/render"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/taxonomy"
)

func newBuilder(outDir string) *Builder {
	return NewBuilder(&config.Config{
		Search: config.SearchConfig{Enabled: true},
		Paths:  config.PathsConfig{Output: outDir},
	}, false)
}

func makeEntity(slug, title, description string) *entity.Entity {
	return &entity.Entity{
		Slug: slug,
		Fields: map[string]interface{}{
			"title":       title,
			"description": description,
		},
	}
}

// TestGenerateSearchIndex_ShortDescription verifies that descriptions under
// the 120-rune limit are written verbatim.
func TestGenerateSearchIndex_ShortDescription(t *testing.T) {
	outDir := t.TempDir()
	b := newBuilder(outDir)

	ent := makeEntity("test-slug", "Test Title", "Short description.")
	if err := b.generateSearchIndex([]*entity.Entity{ent}, outDir); err != nil {
		t.Fatalf("generateSearchIndex: %v", err)
	}

	entries := readSearchIndex(t, outDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0]["d"] != "Short description." {
		t.Errorf("description mismatch: got %q", entries[0]["d"])
	}
}

// TestGenerateSearchIndex_LongASCIIDescription verifies ASCII-only descriptions
// longer than 120 chars are truncated to exactly 120 runes.
func TestGenerateSearchIndex_LongASCIIDescription(t *testing.T) {
	outDir := t.TempDir()
	b := newBuilder(outDir)

	// build a 200-char ASCII string
	long := ""
	for i := 0; i < 200; i++ {
		long += "a"
	}

	ent := makeEntity("slug", "Title", long)
	if err := b.generateSearchIndex([]*entity.Entity{ent}, outDir); err != nil {
		t.Fatalf("generateSearchIndex: %v", err)
	}

	entries := readSearchIndex(t, outDir)
	got := entries[0]["d"]
	if len([]rune(got)) != 120 {
		t.Errorf("expected 120 runes, got %d", len([]rune(got)))
	}
}

// TestGenerateSearchIndex_MultiByteDescriptionTruncation is the regression test
// for the byte-vs-rune truncation bug. A description whose byte length exceeds
// 120 but whose rune count does not must NOT be truncated. A description whose
// rune count exceeds 120 must be truncated at a rune boundary so the result
// is valid UTF-8.
func TestGenerateSearchIndex_MultiByteDescriptionTruncation(t *testing.T) {
	outDir := t.TempDir()
	b := newBuilder(outDir)

	// Each 'é' is 2 bytes (U+00E9). We build a string of 121 'é' characters:
	// rune length = 121 (> 120) so it must be truncated to 120 runes.
	// byte length = 242, so the old code would have produced a split in the
	// middle of a multi-byte sequence → invalid UTF-8.
	longMultiByte := ""
	for i := 0; i < 121; i++ {
		longMultiByte += "é"
	}

	ent := makeEntity("slug", "Title", longMultiByte)
	if err := b.generateSearchIndex([]*entity.Entity{ent}, outDir); err != nil {
		t.Fatalf("generateSearchIndex: %v", err)
	}

	entries := readSearchIndex(t, outDir)
	got := entries[0]["d"]

	if !utf8.ValidString(got) {
		t.Errorf("truncated description is not valid UTF-8: %q", got)
	}
	if runes := []rune(got); len(runes) != 120 {
		t.Errorf("expected 120 runes after truncation, got %d", len(runes))
	}
}

// TestGenerateSearchIndex_DisabledSearch verifies no file is written when search
// is disabled.
func TestGenerateSearchIndex_DisabledSearch(t *testing.T) {
	outDir := t.TempDir()
	b := NewBuilder(&config.Config{
		Search: config.SearchConfig{Enabled: false},
		Paths:  config.PathsConfig{Output: outDir},
	}, false)

	ent := makeEntity("slug", "Title", "desc")
	if err := b.generateSearchIndex([]*entity.Entity{ent}, outDir); err != nil {
		t.Fatalf("generateSearchIndex: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "search-index.json")); !os.IsNotExist(err) {
		t.Error("search-index.json should not be written when search is disabled")
	}
}

// ── shareImageURL ─────────────────────────────────────────────────────────────

func TestShareImageURL(t *testing.T) {
	got := shareImageURL("https://example.com", "recipe-soup.png")
	want := "https://example.com/images/share/recipe-soup.png"
	if got != want {
		t.Errorf("shareImageURL: got %q, want %q", got, want)
	}
}

// ── countTaxEntries ───────────────────────────────────────────────────────────

func TestCountTaxEntries(t *testing.T) {
	taxes := []taxonomy.Taxonomy{
		{Entries: []taxonomy.Entry{{}, {}}},
		{Entries: []taxonomy.Entry{{}}},
	}
	if got := countTaxEntries(taxes); got != 3 {
		t.Errorf("countTaxEntries: got %d, want 3", got)
	}
	if got := countTaxEntries(nil); got != 0 {
		t.Errorf("countTaxEntries(nil): got %d, want 0", got)
	}
}

// ── countFieldDistribution ────────────────────────────────────────────────────

func TestCountFieldDistribution(t *testing.T) {
	entities := []*entity.Entity{
		{Fields: map[string]interface{}{"cuisine": "Italian"}},
		{Fields: map[string]interface{}{"cuisine": "Italian"}},
		{Fields: map[string]interface{}{"cuisine": "French"}},
		{Fields: map[string]interface{}{"cuisine": ""}}, // empty, should be skipped
	}
	result := countFieldDistribution(entities, "cuisine", 10)
	if len(result) != 2 {
		t.Fatalf("want 2 entries, got %d", len(result))
	}
	// Should be sorted desc by count
	if result[0].Name != "Italian" || result[0].Count != 2 {
		t.Errorf("first entry: got {%s %d}, want {Italian 2}", result[0].Name, result[0].Count)
	}
	if result[1].Name != "French" || result[1].Count != 1 {
		t.Errorf("second entry: got {%s %d}, want {French 1}", result[1].Name, result[1].Count)
	}
}

func TestCountFieldDistribution_Limit(t *testing.T) {
	entities := []*entity.Entity{
		{Fields: map[string]interface{}{"tag": "a"}},
		{Fields: map[string]interface{}{"tag": "a"}},
		{Fields: map[string]interface{}{"tag": "b"}},
		{Fields: map[string]interface{}{"tag": "b"}},
		{Fields: map[string]interface{}{"tag": "c"}},
	}
	result := countFieldDistribution(entities, "tag", 2)
	if len(result) != 2 {
		t.Errorf("limit=2: want 2 entries, got %d", len(result))
	}
}

func TestCountFieldDistribution_Empty(t *testing.T) {
	if got := countFieldDistribution(nil, "field", 10); len(got) != 0 {
		t.Errorf("nil entities: want empty, got %v", got)
	}
}

// ── toBreadcrumbItems ─────────────────────────────────────────────────────────

func TestToBreadcrumbItems(t *testing.T) {
	bcs := []render.Breadcrumb{
		{Name: "Home", URL: "https://example.com/"},
		{Name: "Recipes", URL: "https://example.com/recipes/"},
	}
	items := toBreadcrumbItems(bcs)
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Name != "Home" || items[0].URL != "https://example.com/" {
		t.Errorf("first item: got %+v", items[0])
	}
	if items[1].Name != "Recipes" {
		t.Errorf("second item: got %+v", items[1])
	}
}

// readSearchIndex reads and unmarshals the search-index.json from outDir.
func readSearchIndex(t *testing.T, outDir string) []map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(outDir, "search-index.json"))
	if err != nil {
		t.Fatalf("reading search-index.json: %v", err)
	}
	var entries []map[string]string
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("unmarshaling search-index.json: %v", err)
	}
	return entries
}
