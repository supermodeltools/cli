package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/affiliate"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/taxonomy"
)

// ── NewEngine ─────────────────────────────────────────────────────────────────

func TestNewEngine_MissingTemplateDir(t *testing.T) {
	cfg := &config.Config{Paths: config.PathsConfig{Templates: "/nonexistent-templates-dir"}}
	_, err := NewEngine(cfg)
	if err == nil {
		t.Error("NewEngine: want error for missing template dir, got nil")
	}
}

func TestNewEngine_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	e, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine with empty dir: %v", err)
	}
	if e == nil {
		t.Error("NewEngine: want non-nil Engine")
	}
}

func TestNewEngine_SkipsNonHTMLFiles(t *testing.T) {
	dir := t.TempDir()
	// A .txt file should be skipped without error.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	if _, err := NewEngine(cfg); err != nil {
		t.Fatalf("NewEngine should skip non-html files: %v", err)
	}
}

func TestNewEngine_SkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subDir, 0750); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	if _, err := NewEngine(cfg); err != nil {
		t.Fatalf("NewEngine should skip subdirs: %v", err)
	}
}

func TestNewEngine_ValidHTMLTemplate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(`<p>{{.Title}}</p>`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine with valid template: %v", err)
	}
	if eng == nil {
		t.Error("NewEngine: want non-nil Engine")
	}
}

func TestNewEngine_InvalidTemplate(t *testing.T) {
	dir := t.TempDir()
	// Malformed Go template syntax.
	if err := os.WriteFile(filepath.Join(dir, "bad.html"), []byte(`{{.Unclosed`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	_, err := NewEngine(cfg)
	if err == nil {
		t.Error("NewEngine: want error for invalid template syntax")
	}
}

func TestNewEngine_ReadFileError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.html")
	if err := os.WriteFile(path, []byte(`<p>hi</p>`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0600) }) //nolint:errcheck
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	_, err := NewEngine(cfg)
	if err == nil {
		t.Error("NewEngine: want error when template file is unreadable")
	}
}

// TestEngine_RenderMethods tests the Engine render methods with a minimal template.
func TestEngine_RenderMethods(t *testing.T) {
	dir := t.TempDir()
	tmplContent := `{{.}}`
	for _, name := range []string{"entity.html", "homepage.html", "all_entities.html", "static.html"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(tmplContent), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{
		Paths: config.PathsConfig{Templates: dir},
		Templates: config.TemplatesConfig{
			Entity:   "entity.html",
			Homepage: "homepage.html",
		},
	}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	if _, err := eng.RenderEntity(EntityPageContext{}); err != nil {
		t.Errorf("RenderEntity: %v", err)
	}
	if _, err := eng.RenderHomepage(HomepageContext{}); err != nil {
		t.Errorf("RenderHomepage: %v", err)
	}
	if _, err := eng.RenderAllEntities(AllEntitiesPageContext{}); err != nil {
		t.Errorf("RenderAllEntities: %v", err)
	}
	if _, err := eng.RenderStatic("static.html", StaticPageContext{}); err != nil {
		t.Errorf("RenderStatic: %v", err)
	}
}

func TestEngine_RenderNotFound(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if _, err := eng.RenderStatic("nonexistent.html", StaticPageContext{}); err == nil {
		t.Error("render: want error for missing template, got nil")
	}
}

func TestEngine_RenderCSS_Missing(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	// No _styles.css → should return ("", nil).
	css, err := eng.RenderCSS()
	if err != nil {
		t.Errorf("RenderCSS missing: %v", err)
	}
	if css != "" {
		t.Errorf("RenderCSS missing: want empty, got %q", css)
	}
}

func TestEngine_RenderCSS_Present(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "_styles.css"), []byte(`body { color: red; }`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	css, err := eng.RenderCSS()
	if err != nil {
		t.Errorf("RenderCSS: %v", err)
	}
	if !strings.Contains(css, "color: red") {
		t.Errorf("RenderCSS: expected CSS content, got %q", css)
	}
}

func TestEngine_RenderJS_Missing(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	js, err := eng.RenderJS()
	if err != nil {
		t.Errorf("RenderJS missing: %v", err)
	}
	if js != "" {
		t.Errorf("RenderJS missing: want empty, got %q", js)
	}
}

func TestEngine_RenderHub(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hub.html"), []byte(`hub`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	ctx := HubPageContext{
		Taxonomy: taxonomy.Taxonomy{
			Config: config.TaxonomyConfig{Template: "hub.html"},
		},
	}
	if _, err := eng.RenderHub(ctx); err != nil {
		t.Errorf("RenderHub: %v", err)
	}
}

func TestEngine_RenderTaxonomyIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(`index`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	ctx := TaxonomyIndexContext{
		Taxonomy: taxonomy.Taxonomy{
			Config: config.TaxonomyConfig{IndexTemplate: "index.html"},
		},
	}
	if _, err := eng.RenderTaxonomyIndex(ctx); err != nil {
		t.Errorf("RenderTaxonomyIndex: %v", err)
	}
}

func TestEngine_RenderLetter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "letter.html"), []byte(`letter`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	ctx := LetterPageContext{
		Taxonomy: taxonomy.Taxonomy{
			Config: config.TaxonomyConfig{LetterTemplate: "letter.html"},
		},
	}
	if _, err := eng.RenderLetter(ctx); err != nil {
		t.Errorf("RenderLetter: %v", err)
	}
}

// TestEngine_RenderExecuteError covers L240-242: when template execution fails,
// render returns an error.
func TestEngine_RenderExecuteError(t *testing.T) {
	dir := t.TempDir()
	// Template that calls a non-existent sub-template → execute error.
	if err := os.WriteFile(filepath.Join(dir, "broken.html"), []byte(`{{template "nonexistent"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if _, err := eng.RenderStatic("broken.html", StaticPageContext{}); err == nil {
		t.Error("render: want error when template execution fails, got nil")
	}
}

func TestEngine_RenderJS_Present(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "_main.js"), []byte(`console.log("hi");`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.PathsConfig{Templates: dir}}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	js, err := eng.RenderJS()
	if err != nil {
		t.Errorf("RenderJS: %v", err)
	}
	if !strings.Contains(js, "console.log") {
		t.Errorf("RenderJS: expected JS content, got %q", js)
	}
}

// ── GenerateCookModePrompt ────────────────────────────────────────────────────

func TestGenerateCookModePrompt_NilEnrichment(t *testing.T) {
	e := &entity.Entity{Fields: map[string]interface{}{"title": "Pasta"}}
	if got := GenerateCookModePrompt(e, nil, nil); got != "" {
		t.Errorf("nil enrichment: want empty string, got %q", got)
	}
}

func TestGenerateCookModePrompt_BasicTitle(t *testing.T) {
	e := &entity.Entity{Fields: map[string]interface{}{"title": "Spaghetti"}}
	enrichment := map[string]interface{}{}
	got := GenerateCookModePrompt(e, enrichment, nil)
	if !strings.Contains(got, "Spaghetti") {
		t.Errorf("should contain title, got:\n%s", got)
	}
	if !strings.Contains(got, "step by step") {
		t.Errorf("should contain closing prompt, got:\n%s", got)
	}
}

func TestGenerateCookModePrompt_CoachingPrompt(t *testing.T) {
	e := &entity.Entity{Fields: map[string]interface{}{"title": "Risotto"}}
	enrichment := map[string]interface{}{
		"coachingPrompt": "Pay attention to stirring technique.",
	}
	got := GenerateCookModePrompt(e, enrichment, nil)
	if !strings.Contains(got, "Pay attention to stirring technique.") {
		t.Errorf("should contain coachingPrompt, got:\n%s", got)
	}
}

func TestGenerateCookModePrompt_Ingredients(t *testing.T) {
	e := &entity.Entity{
		Fields: map[string]interface{}{"title": "Soup"},
		Sections: map[string]interface{}{
			"ingredients": []string{"1 carrot", "2 potatoes"},
		},
	}
	got := GenerateCookModePrompt(e, map[string]interface{}{}, nil)
	if !strings.Contains(got, "Ingredients:") {
		t.Errorf("should contain Ingredients section, got:\n%s", got)
	}
	if !strings.Contains(got, "- 1 carrot") {
		t.Errorf("should list ingredients, got:\n%s", got)
	}
}

func TestGenerateCookModePrompt_Instructions(t *testing.T) {
	e := &entity.Entity{
		Fields: map[string]interface{}{"title": "Cake"},
		Sections: map[string]interface{}{
			"instructions": []string{"Mix flour", "Bake at 350°F"},
		},
	}
	got := GenerateCookModePrompt(e, map[string]interface{}{}, nil)
	if !strings.Contains(got, "Instructions:") {
		t.Errorf("should contain Instructions section, got:\n%s", got)
	}
	if !strings.Contains(got, "1. Mix flour") {
		t.Errorf("should number instructions, got:\n%s", got)
	}
}

func TestGenerateCookModePrompt_CookingTips(t *testing.T) {
	e := &entity.Entity{Fields: map[string]interface{}{"title": "Steak"}}
	enrichment := map[string]interface{}{
		"cookingTips": []interface{}{"Let it rest", "Season generously"},
	}
	got := GenerateCookModePrompt(e, enrichment, nil)
	if !strings.Contains(got, "Key Tips:") {
		t.Errorf("should contain Key Tips section, got:\n%s", got)
	}
	if !strings.Contains(got, "- Let it rest") {
		t.Errorf("should list tips, got:\n%s", got)
	}
}

func TestGenerateCookModePrompt_CookingTipsNonString(t *testing.T) {
	e := &entity.Entity{Fields: map[string]interface{}{"title": "Steak"}}
	enrichment := map[string]interface{}{
		// tip is an int, not a string — should be skipped
		"cookingTips": []interface{}{42, "Use salt"},
	}
	got := GenerateCookModePrompt(e, enrichment, nil)
	if !strings.Contains(got, "Key Tips:") {
		t.Errorf("should contain Key Tips (one valid tip), got:\n%s", got)
	}
	if !strings.Contains(got, "- Use salt") {
		t.Errorf("should include string tip, got:\n%s", got)
	}
}

func TestGenerateCookModePrompt_AffiliateLinks(t *testing.T) {
	e := &entity.Entity{Fields: map[string]interface{}{"title": "Tacos"}}
	links := []affiliate.Link{
		{Term: "cumin", URL: "https://shop.example.com/cumin", Provider: "Amazon"},
	}
	got := GenerateCookModePrompt(e, map[string]interface{}{}, links)
	if !strings.Contains(got, "Shopping Links:") {
		t.Errorf("should contain Shopping Links section, got:\n%s", got)
	}
	if !strings.Contains(got, "cumin") {
		t.Errorf("should list affiliate term, got:\n%s", got)
	}
	if !strings.Contains(got, "Amazon") {
		t.Errorf("should list provider, got:\n%s", got)
	}
}

func TestGenerateCookModePrompt_AllSections(t *testing.T) {
	e := &entity.Entity{
		Fields: map[string]interface{}{"title": "Full Recipe"},
		Sections: map[string]interface{}{
			"ingredients":  []string{"flour", "eggs"},
			"instructions": []string{"Mix", "Bake"},
		},
	}
	enrichment := map[string]interface{}{
		"coachingPrompt": "Take your time.",
		"cookingTips":    []interface{}{"Don't over-mix"},
	}
	links := []affiliate.Link{
		{Term: "flour", URL: "https://shop.example.com/flour", Provider: "Store"},
	}
	got := GenerateCookModePrompt(e, enrichment, links)
	for _, want := range []string{
		"Full Recipe", "Take your time.",
		"Ingredients:", "- flour",
		"Instructions:", "1. Mix",
		"Key Tips:", "- Don't over-mix",
		"Shopping Links:", "flour",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}
