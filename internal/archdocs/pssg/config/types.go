package config

// Config is the top-level pssg configuration loaded from YAML.
type Config struct {
	Site       SiteConfig       `yaml:"site"`
	Paths      PathsConfig      `yaml:"paths"`
	Data       DataConfig       `yaml:"data"`
	Taxonomies []TaxonomyConfig `yaml:"taxonomies"`
	Pagination PaginationConfig `yaml:"pagination"`
	Schema     SchemaConfig     `yaml:"structured_data"`
	Affiliates AffiliatesConfig `yaml:"affiliates"`
	Enrichment EnrichmentConfig `yaml:"enrichment"`
	Sitemap    SitemapConfig    `yaml:"sitemap"`
	RSS        RSSConfig        `yaml:"rss"`
	Robots     RobotsConfig     `yaml:"robots"`
	LlmsTxt    LlmsTxtConfig   `yaml:"llms_txt"`
	Templates  TemplatesConfig  `yaml:"templates"`
	Output     OutputConfig     `yaml:"output"`
	Extra      ExtraConfig      `yaml:"extra"`
	Search     SearchConfig     `yaml:"search"`

	// ConfigDir is the directory containing the config file (set at load time).
	ConfigDir string `yaml:"-"`
}

type SiteConfig struct {
	Name        string `yaml:"name"`
	BaseURL     string `yaml:"base_url"`
	RepoURL     string `yaml:"repo_url"`
	Description string `yaml:"description"`
	Language    string `yaml:"language"`
	Version     string `yaml:"version"`
	Author      string `yaml:"author"`
	AuthorURL   string `yaml:"author_url"`
	License     string `yaml:"license"`
	CNAME       string `yaml:"cname"`
}

type PathsConfig struct {
	Data      string `yaml:"data"`
	Templates string `yaml:"templates"`
	Output    string `yaml:"output"`
	Cache     string `yaml:"cache"`
	Static    string `yaml:"static"`
	SourceDir string `yaml:"source_dir"`
}

type DataConfig struct {
	Format      string       `yaml:"format"`
	EntityType  string       `yaml:"entity_type"`
	EntitySlug  EntitySlug   `yaml:"entity_slug"`
	BodySections []BodySection `yaml:"body_sections"`
}

type EntitySlug struct {
	Source string `yaml:"source"` // "filename" or "field:<name>"
}

type BodySection struct {
	Name   string `yaml:"name"`
	Header string `yaml:"header"`
	Type   string `yaml:"type"` // "unordered_list", "ordered_list", "faq", "markdown"
}

type TaxonomyConfig struct {
	Name                     string `yaml:"name"`
	Label                    string `yaml:"label"`
	LabelSingular            string `yaml:"label_singular"`
	Field                    string `yaml:"field"`
	MultiValue               bool   `yaml:"multi_value"`
	MinEntities              int    `yaml:"min_entities"`
	LetterPageThreshold      int    `yaml:"letter_page_threshold"`
	Invert                   bool   `yaml:"invert"`
	EnrichmentOverrideField  string `yaml:"enrichment_override_field"`
	Template                 string `yaml:"template"`
	IndexTemplate            string `yaml:"index_template"`
	LetterTemplate           string `yaml:"letter_template"`

	// Description templates (Go template strings evaluated with .Name, .Count, .Start, .End)
	HubTitle           string `yaml:"hub_title"`
	HubMetaDescription string `yaml:"hub_meta_description"`
	HubSubheading      string `yaml:"hub_subheading"`
	IndexDescription   string `yaml:"index_description"`
	CollectionDesc     string `yaml:"collection_description"`
}

type PaginationConfig struct {
	EntitiesPerPage int `yaml:"entities_per_page"`
}

type SchemaConfig struct {
	EntityType     string            `yaml:"entity_type"`
	FieldMappings  map[string]string `yaml:"field_mappings"`
	ExtraKeywords  []string          `yaml:"extra_keywords"`
	DatePublished  string            `yaml:"date_published"`
	HomepageSchema []string          `yaml:"homepage_schemas"`
	EntitySchema   []string          `yaml:"entity_schemas"`
	HubSchema      []string          `yaml:"hub_schemas"`
	IndexSchema    []string          `yaml:"index_schemas"`
}

type AffiliatesConfig struct {
	Providers []AffiliateProviderConfig `yaml:"providers"`
	SearchTermPaths []string            `yaml:"search_term_paths"`
}

type AffiliateProviderConfig struct {
	Name       string `yaml:"name"`
	URLTemplate string `yaml:"url_template"`
	EnvVar     string `yaml:"env_var"`
	AlwaysInclude bool `yaml:"always_include"`
}

type EnrichmentConfig struct {
	CacheDir string `yaml:"cache_dir"`
	IngredientOverrideField string `yaml:"ingredient_override_field"`
}

type SitemapConfig struct {
	MaxURLsPerFile int                       `yaml:"max_urls_per_file"`
	Priorities     map[string]string         `yaml:"priorities"`
	ChangeFreqs    map[string]string         `yaml:"change_freqs"`
}

type RSSConfig struct {
	Enabled       bool   `yaml:"enabled"`
	MainFeed      string `yaml:"main_feed"`
	CategoryFeeds bool   `yaml:"category_feeds"`
	CategoryTaxonomy string `yaml:"category_taxonomy"`
}

type RobotsConfig struct {
	AllowAll   bool     `yaml:"allow_all"`
	ExtraBots  []string `yaml:"extra_bots"`
}

type LlmsTxtConfig struct {
	Enabled    bool     `yaml:"enabled"`
	Tagline    string   `yaml:"tagline"`
	Taxonomies []string `yaml:"taxonomies"`
}

type TemplatesConfig struct {
	Entity         string            `yaml:"entity"`
	Homepage       string            `yaml:"homepage"`
	Hub            string            `yaml:"hub"`
	TaxonomyIndex  string            `yaml:"taxonomy_index"`
	Letter         string            `yaml:"letter"`
	StaticPages    map[string]string `yaml:"static_pages"`
}

type OutputConfig struct {
	CleanBuild  bool   `yaml:"clean_build"`
	Minify      bool   `yaml:"minify"`
	ExtractCSS  string `yaml:"extract_css"`
	ExtractJS   string `yaml:"extract_js"`
	ShareImages bool   `yaml:"share_images"`
}

type ExtraConfig struct {
	Favorites    string    `yaml:"favorites"`
	Contributors string    `yaml:"contributors"`
	CTA          CTAConfig `yaml:"cta"`
}

type CTAConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Heading     string `yaml:"heading"`
	Description string `yaml:"description"`
	ButtonText  string `yaml:"button_text"`
	ButtonURL   string `yaml:"button_url"`
}

type SearchConfig struct {
	Enabled bool     `yaml:"enabled"`
	Fields  []string `yaml:"fields"` // entity fields to index, default: ["title","description","node_type","language","domain","subdomain","tags"]
}
