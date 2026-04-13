package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()
	return f.Name()
}

// ── Load ──────────────────────────────────────────────────────────────────────

func TestLoad_Valid(t *testing.T) {
	path := writeYAML(t, `
site:
  name: "My Site"
  base_url: "https://example.com"
paths:
  data: "data"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Site.Name != "My Site" {
		t.Errorf("site.name: got %q", cfg.Site.Name)
	}
	// Defaults should be applied
	if cfg.Site.Language != "en" {
		t.Errorf("language default: got %q", cfg.Site.Language)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeYAML(t, "not: valid: yaml: {")
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoad_MissingSiteName(t *testing.T) {
	path := writeYAML(t, `
site:
  base_url: "https://example.com"
paths:
  data: "data"
`)
	_, err := Load(path)
	if err == nil {
		t.Error("expected validation error for missing site.name")
	}
}

func TestLoad_MissingBaseURL(t *testing.T) {
	path := writeYAML(t, `
site:
  name: "My Site"
paths:
  data: "data"
`)
	_, err := Load(path)
	if err == nil {
		t.Error("expected validation error for missing site.base_url")
	}
}

func TestLoad_MissingPathsData(t *testing.T) {
	path := writeYAML(t, `
site:
  name: "My Site"
  base_url: "https://example.com"
`)
	_, err := Load(path)
	if err == nil {
		t.Error("expected validation error for missing paths.data")
	}
}

// ── applyDefaults ─────────────────────────────────────────────────────────────

func TestApplyDefaults_SetsLanguage(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	if cfg.Site.Language != "en" {
		t.Errorf("language: got %q", cfg.Site.Language)
	}
}

func TestApplyDefaults_PreservesExistingLanguage(t *testing.T) {
	cfg := &Config{Site: SiteConfig{Language: "fr"}}
	applyDefaults(cfg)
	if cfg.Site.Language != "fr" {
		t.Errorf("should preserve existing language 'fr', got %q", cfg.Site.Language)
	}
}

func TestApplyDefaults_SetsOutputPath(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	if cfg.Paths.Output != "docs" {
		t.Errorf("output: got %q", cfg.Paths.Output)
	}
}

func TestApplyDefaults_SetsPagination(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	if cfg.Pagination.EntitiesPerPage != 48 {
		t.Errorf("entities_per_page: got %d", cfg.Pagination.EntitiesPerPage)
	}
}

func TestApplyDefaults_SetsTaxonomyDefaults(t *testing.T) {
	cfg := &Config{
		Taxonomies: []TaxonomyConfig{{}},
	}
	applyDefaults(cfg)
	if cfg.Taxonomies[0].MinEntities != 1 {
		t.Errorf("min_entities: got %d", cfg.Taxonomies[0].MinEntities)
	}
	if cfg.Taxonomies[0].LetterPageThreshold != 50 {
		t.Errorf("letter_page_threshold: got %d", cfg.Taxonomies[0].LetterPageThreshold)
	}
	if cfg.Taxonomies[0].Template != "hub.html" {
		t.Errorf("template: got %q", cfg.Taxonomies[0].Template)
	}
}

func TestApplyDefaults_SetsSitemapDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	if cfg.Sitemap.Priorities == nil {
		t.Error("sitemap priorities should be set")
	}
	if cfg.Sitemap.ChangeFreqs == nil {
		t.Error("sitemap change freqs should be set")
	}
}

func TestApplyDefaults_SetsTemplateDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	if cfg.Templates.Entity != "recipe.html" {
		t.Errorf("entity template: got %q", cfg.Templates.Entity)
	}
	if cfg.Templates.Homepage != "index.html" {
		t.Errorf("homepage template: got %q", cfg.Templates.Homepage)
	}
}

// ── resolvePaths ──────────────────────────────────────────────────────────────

func TestResolvePaths_RelativePaths(t *testing.T) {
	cfg := &Config{
		ConfigDir: "/base",
		Paths: PathsConfig{
			Data:      "data",
			Templates: "templates",
			Output:    "docs",
			Cache:     ".cache",
		},
	}
	resolvePaths(cfg)
	if cfg.Paths.Data != filepath.Join("/base", "data") {
		t.Errorf("data: got %q", cfg.Paths.Data)
	}
	if cfg.Paths.Output != filepath.Join("/base", "docs") {
		t.Errorf("output: got %q", cfg.Paths.Output)
	}
}

func TestResolvePaths_AbsPathPreserved(t *testing.T) {
	absData := filepath.Join(string(filepath.Separator), "absolute", "data")
	cfg := &Config{
		ConfigDir: filepath.Join(string(filepath.Separator), "base"),
		Paths: PathsConfig{
			Data:      absData,
			Templates: filepath.Join(string(filepath.Separator), "absolute", "templates"),
			Output:    filepath.Join(string(filepath.Separator), "absolute", "docs"),
			Cache:     filepath.Join(string(filepath.Separator), "absolute", ".cache"),
		},
	}
	resolvePaths(cfg)
	if cfg.Paths.Data != absData {
		t.Errorf("absolute path should be preserved: got %q", cfg.Paths.Data)
	}
}

func TestResolvePaths_OptionalPaths(t *testing.T) {
	cfg := &Config{
		ConfigDir: "/base",
		Paths: PathsConfig{
			Data:      "data",
			Templates: "templates",
			Output:    "docs",
			Cache:     ".cache",
			Static:    "static",
		},
		Enrichment: EnrichmentConfig{CacheDir: "enrichment-cache"},
		Extra: ExtraConfig{
			Favorites:    "favorites.json",
			Contributors: "contributors.json",
		},
	}
	resolvePaths(cfg)
	if cfg.Paths.Static != filepath.Join("/base", "static") {
		t.Errorf("static: got %q", cfg.Paths.Static)
	}
	if cfg.Enrichment.CacheDir != filepath.Join("/base", "enrichment-cache") {
		t.Errorf("enrichment cache dir: got %q", cfg.Enrichment.CacheDir)
	}
	if cfg.Extra.Favorites != filepath.Join("/base", "favorites.json") {
		t.Errorf("favorites: got %q", cfg.Extra.Favorites)
	}
	if cfg.Extra.Contributors != filepath.Join("/base", "contributors.json") {
		t.Errorf("contributors: got %q", cfg.Extra.Contributors)
	}
}

func TestResolvePaths_EmptyOptionalPaths(t *testing.T) {
	cfg := &Config{
		ConfigDir: "/base",
		Paths: PathsConfig{
			Data:      "data",
			Templates: "templates",
			Output:    "docs",
			Cache:     ".cache",
			// Static empty
		},
		// Enrichment.CacheDir empty
		// Extra.Favorites empty
		// Extra.Contributors empty
	}
	resolvePaths(cfg)
	if cfg.Paths.Static != "" {
		t.Errorf("empty static should remain empty, got %q", cfg.Paths.Static)
	}
	if cfg.Enrichment.CacheDir != "" {
		t.Errorf("empty enrichment cache dir should remain empty")
	}
}
