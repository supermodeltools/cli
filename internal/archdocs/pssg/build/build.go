package build

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/affiliate"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/enrichment"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/loader"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/output"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/render"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/schema"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/taxonomy"
)

// Builder orchestrates the entire static site generation pipeline.
type Builder struct {
	cfg   *config.Config
	force bool
}

// NewBuilder creates a new builder.
func NewBuilder(cfg *config.Config, force bool) *Builder {
	return &Builder{cfg: cfg, force: force}
}

// Build runs the complete build pipeline.
func (b *Builder) Build() error {
	start := time.Now()
	log.Printf("Building site: %s", b.cfg.Site.Name)

	// 1. Load entities
	log.Printf("Loading entities from %s...", b.cfg.Paths.Data)
	ldr := loader.New(b.cfg)
	entities, err := ldr.Load()
	if err != nil {
		return fmt.Errorf("loading entities: %w", err)
	}
	log.Printf("Loaded %d entities", len(entities))

	// 2. Build slug lookup
	slugMap := make(map[string]*entity.Entity)
	for _, e := range entities {
		slugMap[e.Slug] = e
	}

	// 3. Load enrichment cache
	enrichmentData := make(map[string]map[string]interface{})
	if b.cfg.Enrichment.CacheDir != "" {
		log.Printf("Loading enrichment cache from %s...", b.cfg.Enrichment.CacheDir)
		var err error
		enrichmentData, err = enrichment.ReadAllCaches(b.cfg.Enrichment.CacheDir)
		if err != nil {
			log.Printf("Warning: failed to load enrichment cache: %v", err)
		} else {
			log.Printf("Loaded enrichment data for %d entities", len(enrichmentData))
		}
	}

	// 4. Load extra data
	favorites := b.loadFavorites(slugMap)
	contributors := b.loadContributors()

	// 5. Set up affiliate registry
	affiliateRegistry := affiliate.NewRegistry(b.cfg.Affiliates)

	// 6. Build taxonomies
	log.Printf("Building taxonomies...")
	taxonomies := taxonomy.BuildAll(entities, b.cfg.Taxonomies, enrichmentData)
	for _, tax := range taxonomies {
		log.Printf("  %s: %d entries", tax.Label, len(tax.Entries))
	}

	// 7. Build valid taxonomy slug lookup
	validSlugs := make(map[string]map[string]bool)
	for _, tax := range taxonomies {
		slugSet := make(map[string]bool)
		for _, entry := range tax.Entries {
			slugSet[entry.Slug] = true
		}
		validSlugs[tax.Name] = slugSet
	}

	// 8. Ensure output directory exists
	outDir := b.cfg.Paths.Output
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	// 9. Initialize render engine
	log.Printf("Loading templates from %s...", b.cfg.Paths.Templates)
	engine, err := render.NewEngine(b.cfg)
	if err != nil {
		return fmt.Errorf("initializing render engine: %w", err)
	}

	// 10. Extract CSS/JS
	if b.cfg.Output.ExtractCSS != "" {
		cssContent, err := engine.RenderCSS()
		if err != nil {
			log.Printf("Warning: failed to render CSS: %v", err)
		} else if cssContent != "" {
			cssPath := filepath.Join(outDir, b.cfg.Output.ExtractCSS)
			if err := os.WriteFile(cssPath, []byte(cssContent), 0644); err != nil {
				return fmt.Errorf("writing CSS: %w", err)
			}
		}
	}
	if b.cfg.Output.ExtractJS != "" {
		jsContent, err := engine.RenderJS()
		if err != nil {
			log.Printf("Warning: failed to render JS: %v", err)
		} else if jsContent != "" {
			jsPath := filepath.Join(outDir, b.cfg.Output.ExtractJS)
			if err := os.WriteFile(jsPath, []byte(jsContent), 0644); err != nil {
				return fmt.Errorf("writing JS: %w", err)
			}
		}
	}

	// JSON-LD generator
	schemaGen := schema.NewGenerator(b.cfg.Site, b.cfg.Schema)

	// Track sitemap entries
	var sitemapEntries []output.SitemapEntry
	var sitemapMu sync.Mutex
	today := time.Now().Format("2006-01-02")

	addSitemapEntry := func(path, priority, changefreq string) {
		sitemapMu.Lock()
		defer sitemapMu.Unlock()
		sitemapEntries = append(sitemapEntries, output.NewSitemapEntry(
			b.cfg.Site.BaseURL, path, today, priority, changefreq,
		))
	}

	// Track category taxonomy entries for RSS
	categoryEntries := make(map[string][]*entity.Entity)

	// 11. Render entity pages (concurrent)
	log.Printf("Rendering %d entity pages...", len(entities))
	var entityErrors int64
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32) // 32-goroutine pool

	for _, e := range entities {
		wg.Add(1)
		sem <- struct{}{} // acquire
		go func(e *entity.Entity) {
			defer wg.Done()
			defer func() { <-sem }() // release

			err := b.renderEntityPage(e, engine, schemaGen, slugMap, enrichmentData,
				affiliateRegistry, taxonomies, validSlugs, contributors, outDir, addSitemapEntry)
			if err != nil {
				atomic.AddInt64(&entityErrors, 1)
				fmt.Fprintf(os.Stderr, "Warning: failed to render %s: %v\n", e.Slug, err)
			}
		}(e)
	}
	wg.Wait()
	if entityErrors > 0 {
		log.Printf("  %d entity pages had errors", entityErrors)
	}

	// 11b. Generate search index
	if len(entities) > 0 {
		if err := b.generateSearchIndex(entities, outDir); err != nil {
			log.Printf("Warning: failed to generate search index: %v", err)
		}
	}

	// Build category entries for RSS
	for _, tax := range taxonomies {
		if tax.Name == b.cfg.RSS.CategoryTaxonomy {
			for _, entry := range tax.Entries {
				categoryEntries[entry.Slug] = entry.Entities
			}
		}
	}

	// 12. Render taxonomy pages
	log.Printf("Rendering taxonomy pages...")
	for _, tax := range taxonomies {
		if err := b.renderTaxonomyPages(tax, engine, schemaGen, taxonomies, contributors, outDir, addSitemapEntry, today); err != nil {
			return fmt.Errorf("rendering taxonomy %s: %w", tax.Name, err)
		}
	}

	// 12b. Render all-entities pages
	log.Printf("Rendering all-entities pages...")
	if err := b.renderAllEntitiesPages(engine, schemaGen, entities, taxonomies, outDir, addSitemapEntry); err != nil {
		return fmt.Errorf("rendering all-entities pages: %w", err)
	}

	// 13. Render homepage
	log.Printf("Rendering homepage...")
	if err := b.renderHomepage(engine, schemaGen, entities, taxonomies, favorites, contributors, outDir); err != nil {
		return fmt.Errorf("rendering homepage: %w", err)
	}
	addSitemapEntry("/index.html", b.cfg.Sitemap.Priorities["homepage"], b.cfg.Sitemap.ChangeFreqs["homepage"])

	// 14. Render static pages
	for path, tmpl := range b.cfg.Templates.StaticPages {
		ctx := render.StaticPageContext{
			Site:          b.cfg.Site,
			AllTaxonomies: taxonomies,
		}
		html, err := engine.RenderStatic(tmpl, ctx)
		if err != nil {
			log.Printf("Warning: failed to render static page %s: %v", path, err)
			continue
		}
		outPath := filepath.Join(outDir, path)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("creating dir for %s: %w", path, err)
		}
		if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}

	// 15. Generate sitemap
	log.Printf("Generating sitemap (%d entries)...", len(sitemapEntries))
	sitemapFiles := output.GenerateSitemapFiles(sitemapEntries, b.cfg.Site.BaseURL, b.cfg.Sitemap.MaxURLsPerFile)
	for _, sf := range sitemapFiles {
		if err := os.WriteFile(filepath.Join(outDir, sf.Filename), []byte(sf.Content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", sf.Filename, err)
		}
	}
	log.Printf("  Generated %d sitemap file(s)", len(sitemapFiles))

	// 16. Generate RSS
	rssFeeds := output.GenerateRSSFeeds(entities, b.cfg, categoryEntries)
	for _, feed := range rssFeeds {
		feedPath := filepath.Join(outDir, feed.RelativePath)
		if err := os.MkdirAll(filepath.Dir(feedPath), 0755); err != nil {
			return fmt.Errorf("creating dir for RSS %s: %w", feed.RelativePath, err)
		}
		if err := os.WriteFile(feedPath, []byte(feed.Content), 0644); err != nil {
			return fmt.Errorf("writing RSS %s: %w", feed.RelativePath, err)
		}
	}
	if len(rssFeeds) > 0 {
		log.Printf("Generated %d RSS feed(s)", len(rssFeeds))
	}

	// 17. Generate robots.txt
	robotsContent := output.GenerateRobotsTxt(b.cfg)
	if err := os.WriteFile(filepath.Join(outDir, "robots.txt"), []byte(robotsContent), 0644); err != nil {
		return fmt.Errorf("writing robots.txt: %w", err)
	}

	// 18. Generate llms.txt
	if b.cfg.LlmsTxt.Enabled {
		llmsContent := output.GenerateLlmsTxt(b.cfg, entities, taxonomies)
		if err := os.WriteFile(filepath.Join(outDir, "llms.txt"), []byte(llmsContent), 0644); err != nil {
			return fmt.Errorf("writing llms.txt: %w", err)
		}
	}

	// 19. Generate manifest.json
	manifestContent := output.GenerateManifest(b.cfg)
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), []byte(manifestContent), 0644); err != nil {
		return fmt.Errorf("writing manifest.json: %w", err)
	}

	// 20. Write CNAME if configured
	if b.cfg.Site.CNAME != "" {
		if err := os.WriteFile(filepath.Join(outDir, "CNAME"), []byte(b.cfg.Site.CNAME+"\n"), 0644); err != nil {
			return fmt.Errorf("writing CNAME: %w", err)
		}
	}

	// 21. Copy static assets
	if b.cfg.Paths.Static != "" {
		if err := copyDir(b.cfg.Paths.Static, outDir); err != nil {
			log.Printf("Warning: failed to copy static assets: %v", err)
		}
	}

	elapsed := time.Since(start)
	log.Printf("\nBuild complete!")
	log.Printf("  Entities:  %d", len(entities))
	log.Printf("  Taxonomies: %d (%d total entries)", len(taxonomies), countTaxEntries(taxonomies))
	log.Printf("  Sitemap:   %d URLs in %d file(s)", len(sitemapEntries), len(sitemapFiles))
	log.Printf("  Output:    %s", outDir)
	log.Printf("  Duration:  %s", elapsed.Round(time.Millisecond))

	return nil
}

func (b *Builder) renderEntityPage(
	e *entity.Entity,
	engine *render.Engine,
	schemaGen *schema.Generator,
	slugMap map[string]*entity.Entity,
	enrichmentData map[string]map[string]interface{},
	affiliateReg *affiliate.Registry,
	taxonomies []taxonomy.Taxonomy,
	validSlugs map[string]map[string]bool,
	contributors map[string]interface{},
	outDir string,
	addSitemapEntry func(string, string, string),
) error {
	entityURL := fmt.Sprintf("%s/%s.html", b.cfg.Site.BaseURL, e.Slug)

	// Resolve pairings
	var pairings []*entity.Entity
	if pairingsSlugs := e.GetStringSlice("pairings"); len(pairingsSlugs) > 0 {
		for _, ps := range pairingsSlugs {
			if paired, ok := slugMap[ps]; ok {
				pairings = append(pairings, paired)
			}
		}
	}

	// Enrichment data for this entity
	eData := enrichmentData[e.Slug]

	// Generate affiliate links
	var affLinks []affiliate.Link
	if eData != nil {
		affLinks = affiliateReg.GenerateLinks(eData, b.cfg.Affiliates.SearchTermPaths)
	}

	// Cook mode prompt
	cookPrompt := render.GenerateCookModePrompt(e, eData, affLinks)

	// JSON-LD
	recipeSchema := schemaGen.GenerateRecipeSchema(e, entityURL)

	// Fix pairing names from slugMap
	if related, ok := recipeSchema["isRelatedTo"].([]map[string]interface{}); ok {
		for i, r := range related {
			if slug, ok := r["name"].(string); ok {
				if paired, ok := slugMap[slug]; ok {
					related[i]["name"] = paired.GetString("title")
				}
			}
		}
	}

	// Breadcrumbs
	var breadcrumbs []render.Breadcrumb
	breadcrumbs = append(breadcrumbs, render.Breadcrumb{Name: "Home", URL: b.cfg.Site.BaseURL + "/"})
	if cat := e.GetString("recipe_category"); cat != "" {
		catSlug := entity.ToSlug(cat)
		breadcrumbs = append(breadcrumbs, render.Breadcrumb{
			Name: cat,
			URL:  fmt.Sprintf("%s/category/%s.html", b.cfg.Site.BaseURL, catSlug),
		})
	}
	breadcrumbs = append(breadcrumbs, render.Breadcrumb{Name: e.GetString("title"), URL: ""})

	breadcrumbSchema := schemaGen.GenerateBreadcrumbSchema(toBreadcrumbItems(breadcrumbs))

	// FAQ schema
	var faqSchema map[string]interface{}
	if faqs := e.GetFAQs(); len(faqs) > 0 {
		faqSchema = schemaGen.GenerateFAQSchema(faqs)
	}

	// Share image
	svgContent := render.GenerateEntityShareSVG(
		b.cfg.Site.Name,
		e.GetString("title"),
		e.GetString("recipe_category"),
		e.GetString("cuisine"),
		e.GetString("skill_level"),
	)
	svgFilename := e.Slug + ".svg"
	if err := b.maybeWriteShareSVG(outDir, svgFilename, svgContent); err != nil {
		log.Printf("Warning: failed to write entity share SVG for %s: %v", e.Slug, err)
	}
	imageURL := shareImageURL(b.cfg.Site.BaseURL, svgFilename)

	// Set share image on recipe schema
	recipeSchema["image"] = []string{imageURL}

	jsonLD := schema.MarshalSchemas(recipeSchema, breadcrumbSchema, faqSchema)

	title := e.GetString("title")
	description := e.GetString("description")

	// Entity profile chart data (compact format for JS)
	// Always include metrics so empty values are visible (helps diagnose API gaps)
	nodeType := e.GetString("node_type")
	profileData := map[string]interface{}{}

	profileData["lc"] = e.GetInt("line_count")

	switch nodeType {
	case "Function":
		profileData["co"] = e.GetInt("call_count")
		profileData["cb"] = e.GetInt("called_by_count")
	case "File":
		profileData["ic"] = e.GetInt("import_count")
		profileData["ib"] = e.GetInt("imported_by_count")
		profileData["fn"] = e.GetInt("function_count")
		profileData["cl"] = e.GetInt("class_count")
		profileData["tc"] = e.GetInt("type_count")
	case "Class", "Type":
		profileData["fn"] = e.GetInt("function_count")
		profileData["cb"] = e.GetInt("called_by_count")
	case "Directory":
		profileData["fc"] = e.GetInt("file_count")
		profileData["fn"] = e.GetInt("function_count")
		profileData["cl"] = e.GetInt("class_count")
	default:
		// Domain, Subdomain, etc — include whatever is available
		if v := e.GetInt("function_count"); v > 0 {
			profileData["fn"] = v
		}
		if v := e.GetInt("file_count"); v > 0 {
			profileData["fc"] = v
		}
	}

	if sl := e.GetInt("start_line"); sl > 0 {
		profileData["sl"] = sl
	}
	if el := e.GetInt("end_line"); el > 0 {
		profileData["el"] = el
	}

	// Edge type breakdown
	edgeTypes := map[string]int{}
	ic := e.GetInt("import_count")
	ibc := e.GetInt("imported_by_count")
	if ic+ibc > 0 {
		edgeTypes["imports"] = ic + ibc
	}
	co := e.GetInt("call_count")
	cbc := e.GetInt("called_by_count")
	if co+cbc > 0 {
		edgeTypes["calls"] = co + cbc
	}
	defines := e.GetInt("function_count") + e.GetInt("class_count") + e.GetInt("type_count")
	if defines > 0 {
		edgeTypes["defines"] = defines
	}
	if len(edgeTypes) > 0 {
		profileData["et"] = edgeTypes
	}

	var entityChartJSON []byte
	entityChartJSON, _ = json.Marshal(profileData)

	// Source code (read from workspace if available)
	var sourceCode, sourceLang string
	if filePath := e.GetString("file_path"); filePath != "" {
		if sl := e.GetInt("start_line"); sl > 0 {
			if el := e.GetInt("end_line"); el > 0 {
				sourceDir := b.cfg.Paths.SourceDir
				if sourceDir != "" {
					fullPath := filepath.Join(sourceDir, filePath)
					if data, err := os.ReadFile(fullPath); err == nil {
						lines := strings.Split(string(data), "\n")
						if sl <= len(lines) && el <= len(lines) {
							sourceCode = strings.Join(lines[sl-1:el], "\n")
						}
					}
				}
			}
		}
		sourceLang = e.GetString("language")
		if sourceLang == "" {
			ext := filepath.Ext(filePath)
			langMap := map[string]string{
				".js": "javascript", ".ts": "typescript", ".tsx": "typescript",
				".py": "python", ".go": "go", ".rs": "rust", ".java": "java",
				".rb": "ruby", ".php": "php", ".c": "c", ".cpp": "cpp",
				".cs": "csharp", ".swift": "swift", ".kt": "kotlin",
			}
			sourceLang = langMap[ext]
		}
	}

	ctx := render.EntityPageContext{
		Site:           b.cfg.Site,
		Entity:         e,
		Slug:           e.Slug,
		URL:            entityURL,
		CanonicalURL:   entityURL,
		Breadcrumbs:    breadcrumbs,
		Pairings:       pairings,
		Enrichment:     eData,
		AffiliateLinks: affLinks,
		CookModePrompt: cookPrompt,
		JsonLD:         toTemplateHTML(jsonLD),
		Taxonomies:     taxonomies,
		AllTaxonomies:  taxonomies,
		ValidSlugs:     validSlugs,
		Contributors:   contributors,
		ChartData:      template.JS(entityChartJSON),
		SourceCode:     sourceCode,
		SourceLang:     sourceLang,
		CTA: b.cfg.Extra.CTA,
		OG: render.OGMeta{
			Title:       title + " \u2014 " + b.cfg.Site.Name,
			Description: description,
			URL:         entityURL,
			ImageURL:    imageURL,
			Type:        "article",
			SiteName:    b.cfg.Site.Name,
		},
	}

	html, err := engine.RenderEntity(ctx)
	if err != nil {
		return err
	}

	outPath := filepath.Join(outDir, e.Slug+".html")
	if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}

	addSitemapEntry("/"+e.Slug+".html",
		b.cfg.Sitemap.Priorities["entity"],
		b.cfg.Sitemap.ChangeFreqs["entity"])

	return nil
}

func (b *Builder) renderTaxonomyPages(
	tax taxonomy.Taxonomy,
	engine *render.Engine,
	schemaGen *schema.Generator,
	allTaxonomies []taxonomy.Taxonomy,
	contributors map[string]interface{},
	outDir string,
	addSitemapEntry func(string, string, string),
	today string,
) error {
	// Ensure taxonomy type directory exists
	taxDir := filepath.Join(outDir, tax.Name)
	if err := os.MkdirAll(taxDir, 0755); err != nil {
		return fmt.Errorf("creating taxonomy dir: %w", err)
	}

	perPage := b.cfg.Pagination.EntitiesPerPage

	// Render hub pages for each entry
	for _, entry := range tax.Entries {
		totalPages := (len(entry.Entities) + perPage - 1) / perPage
		if totalPages == 0 {
			totalPages = 1
		}

		// Hub share image (generate once per entry, reuse for all pages)
		typeDist := countFieldDistribution(entry.Entities, "recipe_category", 8)
		hubSVGFilename := fmt.Sprintf("%s-%s.svg", tax.Name, entry.Slug)
		hubImageURL := shareImageURL(b.cfg.Site.BaseURL, hubSVGFilename)
		if totalPages >= 1 {
			hubSVG := render.GenerateHubShareSVG(b.cfg.Site.Name, entry.Name, tax.Label, len(entry.Entities), typeDist)
			if err := b.maybeWriteShareSVG(outDir, hubSVGFilename, hubSVG); err != nil {
				log.Printf("Warning: failed to write hub share SVG for %s/%s: %v", tax.Name, entry.Slug, err)
			}
		}

		// Hub chart data (same for all pages)
		// Build distributions: breakdown by each taxonomy field (except the current one)
		distFields := []struct {
			Key   string
			Field string
		}{
			{"node_type", "node_type"},
			{"language", "language"},
			{"domain", "domain"},
			{"extension", "extension"},
		}
		distributions := make(map[string][]render.NameCount)
		for _, df := range distFields {
			if df.Key == tax.Name {
				continue // skip the current taxonomy dimension
			}
			dist := countFieldDistribution(entry.Entities, df.Field, 8)
			if len(dist) > 0 {
				distributions[df.Key] = dist
			}
		}

		// Build topEntities: largest by line count
		type topEntity struct {
			Name  string `json:"name"`
			Type  string `json:"type"`
			Lines int    `json:"lines"`
			Slug  string `json:"slug"`
		}
		var topEnts []topEntity
		for _, e := range entry.Entities {
			lc := e.GetInt("line_count")
			if lc > 0 {
				topEnts = append(topEnts, topEntity{
					Name:  e.GetString("title"),
					Type:  e.GetString("node_type"),
					Lines: lc,
					Slug:  e.Slug,
				})
			}
		}
		sort.Slice(topEnts, func(i, j int) bool {
			return topEnts[i].Lines > topEnts[j].Lines
		})
		if len(topEnts) > 10 {
			topEnts = topEnts[:10]
		}

		type hubChart struct {
			EntryName        string                        `json:"entryName"`
			TotalEntities    int                           `json:"totalEntities"`
			TypeDistribution []render.NameCount            `json:"typeDistribution"`
			Distributions    map[string][]render.NameCount `json:"distributions"`
			TopEntities      []topEntity                   `json:"topEntities"`
		}
		hubChartJSON, _ := json.Marshal(hubChart{
			EntryName:        entry.Name,
			TotalEntities:    len(entry.Entities),
			TypeDistribution: typeDist,
			Distributions:    distributions,
			TopEntities:      topEnts,
		})

		for page := 1; page <= totalPages; page++ {
			pagination := taxonomy.ComputePagination(entry, page, perPage, tax.Name)

			// Get entities for this page
			pageEntities := entry.Entities
			if pagination.StartIndex < len(entry.Entities) {
				end := pagination.EndIndex
				if end > len(entry.Entities) {
					end = len(entry.Entities)
				}
				pageEntities = entry.Entities[pagination.StartIndex:end]
			}

			// JSON-LD
			pageURL := fmt.Sprintf("%s%s", b.cfg.Site.BaseURL, taxonomy.HubPageURL(tax.Name, entry.Slug, page))
			var items []schema.ItemListEntry
			for _, e := range pageEntities {
				items = append(items, schema.ItemListEntry{
					Name: e.GetString("title"),
					URL:  fmt.Sprintf("%s/%s.html", b.cfg.Site.BaseURL, e.Slug),
				})
			}
			collectionSchema := schemaGen.GenerateCollectionPageSchema(
				entry.Name, fmt.Sprintf("%s %s recipes", entry.Name, tax.LabelSingular),
				pageURL, items, hubImageURL,
			)

			// Breadcrumbs
			breadcrumbs := []render.Breadcrumb{
				{Name: "Home", URL: b.cfg.Site.BaseURL + "/"},
				{Name: tax.Label, URL: fmt.Sprintf("%s/%s/", b.cfg.Site.BaseURL, tax.Name)},
				{Name: entry.Name, URL: ""},
			}
			breadcrumbSchema := schemaGen.GenerateBreadcrumbSchema(toBreadcrumbItems(breadcrumbs))
			jsonLD := schema.MarshalSchemas(collectionSchema, breadcrumbSchema)

			// Contributor profile for author taxonomy
			var contributorProfile map[string]interface{}
			if tax.Name == "author" && contributors != nil {
				if profiles, ok := contributors["profiles"].(map[string]interface{}); ok {
					contributorProfile, _ = profiles[entry.Slug].(map[string]interface{})
				}
			}

			hubDesc := fmt.Sprintf("Browse %d %s %s recipes on %s.", len(entry.Entities), entry.Name, tax.LabelSingular, b.cfg.Site.Name)

			ctx := render.HubPageContext{
				Site:               b.cfg.Site,
				Taxonomy:           tax,
				Entry:              entry,
				Entities:           pageEntities,
				Pagination:         pagination,
				JsonLD:             toTemplateHTML(jsonLD),
				Breadcrumbs:        breadcrumbs,
				AllTaxonomies:      allTaxonomies,
				Contributors:       contributors,
				ContributorProfile: contributorProfile,
				OG: render.OGMeta{
					Title:       entry.Name + " \u2014 " + tax.Label + " \u2014 " + b.cfg.Site.Name,
					Description: hubDesc,
					URL:         pageURL,
					ImageURL:    hubImageURL,
					Type:        "article",
					SiteName:    b.cfg.Site.Name,
				},
				ChartData: template.JS(hubChartJSON),
				CTA:       b.cfg.Extra.CTA,
			}

			html, err := engine.RenderHub(ctx)
			if err != nil {
				return fmt.Errorf("rendering hub %s/%s page %d: %w", tax.Name, entry.Slug, page, err)
			}

			// Determine filename
			var filename string
			if page == 1 {
				filename = entry.Slug + ".html"
			} else {
				filename = fmt.Sprintf("%s-page-%d.html", entry.Slug, page)
			}

			if err := os.WriteFile(filepath.Join(taxDir, filename), []byte(html), 0644); err != nil {
				return fmt.Errorf("writing hub page: %w", err)
			}

			// Sitemap
			priority := b.cfg.Sitemap.Priorities["hub_page_1"]
			if page > 1 {
				priority = b.cfg.Sitemap.Priorities["hub_page_n"]
			}
			addSitemapEntry(fmt.Sprintf("/%s/%s", tax.Name, filename), priority, b.cfg.Sitemap.ChangeFreqs["hub"])
		}
	}

	// Render taxonomy index page
	hasLetters := len(tax.Entries) >= tax.Config.LetterPageThreshold
	letterGroups := taxonomy.GroupByLetter(tax.Entries)

	var letters []string
	for _, lg := range letterGroups {
		letters = append(letters, lg.Letter)
	}

	topEntries := taxonomy.TopEntries(tax.Entries, 12)

	// Taxonomy index share image
	var taxIndexEntries []render.NameCount
	for _, entry := range taxonomy.TopEntries(tax.Entries, 20) {
		taxIndexEntries = append(taxIndexEntries, render.NameCount{Name: entry.Name, Count: len(entry.Entities)})
	}
	taxIndexSVGFilename := fmt.Sprintf("%s-index.svg", tax.Name)
	taxIndexSVG := render.GenerateTaxIndexShareSVG(b.cfg.Site.Name, tax.Label, taxIndexEntries)
	if err := b.maybeWriteShareSVG(outDir, taxIndexSVGFilename, taxIndexSVG); err != nil {
		log.Printf("Warning: failed to write taxonomy index share SVG for %s: %v", tax.Name, err)
	}
	taxIndexImageURL := shareImageURL(b.cfg.Site.BaseURL, taxIndexSVGFilename)

	// Taxonomy index chart data
	type taxChart struct {
		TaxonomyName string             `json:"taxonomyName"`
		Entries      []render.NameCount `json:"entries"`
	}
	taxChartJSON, _ := json.Marshal(taxChart{
		TaxonomyName: tax.Label,
		Entries:      taxIndexEntries,
	})

	// Index page JSON-LD
	var indexItems []schema.ItemListEntry
	for _, entry := range tax.Entries {
		indexItems = append(indexItems, schema.ItemListEntry{
			Name: entry.Name,
			URL:  fmt.Sprintf("%s/%s/%s.html", b.cfg.Site.BaseURL, tax.Name, entry.Slug),
		})
	}
	indexURL := fmt.Sprintf("%s/%s/", b.cfg.Site.BaseURL, tax.Name)
	indexSchema := schemaGen.GenerateItemListSchema(tax.Label, fmt.Sprintf("Browse all %s", tax.Label), indexItems, taxIndexImageURL)
	breadcrumbs := []render.Breadcrumb{
		{Name: "Home", URL: b.cfg.Site.BaseURL + "/"},
		{Name: tax.Label, URL: ""},
	}
	breadcrumbSchema := schemaGen.GenerateBreadcrumbSchema(toBreadcrumbItems(breadcrumbs))
	jsonLD := schema.MarshalSchemas(indexSchema, breadcrumbSchema)

	ctx := render.TaxonomyIndexContext{
		Site:          b.cfg.Site,
		Taxonomy:      tax,
		Entries:       tax.Entries,
		TopEntries:    topEntries,
		LetterGroups:  letterGroups,
		HasLetters:    hasLetters,
		Letters:       letters,
		JsonLD:        toTemplateHTML(jsonLD),
		Breadcrumbs:   breadcrumbs,
		AllTaxonomies: allTaxonomies,
		OG: render.OGMeta{
			Title:       tax.Label + " \u2014 " + b.cfg.Site.Name,
			Description: tax.Config.IndexDescription,
			URL:         indexURL,
			ImageURL:    taxIndexImageURL,
			Type:        "article",
			SiteName:    b.cfg.Site.Name,
		},
		ChartData: template.JS(taxChartJSON),
		CTA:       b.cfg.Extra.CTA,
	}

	html, err := engine.RenderTaxonomyIndex(ctx)
	if err != nil {
		return fmt.Errorf("rendering taxonomy index %s: %w", tax.Name, err)
	}

	if err := os.WriteFile(filepath.Join(taxDir, "index.html"), []byte(html), 0644); err != nil {
		return fmt.Errorf("writing taxonomy index: %w", err)
	}
	addSitemapEntry(fmt.Sprintf("/%s/", tax.Name), b.cfg.Sitemap.Priorities["taxonomy_index"], b.cfg.Sitemap.ChangeFreqs["taxonomy_index"])

	// Render letter pages if threshold met
	if hasLetters {
		for _, lg := range letterGroups {
			// Letter share image
			letterSlug := strings.ToLower(lg.Letter)
			if lg.Letter == "#" {
				letterSlug = "num"
			}
			letterSVGFilename := fmt.Sprintf("%s-letter-%s.svg", tax.Name, letterSlug)
			letterSVG := render.GenerateLetterShareSVG(b.cfg.Site.Name, tax.Label, lg.Letter, len(lg.Entries))
			if err := b.maybeWriteShareSVG(outDir, letterSVGFilename, letterSVG); err != nil {
				log.Printf("Warning: failed to write letter share SVG for %s/%s: %v", tax.Name, lg.Letter, err)
			}
			letterImageURL := shareImageURL(b.cfg.Site.BaseURL, letterSVGFilename)

			// Letter chart data
			var letterEntries []render.NameCount
			limit := 15
			if len(lg.Entries) < limit {
				limit = len(lg.Entries)
			}
			for _, e := range lg.Entries[:limit] {
				letterEntries = append(letterEntries, render.NameCount{Name: e.Name, Count: len(e.Entities)})
			}
			type letterChart struct {
				Letter       string             `json:"letter"`
				TaxonomyName string             `json:"taxonomyName"`
				Entries      []render.NameCount `json:"entries"`
			}
			letterChartJSON, _ := json.Marshal(letterChart{
				Letter:       lg.Letter,
				TaxonomyName: tax.Label,
				Entries:      letterEntries,
			})

			letterFile := fmt.Sprintf("letter-%s.html", letterSlug)
			letterPageURL := fmt.Sprintf("%s/%s/%s", b.cfg.Site.BaseURL, tax.Name, letterFile)

			letterBreadcrumbs := []render.Breadcrumb{
				{Name: "Home", URL: b.cfg.Site.BaseURL + "/"},
				{Name: tax.Label, URL: fmt.Sprintf("%s/%s/", b.cfg.Site.BaseURL, tax.Name)},
				{Name: fmt.Sprintf("Letter %s", lg.Letter), URL: ""},
			}

			letterCtx := render.LetterPageContext{
				Site:          b.cfg.Site,
				Taxonomy:      tax,
				Letter:        lg.Letter,
				Entries:       lg.Entries,
				Letters:       letters,
				Breadcrumbs:   letterBreadcrumbs,
				AllTaxonomies: allTaxonomies,
				OG: render.OGMeta{
					Title:       fmt.Sprintf("%s \u2014 Letter %s \u2014 %s", tax.Label, lg.Letter, b.cfg.Site.Name),
					Description: fmt.Sprintf("Browse %s starting with %s on %s.", tax.Label, lg.Letter, b.cfg.Site.Name),
					URL:         letterPageURL,
					ImageURL:    letterImageURL,
					Type:        "article",
					SiteName:    b.cfg.Site.Name,
				},
				ChartData: template.JS(letterChartJSON),
				CTA:       b.cfg.Extra.CTA,
			}

			letterHTML, err := engine.RenderLetter(letterCtx)
			if err != nil {
				return fmt.Errorf("rendering letter page %s/%s: %w", tax.Name, lg.Letter, err)
			}

			if err := os.WriteFile(filepath.Join(taxDir, letterFile), []byte(letterHTML), 0644); err != nil {
				return fmt.Errorf("writing letter page: %w", err)
			}
			addSitemapEntry(fmt.Sprintf("/%s/%s", tax.Name, letterFile),
				b.cfg.Sitemap.Priorities["letter_page"],
				b.cfg.Sitemap.ChangeFreqs["letter_page"])
		}
	}

	return nil
}

func (b *Builder) renderAllEntitiesPages(
	engine *render.Engine,
	schemaGen *schema.Generator,
	entities []*entity.Entity,
	allTaxonomies []taxonomy.Taxonomy,
	outDir string,
	addSitemapEntry func(string, string, string),
) error {
	// Ensure all/ directory exists
	allDir := filepath.Join(outDir, "all")
	if err := os.MkdirAll(allDir, 0755); err != nil {
		return fmt.Errorf("creating all dir: %w", err)
	}

	// Global type distribution
	typeDist := countFieldDistribution(entities, "recipe_category", 10)

	// Share image (once)
	allSVG := render.GenerateAllEntitiesShareSVG(b.cfg.Site.Name, len(entities), typeDist)
	if err := b.maybeWriteShareSVG(outDir, "all-entities.svg", allSVG); err != nil {
		log.Printf("Warning: failed to write all-entities share SVG: %v", err)
	}
	imageURL := shareImageURL(b.cfg.Site.BaseURL, "all-entities.svg")

	// Chart data
	type allChart struct {
		TotalEntities    int                `json:"totalEntities"`
		TypeDistribution []render.NameCount `json:"typeDistribution"`
	}
	chartJSON, _ := json.Marshal(allChart{
		TotalEntities:    len(entities),
		TypeDistribution: typeDist,
	})

	perPage := b.cfg.Pagination.EntitiesPerPage
	totalPages := (len(entities) + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}

	for page := 1; page <= totalPages; page++ {
		start := (page - 1) * perPage
		end := start + perPage
		if end > len(entities) {
			end = len(entities)
		}
		pageEntities := entities[start:end]

		// Build pagination info
		pagination := taxonomy.PaginationInfo{
			CurrentPage: page,
			TotalPages:  totalPages,
			TotalItems:  len(entities),
			StartIndex:  start,
			EndIndex:    end,
		}
		for p := 1; p <= totalPages; p++ {
			url := "/all/index.html"
			if p > 1 {
				url = fmt.Sprintf("/all/page-%d.html", p)
			}
			pagination.PageURLs = append(pagination.PageURLs, taxonomy.PageURL{Number: p, URL: url})
		}
		if page > 1 {
			if page == 2 {
				pagination.PrevURL = "/all/index.html"
			} else {
				pagination.PrevURL = fmt.Sprintf("/all/page-%d.html", page-1)
			}
		}
		if page < totalPages {
			pagination.NextURL = fmt.Sprintf("/all/page-%d.html", page+1)
		}

		pageURL := fmt.Sprintf("%s/all/index.html", b.cfg.Site.BaseURL)
		if page > 1 {
			pageURL = fmt.Sprintf("%s/all/page-%d.html", b.cfg.Site.BaseURL, page)
		}

		// JSON-LD
		var items []schema.ItemListEntry
		for _, e := range pageEntities {
			items = append(items, schema.ItemListEntry{
				Name: e.GetString("title"),
				URL:  fmt.Sprintf("%s/%s.html", b.cfg.Site.BaseURL, e.Slug),
			})
		}
		collectionSchema := schemaGen.GenerateCollectionPageSchema(
			"All Recipes",
			fmt.Sprintf("Browse all %d recipes on %s", len(entities), b.cfg.Site.Name),
			pageURL, items, imageURL,
		)
		breadcrumbs := []render.Breadcrumb{
			{Name: "Home", URL: b.cfg.Site.BaseURL + "/"},
			{Name: "All Recipes", URL: ""},
		}
		breadcrumbSchema := schemaGen.GenerateBreadcrumbSchema(toBreadcrumbItems(breadcrumbs))
		jsonLD := schema.MarshalSchemas(collectionSchema, breadcrumbSchema)

		// Only include chart data on page 1
		var pageChartData template.JS
		if page == 1 {
			pageChartData = template.JS(chartJSON)
		}

		ctx := render.AllEntitiesPageContext{
			Site:          b.cfg.Site,
			Entities:      pageEntities,
			Pagination:    pagination,
			JsonLD:        toTemplateHTML(jsonLD),
			Breadcrumbs:   breadcrumbs,
			AllTaxonomies: allTaxonomies,
			EntityCount:   len(entities),
			TotalEntities: len(entities),
			OG: render.OGMeta{
				Title:       "All Recipes \u2014 " + b.cfg.Site.Name,
				Description: fmt.Sprintf("Browse all %d recipes on %s.", len(entities), b.cfg.Site.Name),
				URL:         pageURL,
				ImageURL:    imageURL,
				Type:        "article",
				SiteName:    b.cfg.Site.Name,
			},
			ChartData: pageChartData,
			CTA:       b.cfg.Extra.CTA,
		}

		html, err := engine.RenderAllEntities(ctx)
		if err != nil {
			return fmt.Errorf("rendering all-entities page %d: %w", page, err)
		}

		filename := "index.html"
		if page > 1 {
			filename = fmt.Sprintf("page-%d.html", page)
		}

		if err := os.WriteFile(filepath.Join(allDir, filename), []byte(html), 0644); err != nil {
			return fmt.Errorf("writing all-entities page: %w", err)
		}

		addSitemapEntry(fmt.Sprintf("/all/%s", filename), "0.5", "weekly")
	}

	return nil
}

func (b *Builder) renderHomepage(
	engine *render.Engine,
	schemaGen *schema.Generator,
	entities []*entity.Entity,
	taxonomies []taxonomy.Taxonomy,
	favorites []*entity.Entity,
	contributors map[string]interface{},
	outDir string,
) error {
	// Share image
	var taxStats []render.NameCount
	for _, tax := range taxonomies {
		taxStats = append(taxStats, render.NameCount{Name: tax.Label, Count: len(tax.Entries)})
	}
	svgContent := render.GenerateHomepageShareSVG(b.cfg.Site.Name, b.cfg.Site.Description, taxStats, len(entities))
	if err := b.maybeWriteShareSVG(outDir, "homepage.svg", svgContent); err != nil {
		log.Printf("Warning: failed to write homepage share SVG: %v", err)
	}
	imageURL := shareImageURL(b.cfg.Site.BaseURL, "homepage.svg")

	// Chart data: treemap of taxonomies
	type chartTax struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
		Slug  string `json:"slug"`
	}
	type homepageChart struct {
		Taxonomies    []chartTax `json:"taxonomies"`
		TotalEntities int        `json:"totalEntities"`
	}
	var chartTaxonomies []chartTax
	for _, tax := range taxonomies {
		totalCount := 0
		for _, entry := range tax.Entries {
			totalCount += len(entry.Entities)
		}
		chartTaxonomies = append(chartTaxonomies, chartTax{
			Name:  tax.Label,
			Count: totalCount,
			Slug:  tax.Name,
		})
	}
	chartJSON, _ := json.Marshal(homepageChart{Taxonomies: chartTaxonomies, TotalEntities: len(entities)})

	// Architecture overview: domain/subdomain force graph
	type archNode struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Type  string `json:"type"`
		Count int    `json:"count"`
		Slug  string `json:"slug,omitempty"`
	}
	type archLink struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	type archOverview struct {
		Nodes []archNode `json:"nodes"`
		Links []archLink `json:"links"`
	}

	var archNodes []archNode
	var archLinks []archLink

	// Root node is the repo/site
	rootID := "__root__"
	archNodes = append(archNodes, archNode{ID: rootID, Name: b.cfg.Site.Name, Type: "root", Count: len(entities)})

	// Find subdomain -> domain parent relationships
	subdomainParent := make(map[string]string) // subdomain name -> domain name
	for _, tax := range taxonomies {
		if tax.Name == "subdomain" {
			for _, entry := range tax.Entries {
				parentDomain := ""
				if len(entry.Entities) > 0 {
					parentDomain = entry.Entities[0].GetString("domain")
				}
				subdomainParent[entry.Name] = parentDomain
			}
		}
	}

	// Add domain nodes
	for _, tax := range taxonomies {
		if tax.Name == "domain" {
			for _, entry := range tax.Entries {
				nodeID := "domain:" + entry.Slug
				archNodes = append(archNodes, archNode{
					ID:    nodeID,
					Name:  entry.Name,
					Type:  "domain",
					Count: len(entry.Entities),
					Slug:  "domain/" + entry.Slug,
				})
				archLinks = append(archLinks, archLink{Source: rootID, Target: nodeID})
			}
		}
	}
	// Add subdomain nodes
	for _, tax := range taxonomies {
		if tax.Name == "subdomain" {
			for _, entry := range tax.Entries {
				nodeID := "subdomain:" + entry.Slug
				archNodes = append(archNodes, archNode{
					ID:    nodeID,
					Name:  entry.Name,
					Type:  "subdomain",
					Count: len(entry.Entities),
					Slug:  "subdomain/" + entry.Slug,
				})
				parentDomain := subdomainParent[entry.Name]
				if parentDomain != "" {
					parentSlug := entity.ToSlug(parentDomain)
					archLinks = append(archLinks, archLink{Source: "domain:" + parentSlug, Target: nodeID})
				} else {
					archLinks = append(archLinks, archLink{Source: rootID, Target: nodeID})
				}
			}
		}
	}

	var archJSON []byte
	if len(archNodes) > 1 {
		archJSON, _ = json.Marshal(archOverview{Nodes: archNodes, Links: archLinks})
	}

	// JSON-LD
	websiteSchema := schemaGen.GenerateWebSiteSchema(imageURL)

	var items []schema.ItemListEntry
	for _, e := range entities {
		items = append(items, schema.ItemListEntry{
			Name: e.GetString("title"),
			URL:  fmt.Sprintf("%s/%s.html", b.cfg.Site.BaseURL, e.Slug),
		})
	}
	itemListSchema := schemaGen.GenerateItemListSchema(
		b.cfg.Site.Name,
		b.cfg.Site.Description,
		items,
		imageURL,
	)

	jsonLD := schema.MarshalSchemas(websiteSchema, itemListSchema)

	ctx := render.HomepageContext{
		Site:         b.cfg.Site,
		Entities:     entities,
		Taxonomies:   taxonomies,
		Favorites:    favorites,
		JsonLD:       toTemplateHTML(jsonLD),
		EntityCount:  len(entities),
		Contributors: contributors,
		OG: render.OGMeta{
			Title:       b.cfg.Site.Name,
			Description: b.cfg.Site.Description,
			URL:         b.cfg.Site.BaseURL + "/",
			ImageURL:    imageURL,
			Type:        "website",
			SiteName:    b.cfg.Site.Name,
		},
		ChartData: template.JS(chartJSON),
		ArchData:  template.JS(archJSON),
		CTA:       b.cfg.Extra.CTA,
	}

	html, err := engine.RenderHomepage(ctx)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(outDir, "index.html"), []byte(html), 0644)
}

func (b *Builder) loadFavorites(slugMap map[string]*entity.Entity) []*entity.Entity {
	if b.cfg.Extra.Favorites == "" {
		return nil
	}

	data, err := os.ReadFile(b.cfg.Extra.Favorites)
	if err != nil {
		log.Printf("Warning: failed to load favorites: %v", err)
		return nil
	}

	var slugs []string
	if err := json.Unmarshal(data, &slugs); err != nil {
		log.Printf("Warning: failed to parse favorites: %v", err)
		return nil
	}

	var result []*entity.Entity
	for _, slug := range slugs {
		if e, ok := slugMap[slug]; ok {
			result = append(result, e)
		}
	}
	return result
}

func (b *Builder) loadContributors() map[string]interface{} {
	if b.cfg.Extra.Contributors == "" {
		return nil
	}

	data, err := os.ReadFile(b.cfg.Extra.Contributors)
	if err != nil {
		log.Printf("Warning: failed to load contributors: %v", err)
		return nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		log.Printf("Warning: failed to parse contributors: %v", err)
		return nil
	}
	return result
}

func toBreadcrumbItems(breadcrumbs []render.Breadcrumb) []schema.BreadcrumbItem {
	items := make([]schema.BreadcrumbItem, len(breadcrumbs))
	for i, bc := range breadcrumbs {
		items[i] = schema.BreadcrumbItem{Name: bc.Name, URL: bc.URL}
	}
	return items
}

// toTemplateHTML converts a string to template.HTML (trusted HTML).
func toTemplateHTML(s string) template.HTML {
	return template.HTML(s)
}

func countTaxEntries(taxonomies []taxonomy.Taxonomy) int {
	total := 0
	for _, tax := range taxonomies {
		total += len(tax.Entries)
	}
	return total
}

// countFieldDistribution counts occurrences of a string field across entities,
// returns sorted desc, capped at limit.
func countFieldDistribution(entities []*entity.Entity, field string, limit int) []render.NameCount {
	counts := make(map[string]int)
	for _, e := range entities {
		val := e.GetString(field)
		if val != "" {
			counts[val]++
		}
	}

	var result []render.NameCount
	for name, count := range counts {
		result = append(result, render.NameCount{Name: name, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

// writeShareSVG writes an SVG share image to the images/share/ directory.
func writeShareSVG(outDir, filename, svg string) error {
	dir := filepath.Join(outDir, "images", "share")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, filename), []byte(svg), 0644)
}

// maybeWriteShareSVG skips SVG generation when output.share_images is false.
func (b *Builder) maybeWriteShareSVG(outDir, filename, svg string) error {
	if !b.cfg.Output.ShareImages {
		return nil
	}
	return writeShareSVG(outDir, filename, svg)
}

// shareImageURL returns the full URL for a share image.
func shareImageURL(baseURL, filename string) string {
	return fmt.Sprintf("%s/images/share/%s", baseURL, filename)
}

type searchEntry struct {
	T string `json:"t"`           // title
	D string `json:"d,omitempty"` // description (truncated)
	S string `json:"s"`           // slug
	N string `json:"n,omitempty"` // node_type
	L string `json:"l,omitempty"` // language
	M string `json:"m,omitempty"` // domain
}

func (b *Builder) generateSearchIndex(entities []*entity.Entity, outDir string) error {
	if !b.cfg.Search.Enabled {
		return nil
	}

	entries := make([]searchEntry, 0, len(entities))
	for _, e := range entities {
		desc := e.GetString("description")
		if len(desc) > 120 {
			desc = desc[:120]
		}
		entries = append(entries, searchEntry{
			T: e.GetString("title"),
			D: desc,
			S: e.Slug,
			N: e.GetString("node_type"),
			L: e.GetString("language"),
			M: e.GetString("domain"),
		})
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}

	outPath := filepath.Join(outDir, "search-index.json")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return err
	}
	log.Printf("  Generated search index (%d entries, %dKB)", len(entries), len(data)/1024)
	return nil
}

// copyDir copies files from src to dst directory.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
