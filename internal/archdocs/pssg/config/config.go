package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a YAML config file, applies defaults, and validates.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	cfg.ConfigDir = filepath.Dir(path)
	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	// Resolve relative paths against config directory
	resolvePaths(&cfg)

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Site.Language == "" {
		cfg.Site.Language = "en"
	}
	if cfg.Paths.Output == "" {
		cfg.Paths.Output = "docs"
	}
	if cfg.Paths.Templates == "" {
		cfg.Paths.Templates = "templates"
	}
	if cfg.Paths.Cache == "" {
		cfg.Paths.Cache = ".cache"
	}
	if cfg.Data.Format == "" {
		cfg.Data.Format = "markdown"
	}
	if cfg.Data.EntitySlug.Source == "" {
		cfg.Data.EntitySlug.Source = "filename"
	}
	if cfg.Pagination.EntitiesPerPage == 0 {
		cfg.Pagination.EntitiesPerPage = 48
	}
	if cfg.Sitemap.MaxURLsPerFile == 0 {
		cfg.Sitemap.MaxURLsPerFile = 50000
	}
	if cfg.Schema.DatePublished == "" {
		cfg.Schema.DatePublished = "2025-01-01"
	}

	// Default taxonomy settings
	for i := range cfg.Taxonomies {
		if cfg.Taxonomies[i].MinEntities == 0 {
			cfg.Taxonomies[i].MinEntities = 1
		}
		if cfg.Taxonomies[i].LetterPageThreshold == 0 {
			cfg.Taxonomies[i].LetterPageThreshold = 50
		}
		if cfg.Taxonomies[i].Template == "" {
			cfg.Taxonomies[i].Template = "hub.html"
		}
		if cfg.Taxonomies[i].IndexTemplate == "" {
			cfg.Taxonomies[i].IndexTemplate = "taxonomy_index.html"
		}
		if cfg.Taxonomies[i].LetterTemplate == "" {
			cfg.Taxonomies[i].LetterTemplate = "letter.html"
		}
	}

	// Default sitemap priorities
	if cfg.Sitemap.Priorities == nil {
		cfg.Sitemap.Priorities = map[string]string{
			"homepage":       "1.0",
			"entity":         "0.8",
			"taxonomy_index": "0.7",
			"hub_page_1":     "0.6",
			"hub_page_n":     "0.4",
			"letter_page":    "0.5",
		}
	}
	if cfg.Sitemap.ChangeFreqs == nil {
		cfg.Sitemap.ChangeFreqs = map[string]string{
			"homepage":       "daily",
			"entity":         "weekly",
			"taxonomy_index": "weekly",
			"hub":            "weekly",
			"letter_page":    "weekly",
		}
	}

	// Default templates
	if cfg.Templates.Entity == "" {
		cfg.Templates.Entity = "recipe.html"
	}
	if cfg.Templates.Homepage == "" {
		cfg.Templates.Homepage = "index.html"
	}
	if cfg.Templates.Hub == "" {
		cfg.Templates.Hub = "hub.html"
	}
	if cfg.Templates.TaxonomyIndex == "" {
		cfg.Templates.TaxonomyIndex = "taxonomy_index.html"
	}
	if cfg.Templates.Letter == "" {
		cfg.Templates.Letter = "letter.html"
	}
}

func validate(cfg *Config) error {
	if cfg.Site.Name == "" {
		return fmt.Errorf("site.name is required")
	}
	if cfg.Site.BaseURL == "" {
		return fmt.Errorf("site.base_url is required")
	}
	if cfg.Paths.Data == "" {
		return fmt.Errorf("paths.data is required")
	}
	return nil
}

func resolvePaths(cfg *Config) {
	resolve := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(cfg.ConfigDir, p)
	}

	cfg.Paths.Data = resolve(cfg.Paths.Data)
	cfg.Paths.Templates = resolve(cfg.Paths.Templates)
	cfg.Paths.Output = resolve(cfg.Paths.Output)
	cfg.Paths.Cache = resolve(cfg.Paths.Cache)
	if cfg.Paths.Static != "" {
		cfg.Paths.Static = resolve(cfg.Paths.Static)
	}
	if cfg.Enrichment.CacheDir != "" {
		cfg.Enrichment.CacheDir = resolve(cfg.Enrichment.CacheDir)
	}
	if cfg.Extra.Favorites != "" {
		cfg.Extra.Favorites = resolve(cfg.Extra.Favorites)
	}
	if cfg.Extra.Contributors != "" {
		cfg.Extra.Contributors = resolve(cfg.Extra.Contributors)
	}
}
