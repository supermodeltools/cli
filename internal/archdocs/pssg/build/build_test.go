package build

import (
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
	"strings"
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

// ── toTemplateHTML ────────────────────────────────────────────────────────────

func TestToTemplateHTML(t *testing.T) {
	input := "<strong>hello &amp; world</strong>"
	got := toTemplateHTML(input)
	if got != template.HTML(input) {
		t.Errorf("toTemplateHTML: got %q, want %q", got, input)
	}
}

// ── writeShareSVG ─────────────────────────────────────────────────────────────

func TestWriteShareSVG(t *testing.T) {
	outDir := t.TempDir()
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`
	if err := writeShareSVG(outDir, "test.svg", svg); err != nil {
		t.Fatalf("writeShareSVG: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outDir, "images", "share", "test.svg"))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if !strings.Contains(string(data), "<svg") {
		t.Error("written file should contain SVG content")
	}
}

// ── maybeWriteShareSVG ────────────────────────────────────────────────────────

func TestMaybeWriteShareSVG_Disabled(t *testing.T) {
	outDir := t.TempDir()
	b := NewBuilder(&config.Config{
		Output: config.OutputConfig{ShareImages: false},
		Paths:  config.PathsConfig{Output: outDir},
	}, false)
	if err := b.maybeWriteShareSVG(outDir, "test.svg", "<svg/>"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// File should NOT be written when ShareImages=false.
	if _, err := os.Stat(filepath.Join(outDir, "images", "share", "test.svg")); !os.IsNotExist(err) {
		t.Error("share image should not be written when ShareImages=false")
	}
}

func TestMaybeWriteShareSVG_Enabled(t *testing.T) {
	outDir := t.TempDir()
	b := NewBuilder(&config.Config{
		Output: config.OutputConfig{ShareImages: true},
		Paths:  config.PathsConfig{Output: outDir},
	}, false)
	if err := b.maybeWriteShareSVG(outDir, "test.svg", "<svg/>"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "images", "share", "test.svg")); err != nil {
		t.Errorf("share image should be written when ShareImages=true: %v", err)
	}
}

// TestWriteShareSVG_MkdirAllError covers L1310: writeShareSVG returns an error
// when MkdirAll fails because a file exists at the parent path.
func TestWriteShareSVG_MkdirAllError(t *testing.T) {
	outDir := t.TempDir()
	// Place a regular file at the "images" subdirectory so MkdirAll("images/share") fails.
	if err := os.WriteFile(filepath.Join(outDir, "images"), []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	err := writeShareSVG(outDir, "test.svg", "<svg/>")
	if err == nil {
		t.Error("expected MkdirAll error when parent path is a file")
	}
}

// TestGenerateSearchIndex_WriteFileError covers L1365: generateSearchIndex returns
// an error when os.WriteFile fails because the output directory is not writable.
func TestGenerateSearchIndex_WriteFileError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	outDir := t.TempDir()
	if err := os.Chmod(outDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(outDir, 0755) }) //nolint:errcheck

	ent := &entity.Entity{Slug: "test-recipe", Fields: map[string]interface{}{"title": "Test"}}
	b := NewBuilder(&config.Config{Search: config.SearchConfig{Enabled: true}}, false)
	err := b.generateSearchIndex([]*entity.Entity{ent}, outDir)
	if err == nil {
		t.Error("expected WriteFile error when outDir is not writable")
	}
}

// ── copyDir ───────────────────────────────────────────────────────────────────

func TestCopyDir_CopiesFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatalf("copied file not found: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content mismatch: got %q, want %q", data, "hello")
	}
}

func TestCopyDir_CopiesSubdirs(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	sub := filepath.Join(src, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("world"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir with subdir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	if err != nil {
		t.Fatalf("copied subdir file not found: %v", err)
	}
	if string(data) != "world" {
		t.Errorf("content mismatch: got %q", data)
	}
}

func TestCopyDir_NonExistentSrc(t *testing.T) {
	dst := t.TempDir()
	// Non-existent src → IsNotExist → returns nil (no-op)
	if err := copyDir(filepath.Join(t.TempDir(), "nonexistent"), dst); err != nil {
		t.Errorf("copyDir on non-existent src should return nil, got: %v", err)
	}
}

func TestCopyDir_ReadFileError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	src := t.TempDir()
	dst := t.TempDir()

	f := filepath.Join(src, "locked.txt")
	if err := os.WriteFile(f, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(f, 0600) }) //nolint:errcheck

	if err := copyDir(src, dst); err == nil {
		t.Error("copyDir should fail when a file cannot be read")
	}
}

func TestCopyDir_WriteFileError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	src := t.TempDir()
	dst := t.TempDir()

	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dst, 0555); err != nil { // read-only
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dst, 0755) }) //nolint:errcheck

	if err := copyDir(src, dst); err == nil {
		t.Error("copyDir should fail when destination is read-only")
	}
}

func TestCopyDir_ReadDirError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	// Create a dir that exists but is unreadable (non-IsNotExist error).
	src := t.TempDir()
	if err := os.Chmod(src, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(src, 0755) }) //nolint:errcheck

	dst := t.TempDir()
	if err := copyDir(src, dst); err == nil {
		t.Error("copyDir should fail when src dir is unreadable")
	}
}

func TestCopyDir_RecursiveError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	src := t.TempDir()
	dst := t.TempDir()

	sub := filepath.Join(src, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(sub, "locked.txt")
	if err := os.WriteFile(locked, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0600) }) //nolint:errcheck

	if err := copyDir(src, dst); err == nil {
		t.Error("copyDir should fail when recursive copy encounters an unreadable file")
	}
}

func TestCopyDir_MkdirAllError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create a subdir in src
	sub := filepath.Join(src, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "f.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	// Block dst/sub creation by placing a regular file there
	if err := os.WriteFile(filepath.Join(dst, "sub"), []byte("blocker"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := copyDir(src, dst); err == nil {
		t.Error("copyDir should fail when MkdirAll cannot create a subdir")
	}
}

// ── loadFavorites ─────────────────────────────────────────────────────────────

func TestLoadFavorites_EmptyPath(t *testing.T) {
	b := NewBuilder(&config.Config{}, false)
	if got := b.loadFavorites(nil); got != nil {
		t.Errorf("empty favorites path should return nil, got %v", got)
	}
}

func TestLoadFavorites_ValidFile(t *testing.T) {
	dir := t.TempDir()
	favFile := filepath.Join(dir, "favorites.json")
	if err := os.WriteFile(favFile, []byte(`["slug-a","slug-b"]`), 0600); err != nil {
		t.Fatal(err)
	}

	ents := map[string]*entity.Entity{
		"slug-a": {Slug: "slug-a"},
		"slug-b": {Slug: "slug-b"},
	}
	b := NewBuilder(&config.Config{Extra: config.ExtraConfig{Favorites: favFile}}, false)
	result := b.loadFavorites(ents)
	if len(result) != 2 {
		t.Errorf("expected 2 favorites, got %d", len(result))
	}
}

func TestLoadFavorites_MissingFile(t *testing.T) {
	b := NewBuilder(&config.Config{Extra: config.ExtraConfig{Favorites: "/nonexistent/favorites.json"}}, false)
	if got := b.loadFavorites(nil); got != nil {
		t.Errorf("missing file should return nil, got %v", got)
	}
}

func TestLoadFavorites_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	favFile := filepath.Join(dir, "favorites.json")
	if err := os.WriteFile(favFile, []byte(`{not valid`), 0600); err != nil {
		t.Fatal(err)
	}
	b := NewBuilder(&config.Config{Extra: config.ExtraConfig{Favorites: favFile}}, false)
	if got := b.loadFavorites(nil); got != nil {
		t.Errorf("invalid JSON should return nil, got %v", got)
	}
}

// ── loadContributors ──────────────────────────────────────────────────────────

func TestLoadContributors_EmptyPath(t *testing.T) {
	b := NewBuilder(&config.Config{}, false)
	if got := b.loadContributors(); got != nil {
		t.Errorf("empty contributors path should return nil, got %v", got)
	}
}

func TestLoadContributors_ValidFile(t *testing.T) {
	dir := t.TempDir()
	cFile := filepath.Join(dir, "contributors.json")
	if err := os.WriteFile(cFile, []byte(`{"alice":{"role":"editor"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	b := NewBuilder(&config.Config{Extra: config.ExtraConfig{Contributors: cFile}}, false)
	result := b.loadContributors()
	if result == nil {
		t.Error("should return non-nil map for valid JSON")
	}
	if _, ok := result["alice"]; !ok {
		t.Error("result should contain 'alice'")
	}
}

func TestLoadContributors_MissingFile(t *testing.T) {
	b := NewBuilder(&config.Config{Extra: config.ExtraConfig{Contributors: "/nonexistent/contributors.json"}}, false)
	if got := b.loadContributors(); got != nil {
		t.Errorf("missing file should return nil, got %v", got)
	}
}

func TestLoadContributors_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cFile := filepath.Join(dir, "contributors.json")
	if err := os.WriteFile(cFile, []byte(`not json`), 0600); err != nil {
		t.Fatal(err)
	}
	b := NewBuilder(&config.Config{Extra: config.ExtraConfig{Contributors: cFile}}, false)
	if got := b.loadContributors(); got != nil {
		t.Errorf("invalid JSON should return nil, got %v", got)
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
