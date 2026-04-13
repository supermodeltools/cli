package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
)

// ── splitFrontmatter ──────────────────────────────────────────────────────────

func TestSplitFrontmatter_NoFrontmatter(t *testing.T) {
	fm, body, err := splitFrontmatter("just body text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm != "" {
		t.Errorf("expected empty frontmatter, got %q", fm)
	}
	if body != "just body text" {
		t.Errorf("expected body %q, got %q", "just body text", body)
	}
}

func TestSplitFrontmatter_WithFrontmatter(t *testing.T) {
	content := "---\ntitle: Test\n---\nbody here"
	fm, body, err := splitFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm != "title: Test" {
		t.Errorf("frontmatter mismatch: got %q", fm)
	}
	if body != "body here" {
		t.Errorf("body mismatch: got %q", body)
	}
}

func TestSplitFrontmatter_NoClosingDashes(t *testing.T) {
	content := "---\ntitle: Test\nno closing"
	_, _, err := splitFrontmatter(content)
	if err == nil {
		t.Error("expected error for missing closing ---")
	}
}

func TestSplitFrontmatter_EmptyBody(t *testing.T) {
	content := "---\ntitle: Test\n---"
	fm, body, err := splitFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm != "title: Test" {
		t.Errorf("frontmatter: got %q", fm)
	}
	if body != "" {
		t.Errorf("empty body expected, got %q", body)
	}
}

// ── extractSection ────────────────────────────────────────────────────────────

func TestExtractSection_Found(t *testing.T) {
	body := "## Ingredients\n- flour\n- sugar\n## Instructions\nMix well."
	got := extractSection(body, "Ingredients")
	if got != "- flour\n- sugar" {
		t.Errorf("got %q", got)
	}
}

func TestExtractSection_NotFound(t *testing.T) {
	body := "## Instructions\nDo this."
	got := extractSection(body, "Ingredients")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractSection_LastSection(t *testing.T) {
	body := "## Instructions\nDo this.\nAnd that."
	got := extractSection(body, "Instructions")
	if got != "Do this.\nAnd that." {
		t.Errorf("got %q", got)
	}
}

func TestExtractSection_HeadingWithNoNewline(t *testing.T) {
	// No newline after heading → extractSection returns ""
	body := "## Ingredients"
	got := extractSection(body, "Ingredients")
	if got != "" {
		t.Errorf("expected empty for heading without newline, got %q", got)
	}
}

// ── parseUnorderedList ────────────────────────────────────────────────────────

func TestParseUnorderedList_DashItems(t *testing.T) {
	items := parseUnorderedList("- flour\n- sugar\n- butter")
	if len(items) != 3 || items[0] != "flour" || items[2] != "butter" {
		t.Errorf("unexpected items: %v", items)
	}
}

func TestParseUnorderedList_StarItems(t *testing.T) {
	items := parseUnorderedList("* one\n* two")
	if len(items) != 2 || items[0] != "one" {
		t.Errorf("unexpected items: %v", items)
	}
}

func TestParseUnorderedList_Mixed(t *testing.T) {
	items := parseUnorderedList("- a\n* b\nplain line")
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d: %v", len(items), items)
	}
}

func TestParseUnorderedList_Empty(t *testing.T) {
	items := parseUnorderedList("")
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %v", items)
	}
}

// ── parseOrderedList ──────────────────────────────────────────────────────────

func TestParseOrderedList_Basic(t *testing.T) {
	items := parseOrderedList("1. First\n2. Second\n3. Third")
	if len(items) != 3 || items[0] != "First" || items[2] != "Third" {
		t.Errorf("unexpected items: %v", items)
	}
}

func TestParseOrderedList_SkipsNonNumeric(t *testing.T) {
	items := parseOrderedList("a. Not an item\n1. Real item")
	if len(items) != 1 || items[0] != "Real item" {
		t.Errorf("unexpected items: %v", items)
	}
}

func TestParseOrderedList_ShortLine(t *testing.T) {
	// Line < 3 chars → skipped
	items := parseOrderedList("1.\n2. Item")
	if len(items) != 1 || items[0] != "Item" {
		t.Errorf("unexpected items: %v", items)
	}
}

func TestParseOrderedList_Empty(t *testing.T) {
	items := parseOrderedList("")
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %v", items)
	}
}

// ── parseFAQs ─────────────────────────────────────────────────────────────────

func TestParseFAQs_Basic(t *testing.T) {
	content := "### What is it?\nIt is a thing.\n\n### How does it work?\nMagically."
	faqs := parseFAQs(content)
	if len(faqs) != 2 {
		t.Fatalf("expected 2 FAQs, got %d", len(faqs))
	}
	if faqs[0].Question != "What is it?" {
		t.Errorf("q0: got %q", faqs[0].Question)
	}
	if faqs[0].Answer != "It is a thing." {
		t.Errorf("a0: got %q", faqs[0].Answer)
	}
}

func TestParseFAQs_Empty(t *testing.T) {
	faqs := parseFAQs("")
	if len(faqs) != 0 {
		t.Errorf("expected empty FAQs, got %v", faqs)
	}
}

func TestParseFAQs_QuestionOnly(t *testing.T) {
	content := "### Why?"
	faqs := parseFAQs(content)
	if len(faqs) != 1 || faqs[0].Question != "Why?" {
		t.Errorf("unexpected FAQs: %v", faqs)
	}
	if faqs[0].Answer != "" {
		t.Errorf("expected empty answer, got %q", faqs[0].Answer)
	}
}

// ── deriveSlug ────────────────────────────────────────────────────────────────

func TestDeriveSlug_FromField(t *testing.T) {
	l := &MarkdownLoader{Config: &config.Config{
		Data: config.DataConfig{EntitySlug: config.EntitySlug{Source: "field:title"}},
	}}
	fields := map[string]interface{}{"title": "My Recipe!"}
	slug := l.deriveSlug("/data/my-recipe.md", fields)
	if slug == "" {
		t.Error("expected non-empty slug from field")
	}
}

func TestDeriveSlug_FromFieldNonString(t *testing.T) {
	l := &MarkdownLoader{Config: &config.Config{
		Data: config.DataConfig{EntitySlug: config.EntitySlug{Source: "field:count"}},
	}}
	fields := map[string]interface{}{"count": 42}
	// Non-string field → fall through to filename
	slug := l.deriveSlug("/data/my-file.md", fields)
	if slug != "my-file" {
		t.Errorf("expected 'my-file', got %q", slug)
	}
}

func TestDeriveSlug_FromFilename(t *testing.T) {
	l := &MarkdownLoader{Config: &config.Config{}}
	fields := map[string]interface{}{}
	slug := l.deriveSlug("/data/chocolate-cake.md", fields)
	if slug != "chocolate-cake" {
		t.Errorf("expected 'chocolate-cake', got %q", slug)
	}
}

// ── MarkdownLoader.Load ───────────────────────────────────────────────────────

func TestLoad_ValidMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "recipe.md"), []byte("---\ntitle: Cake\n---\nBody here."), 0600); err != nil {
		t.Fatal(err)
	}

	l := &MarkdownLoader{Config: &config.Config{
		Paths: config.PathsConfig{Data: dir},
	}}
	entities, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entities) != 1 {
		t.Errorf("expected 1 entity, got %d", len(entities))
	}
}

func TestLoad_SkipsNonMdFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"a":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "recipe.md"), []byte("---\ntitle: Cake\n---"), 0600); err != nil {
		t.Fatal(err)
	}

	l := &MarkdownLoader{Config: &config.Config{
		Paths: config.PathsConfig{Data: dir},
	}}
	entities, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entities) != 1 {
		t.Errorf("expected 1 entity (only .md), got %d", len(entities))
	}
}

func TestLoad_SkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "inner.md"), []byte("---\ntitle: Inner\n---"), 0600); err != nil {
		t.Fatal(err)
	}

	l := &MarkdownLoader{Config: &config.Config{
		Paths: config.PathsConfig{Data: dir},
	}}
	entities, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("expected 0 entities (subdir skipped), got %d", len(entities))
	}
}

func TestLoad_DataDirNotExist(t *testing.T) {
	l := &MarkdownLoader{Config: &config.Config{
		Paths: config.PathsConfig{Data: "/nonexistent-dir-xyz"},
	}}
	_, err := l.Load()
	if err == nil {
		t.Error("expected error for non-existent data dir")
	}
}

func TestLoad_SkipsUnparseableFiles(t *testing.T) {
	dir := t.TempDir()
	// Invalid frontmatter (no closing ---)
	if err := os.WriteFile(filepath.Join(dir, "bad.md"), []byte("---\ntitle: Bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "good.md"), []byte("---\ntitle: Good\n---\nBody"), 0600); err != nil {
		t.Fatal(err)
	}

	l := &MarkdownLoader{Config: &config.Config{
		Paths: config.PathsConfig{Data: dir},
	}}
	entities, err := l.Load()
	if err != nil {
		t.Fatalf("Load should not fail on parse errors: %v", err)
	}
	if len(entities) != 1 {
		t.Errorf("expected 1 entity (bad.md skipped), got %d", len(entities))
	}
}

func TestLoad_UnreadableFile(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "locked.md")
	if err := os.WriteFile(f, []byte("---\ntitle: T\n---"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(f, 0600) }) //nolint:errcheck

	l := &MarkdownLoader{Config: &config.Config{
		Paths: config.PathsConfig{Data: dir},
	}}
	// Unreadable file is skipped with a warning (Load returns remaining entities)
	entities, err := l.Load()
	if err != nil {
		t.Fatalf("Load should not fail on unreadable file (warn+skip): %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("expected 0 entities (unreadable file skipped), got %d", len(entities))
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	// Frontmatter references an undefined anchor → yaml.Unmarshal error
	if err := os.WriteFile(filepath.Join(dir, "bad-yaml.md"), []byte("---\nfield: *undefined_anchor\n---\nBody"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "good.md"), []byte("---\ntitle: Good\n---\nBody"), 0600); err != nil {
		t.Fatal(err)
	}

	l := &MarkdownLoader{Config: &config.Config{
		Paths: config.PathsConfig{Data: dir},
	}}
	entities, err := l.Load()
	if err != nil {
		t.Fatalf("Load should not fail on YAML parse errors (warn+skip): %v", err)
	}
	if len(entities) != 1 {
		t.Errorf("expected 1 entity (bad-yaml.md skipped), got %d", len(entities))
	}
}

// ── New ───────────────────────────────────────────────────────────────────────

func TestNew_MarkdownFormat(t *testing.T) {
	l := New(&config.Config{Data: config.DataConfig{Format: "markdown"}})
	if _, ok := l.(*MarkdownLoader); !ok {
		t.Error("expected *MarkdownLoader for markdown format")
	}
}

func TestNew_DefaultFormat(t *testing.T) {
	l := New(&config.Config{Data: config.DataConfig{Format: "unknown"}})
	if _, ok := l.(*MarkdownLoader); !ok {
		t.Error("expected *MarkdownLoader for unknown format")
	}
}

// ── parseSections (body sections) ────────────────────────────────────────────

func TestParseSections_AllTypes(t *testing.T) {
	body := "## Ingredients\n- flour\n- sugar\n## Steps\n1. Mix\n2. Bake\n## FAQs\n### What temp?\n350°F\n## Notes\nExtra notes."

	l := &MarkdownLoader{Config: &config.Config{
		Data: config.DataConfig{
			BodySections: []config.BodySection{
				{Header: "Ingredients", Name: "ingredients", Type: "unordered_list"},
				{Header: "Steps", Name: "steps", Type: "ordered_list"},
				{Header: "FAQs", Name: "faqs", Type: "faq"},
				{Header: "Notes", Name: "notes", Type: "markdown"},
			},
		},
	}}
	sections := l.parseSections(body)

	if items, ok := sections["ingredients"].([]string); !ok || len(items) != 2 {
		t.Errorf("ingredients: expected []string with 2 items, got %v", sections["ingredients"])
	}
	if items, ok := sections["steps"].([]string); !ok || len(items) != 2 {
		t.Errorf("steps: expected []string with 2 items, got %v", sections["steps"])
	}
	if notes, ok := sections["notes"].(string); !ok || notes == "" {
		t.Errorf("notes: expected non-empty string, got %v", sections["notes"])
	}
}

func TestParseSections_DefaultType(t *testing.T) {
	body := "## Tips\nSome tips here."
	l := &MarkdownLoader{Config: &config.Config{
		Data: config.DataConfig{
			BodySections: []config.BodySection{
				{Header: "Tips", Name: "tips", Type: "other"},
			},
		},
	}}
	sections := l.parseSections(body)
	if v, ok := sections["tips"].(string); !ok || v == "" {
		t.Errorf("default type: expected string, got %v", sections["tips"])
	}
}

func TestParseSections_MissingSectionSkipped(t *testing.T) {
	body := "## Instructions\nDo this."
	l := &MarkdownLoader{Config: &config.Config{
		Data: config.DataConfig{
			BodySections: []config.BodySection{
				{Header: "Ingredients", Name: "ingredients", Type: "unordered_list"},
			},
		},
	}}
	sections := l.parseSections(body)
	if _, ok := sections["ingredients"]; ok {
		t.Error("missing section should not appear in result")
	}
}
