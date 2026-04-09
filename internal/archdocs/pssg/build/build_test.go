package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
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
