package render

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/affiliate"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/taxonomy"
)

// Engine is the template rendering engine.
type Engine struct {
	tmpl    *template.Template
	cfg     *config.Config
}

// EntityPageContext is the template context for entity (recipe) pages.
type EntityPageContext struct {
	Site            config.SiteConfig
	Entity          *entity.Entity
	Slug            string
	URL             string
	CanonicalURL    string
	Breadcrumbs     []Breadcrumb
	Pairings        []*entity.Entity
	Enrichment      map[string]interface{}
	AffiliateLinks  []affiliate.Link
	CookModePrompt  string
	JsonLD          template.HTML
	Taxonomies      []taxonomy.Taxonomy
	AllTaxonomies   []taxonomy.Taxonomy
	ValidSlugs      map[string]map[string]bool
	Contributors    map[string]interface{}
	OG              OGMeta
	ChartData       template.JS
	CTA             config.CTAConfig
	SourceCode      string
	SourceLang      string
}

// HomepageContext is the template context for the homepage.
type HomepageContext struct {
	Site          config.SiteConfig
	Entities      []*entity.Entity
	Taxonomies    []taxonomy.Taxonomy
	Favorites     []*entity.Entity
	JsonLD        template.HTML
	EntityCount   int
	Contributors  map[string]interface{}
	OG            OGMeta
	ChartData     template.JS
	CTA           config.CTAConfig
	ArchData      template.JS
}

// HubPageContext is the template context for taxonomy hub (category) pages.
type HubPageContext struct {
	Site           config.SiteConfig
	Taxonomy       taxonomy.Taxonomy
	Entry          taxonomy.Entry
	Entities       []*entity.Entity
	Pagination     taxonomy.PaginationInfo
	JsonLD         template.HTML
	Breadcrumbs    []Breadcrumb
	AllTaxonomies  []taxonomy.Taxonomy
	Contributors   map[string]interface{}
	ContributorProfile map[string]interface{}
	OG             OGMeta
	ChartData      template.JS
	CTA            config.CTAConfig
}

// TaxonomyIndexContext is the template context for taxonomy index pages.
type TaxonomyIndexContext struct {
	Site          config.SiteConfig
	Taxonomy      taxonomy.Taxonomy
	Entries       []taxonomy.Entry
	TopEntries    []taxonomy.Entry
	LetterGroups  []taxonomy.LetterGroup
	HasLetters    bool
	Letters       []string
	JsonLD        template.HTML
	Breadcrumbs   []Breadcrumb
	AllTaxonomies []taxonomy.Taxonomy
	OG            OGMeta
	ChartData     template.JS
	CTA           config.CTAConfig
}

// LetterPageContext is the template context for A-Z letter pages.
type LetterPageContext struct {
	Site          config.SiteConfig
	Taxonomy      taxonomy.Taxonomy
	Letter        string
	Entries       []taxonomy.Entry
	Letters       []string
	JsonLD        template.HTML
	Breadcrumbs   []Breadcrumb
	AllTaxonomies []taxonomy.Taxonomy
	OG            OGMeta
	ChartData     template.JS
	CTA           config.CTAConfig
}

// AllEntitiesPageContext is the template context for the all-entities listing pages.
type AllEntitiesPageContext struct {
	Site          config.SiteConfig
	Entities      []*entity.Entity
	Pagination    taxonomy.PaginationInfo
	JsonLD        template.HTML
	Breadcrumbs   []Breadcrumb
	AllTaxonomies []taxonomy.Taxonomy
	EntityCount    int
	TotalEntities  int
	OG             OGMeta
	ChartData      template.JS
	CTA            config.CTAConfig
}

// StaticPageContext is the template context for static pages.
type StaticPageContext struct {
	Site          config.SiteConfig
	Title         string
	Content       template.HTML
	JsonLD        template.HTML
	Breadcrumbs   []Breadcrumb
	AllTaxonomies []taxonomy.Taxonomy
}

// Breadcrumb is a single breadcrumb entry.
type Breadcrumb struct {
	Name string
	URL  string
}

// OGMeta holds Open Graph and Twitter Card metadata for a page.
type OGMeta struct {
	Title       string
	Description string
	URL         string
	ImageURL    string
	Type        string // "website" for homepage, "article" for all others
	SiteName    string
}

// NameCount is a generic name+count pair used for chart data and share images.
type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// NewEngine creates a render engine loading templates from the given directory.
func NewEngine(cfg *config.Config) (*Engine, error) {
	funcMap := BuildFuncMap()

	tmplDir := cfg.Paths.Templates
	entries, err := os.ReadDir(tmplDir)
	if err != nil {
		return nil, fmt.Errorf("reading template dir %s: %w", tmplDir, err)
	}

	tmpl := template.New("").Funcs(funcMap)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".html" && ext != ".css" && ext != ".js" {
			continue
		}

		path := filepath.Join(tmplDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading template %s: %w", name, err)
		}

		_, err = tmpl.New(name).Parse(string(data))
		if err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", name, err)
		}
	}

	return &Engine{tmpl: tmpl, cfg: cfg}, nil
}

// RenderEntity renders an entity page.
func (e *Engine) RenderEntity(ctx EntityPageContext) (string, error) {
	return e.render(e.cfg.Templates.Entity, ctx)
}

// RenderHomepage renders the homepage.
func (e *Engine) RenderHomepage(ctx HomepageContext) (string, error) {
	return e.render(e.cfg.Templates.Homepage, ctx)
}

// RenderHub renders a taxonomy hub page.
func (e *Engine) RenderHub(ctx HubPageContext) (string, error) {
	templateName := ctx.Taxonomy.Config.Template
	return e.render(templateName, ctx)
}

// RenderTaxonomyIndex renders a taxonomy index page.
func (e *Engine) RenderTaxonomyIndex(ctx TaxonomyIndexContext) (string, error) {
	templateName := ctx.Taxonomy.Config.IndexTemplate
	return e.render(templateName, ctx)
}

// RenderLetter renders a letter page.
func (e *Engine) RenderLetter(ctx LetterPageContext) (string, error) {
	templateName := ctx.Taxonomy.Config.LetterTemplate
	return e.render(templateName, ctx)
}

// RenderAllEntities renders an all-entities listing page.
func (e *Engine) RenderAllEntities(ctx AllEntitiesPageContext) (string, error) {
	return e.render("all_entities.html", ctx)
}

// RenderStatic renders a static page.
func (e *Engine) RenderStatic(templateName string, ctx StaticPageContext) (string, error) {
	return e.render(templateName, ctx)
}

func (e *Engine) render(name string, data interface{}) (string, error) {
	t := e.tmpl.Lookup(name)
	if t == nil {
		return "", fmt.Errorf("template %q not found", name)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template %q: %w", name, err)
	}

	return buf.String(), nil
}

// RenderCSS reads and returns the CSS template content.
// Uses Tree.Root.String() to avoid html/template escaping in CSS code.
func (e *Engine) RenderCSS() (string, error) {
	t := e.tmpl.Lookup("_styles.css")
	if t == nil {
		return "", nil
	}
	return t.Tree.Root.String(), nil
}

// RenderJS reads and returns the JS template content.
// Uses Tree.Root.String() to avoid html/template escaping < to &lt; in JS code.
func (e *Engine) RenderJS() (string, error) {
	t := e.tmpl.Lookup("_main.js")
	if t == nil {
		return "", nil
	}
	return t.Tree.Root.String(), nil
}

// GenerateCookModePrompt builds a cook-with-AI prompt for a recipe.
func GenerateCookModePrompt(e *entity.Entity, enrichment map[string]interface{}, affiliateLinks []affiliate.Link) string {
	if enrichment == nil {
		return ""
	}

	var parts []string

	title := e.GetString("title")
	parts = append(parts, fmt.Sprintf("I want to cook: %s", title))

	// Coaching prompt
	if cp, ok := enrichment["coachingPrompt"].(string); ok && cp != "" {
		parts = append(parts, cp)
	}

	// Ingredients
	if ingredients := e.GetIngredients(); len(ingredients) > 0 {
		items := make([]string, len(ingredients))
		for i, ing := range ingredients {
			items[i] = "- " + ing
		}
		parts = append(parts, "Ingredients:\n"+strings.Join(items, "\n"))
	}

	// Instructions
	if instructions := e.GetInstructions(); len(instructions) > 0 {
		items := make([]string, len(instructions))
		for i, inst := range instructions {
			items[i] = fmt.Sprintf("%d. %s", i+1, inst)
		}
		parts = append(parts, "Instructions:\n"+strings.Join(items, "\n"))
	}

	// Cooking tips
	if tips, ok := enrichment["cookingTips"].([]interface{}); ok && len(tips) > 0 {
		items := make([]string, 0, len(tips))
		for _, tip := range tips {
			if s, ok := tip.(string); ok {
				items = append(items, "- "+s)
			}
		}
		if len(items) > 0 {
			parts = append(parts, "Key Tips:\n"+strings.Join(items, "\n"))
		}
	}

	// Shopping links
	if len(affiliateLinks) > 0 {
		items := make([]string, 0, len(affiliateLinks))
		for _, link := range affiliateLinks {
			items = append(items, fmt.Sprintf("- %s: %s (%s)", link.Term, link.URL, link.Provider))
		}
		parts = append(parts, "Shopping Links:\n"+strings.Join(items, "\n"))
	}

	parts = append(parts, "Please guide me through this recipe step by step, including timing, technique details, and what to watch for at each stage.")

	return strings.Join(parts, "\n\n")
}
