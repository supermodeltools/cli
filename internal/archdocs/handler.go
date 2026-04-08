// Package archdocs generates static architecture documentation for a repository
// by uploading it to the Supermodel API, converting the returned graph to
// markdown via graph2md, and building a static HTML site with pssg.
package archdocs

import (
	"context"
	"embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"encoding/json"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/archdocs/graph2md"
	pssgbuild "github.com/supermodeltools/cli/internal/archdocs/pssg/build"
	pssgconfig "github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/build"
	"github.com/supermodeltools/cli/internal/cache"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

//go:embed templates/*
var bundledTemplates embed.FS

// Options configures the arch-docs command.
type Options struct {
	// SiteName is the display title for the generated site.
	// Defaults to "<repo> Architecture Docs".
	SiteName string

	// BaseURL is the canonical base URL where the site will be hosted.
	// Defaults to "https://example.com".
	BaseURL string

	// Repo is the "owner/repo" GitHub slug used to build a repo URL and
	// derive defaults. Optional – inferred from git remote when empty.
	Repo string

	// Output is the directory to write the generated site into.
	// Defaults to "./arch-docs-output".
	Output string

	// TemplatesDir overrides the bundled HTML/CSS/JS templates.
	TemplatesDir string

	// MaxSourceFiles caps how many source files are included in the archive
	// sent to the API. 0 means unlimited.
	MaxSourceFiles int

	// MaxEntities caps how many entity pages are generated. 0 means unlimited.
	MaxEntities int

	// Force bypasses the analysis cache and re-uploads even if cached.
	Force bool
}

// pssgConfigTemplate is the YAML configuration template for the pssg static
// site generator. Placeholders (in order): site.name, site.base_url,
// site.repo_url, site.description (repo name), paths.data, paths.templates,
// paths.output, paths.source_dir, rss.title, rss.description (repo name),
// llms_txt.title, llms_txt.description (repo name).
const pssgConfigTemplate = `site:
  name: "%s"
  base_url: "%s"
  repo_url: "%s"
  description: "Architecture documentation for the %s codebase. Explore files, functions, classes, domains, and dependencies."
  author: "Supermodel"
  language: "en"

paths:
  data: "%s"
  templates: "%s"
  output: "%s"
  source_dir: "%s"

data:
  format: "markdown"
  entity_type: "entity"
  entity_slug:
    source: "filename"
  body_sections:
    - name: "Functions"
      header: "Functions"
      type: "unordered_list"
    - name: "Classes"
      header: "Classes"
      type: "unordered_list"
    - name: "Types"
      header: "Types"
      type: "unordered_list"
    - name: "Dependencies"
      header: "Dependencies"
      type: "unordered_list"
    - name: "Imported By"
      header: "Imported By"
      type: "unordered_list"
    - name: "Calls"
      header: "Calls"
      type: "unordered_list"
    - name: "Called By"
      header: "Called By"
      type: "unordered_list"
    - name: "Source Files"
      header: "Source Files"
      type: "unordered_list"
    - name: "Subdirectories"
      header: "Subdirectories"
      type: "unordered_list"
    - name: "Files"
      header: "Files"
      type: "unordered_list"
    - name: "Source"
      header: "Source"
      type: "unordered_list"
    - name: "Extends"
      header: "Extends"
      type: "unordered_list"
    - name: "Defined In"
      header: "Defined In"
      type: "unordered_list"
    - name: "Subdomains"
      header: "Subdomains"
      type: "unordered_list"
    - name: "Domain"
      header: "Domain"
      type: "unordered_list"
    - name: "faqs"
      header: "FAQs"
      type: "faq"

taxonomies:
  - name: "node_type"
    label: "Node Types"
    label_singular: "Node Type"
    field: "node_type"
    multi_value: false
    min_entities: 1
    index_description: "Browse by entity type"

  - name: "language"
    label: "Languages"
    label_singular: "Language"
    field: "language"
    multi_value: false
    min_entities: 1
    index_description: "Browse by programming language"

  - name: "domain"
    label: "Domains"
    label_singular: "Domain"
    field: "domain"
    multi_value: false
    min_entities: 1
    index_description: "Browse by architectural domain"

  - name: "subdomain"
    label: "Subdomains"
    label_singular: "Subdomain"
    field: "subdomain"
    multi_value: false
    min_entities: 1
    index_description: "Browse by architectural subdomain"

  - name: "top_directory"
    label: "Top Directories"
    label_singular: "Directory"
    field: "top_directory"
    multi_value: false
    min_entities: 1
    index_description: "Browse by top-level directory"

  - name: "extension"
    label: "File Extensions"
    label_singular: "Extension"
    field: "extension"
    multi_value: false
    min_entities: 1
    index_description: "Browse by file extension"

  - name: "tags"
    label: "Tags"
    label_singular: "Tag"
    field: "tags"
    multi_value: true
    min_entities: 1
    index_description: "Browse by tag"

pagination:
  per_page: 48
  url_pattern: "/{taxonomy}/{entry}/{page}"

structured_data:
  entity_type: "SoftwareSourceCode"
  field_mappings:
    name: "title"
    description: "description"
    programmingLanguage: "language"
    codeRepository: "repo_url"

sitemap:
  enabled: true
  max_urls_per_file: 50000
  priorities:
    homepage: 1.0
    entity: 0.8
    taxonomy_index: 0.7
    hub_page_1: 0.6
    hub_page_n: 0.4

rss:
  enabled: true
  title: "%s"
  description: "Architecture documentation for the %s codebase"

robots:
  enabled: true

llms_txt:
  enabled: true
  title: "%s"
  description: "Architecture documentation for the %s codebase"

search:
  enabled: true

templates:
  entity: "entity.html"
  homepage: "index.html"
  hub: "hub.html"
  taxonomy_index: "taxonomy_index.html"
  all_entities: "all_entities.html"

output:
  clean_before_build: true
  extract_css: "styles.css"
  extract_js: "main.js"
  share_images: false

extra:
  cta:
    enabled: true
    heading: "Analyze Your Own Codebase"
    description: "Get architecture documentation, dependency graphs, and domain analysis for your codebase in minutes."
    button_text: "Try Supermodel Free"
    button_url: "https://dashboard.supermodeltools.com/billing/"
`

// Run generates architecture documentation for the repository at dir.
func Run(ctx context.Context, cfg *config.Config, dir string, opts Options) error { //nolint:gocyclo,gocritic // sequential pipeline; opts is a value-semantic config struct
	// Resolve absolute path
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Derive repo name / URL from opts.Repo or directory name
	repoName, repoURL := deriveRepoInfo(opts.Repo, absDir)

	// Apply defaults
	if opts.SiteName == "" {
		if repoName != "" {
			opts.SiteName = repoName + " Architecture Docs"
		} else {
			opts.SiteName = "Architecture Docs"
		}
	}
	if opts.BaseURL == "" {
		opts.BaseURL = "https://example.com"
	}
	if opts.Output == "" {
		opts.Output = filepath.Join(absDir, "arch-docs-output")
	} else if !filepath.IsAbs(opts.Output) {
		opts.Output = filepath.Join(absDir, opts.Output)
	}
	if opts.MaxSourceFiles == 0 {
		opts.MaxSourceFiles = 3000
	}
	if opts.MaxEntities == 0 {
		opts.MaxEntities = 12000
	}

	rawResult, err := analyzeOrCachedRaw(ctx, cfg, absDir, opts.Force)
	if err != nil {
		return err
	}

	// Write raw graph JSON to a temp file for graph2md
	tmpDir, err := os.MkdirTemp("", "supermodel-archdocs-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	graphPath := filepath.Join(tmpDir, "graph.json")
	if err := os.WriteFile(graphPath, rawResult, 0o600); err != nil {
		return fmt.Errorf("write graph JSON: %w", err)
	}

	// Convert graph → markdown
	ui.Step("Generating markdown from graph…")
	contentDir := filepath.Join(tmpDir, "content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		return fmt.Errorf("create content dir: %w", err)
	}
	if err := graph2md.Run(graphPath, contentDir, repoName, repoURL, opts.MaxEntities); err != nil {
		return fmt.Errorf("graph2md: %w", err)
	}
	entityCount := countFiles(contentDir, ".md")
	ui.Success("Generated %d entity pages", entityCount)

	// Resolve templates directory (bundled or user-supplied)
	tplDir, tplCleanup, err := resolveTemplates(opts.TemplatesDir)
	if err != nil {
		return fmt.Errorf("resolve templates: %w", err)
	}
	if tplCleanup != nil {
		defer tplCleanup()
	}

	// Write pssg.yaml and build the static site
	ui.Step("Building static site…")
	configPath := filepath.Join(tmpDir, "pssg.yaml")
	if err := writePssgConfig(configPath, opts.SiteName, opts.BaseURL, repoURL, repoName, contentDir, tplDir, opts.Output, absDir); err != nil {
		return fmt.Errorf("write pssg config: %w", err)
	}

	pssgCfg, err := pssgconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load pssg config: %w", err)
	}
	builder := pssgbuild.NewBuilder(pssgCfg, false)
	if err := builder.Build(); err != nil {
		return fmt.Errorf("pssg build: %w", err)
	}

	// Rewrite root-relative paths if base URL has a path prefix
	if prefix := extractPathPrefix(opts.BaseURL); prefix != "" {
		if err := rewritePathPrefix(opts.Output, prefix); err != nil {
			return fmt.Errorf("rewrite paths: %w", err)
		}
	}

	pageCount := countFiles(opts.Output, ".html")
	ui.Success("Site built: %d pages → %s", pageCount, opts.Output)
	fmt.Fprintf(os.Stdout, "\n  entities : %s\n  pages    : %s\n  output   : %s\n\n",
		strconv.Itoa(entityCount), strconv.Itoa(pageCount), opts.Output)
	return nil
}

// deriveRepoInfo extracts a short repo name and GitHub URL from a
// "owner/repo" slug. Falls back to the directory base name.
func deriveRepoInfo(repoSlug, dir string) (name, repoURL string) {
	if repoSlug != "" {
		parts := strings.SplitN(repoSlug, "/", 2)
		if len(parts) == 2 {
			return parts[1], "https://github.com/" + repoSlug
		}
		return repoSlug, ""
	}
	return filepath.Base(dir), ""
}

// resolveTemplates returns the path to the templates directory. If override is
// non-empty it is used directly. Otherwise the bundled templates are extracted
// to a temporary directory and a cleanup function is returned.
func resolveTemplates(override string) (dir string, cleanup func(), err error) {
	if override != "" {
		return override, nil, nil
	}

	tmp, err := os.MkdirTemp("", "supermodel-templates-*")
	if err != nil {
		return "", nil, err
	}

	entries, err := bundledTemplates.ReadDir("templates")
	if err != nil {
		os.RemoveAll(tmp)
		return "", nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := bundledTemplates.ReadFile("templates/" + e.Name())
		if err != nil {
			os.RemoveAll(tmp)
			return "", nil, err
		}
		if err := os.WriteFile(filepath.Join(tmp, e.Name()), data, 0o600); err != nil {
			os.RemoveAll(tmp)
			return "", nil, err
		}
	}

	return tmp, func() { os.RemoveAll(tmp) }, nil
}

// writePssgConfig generates a pssg.yaml configuration file from the template.
func writePssgConfig(path, siteName, baseURL, repoURL, repoName, contentDir, tplDir, outputDir, sourceDir string) error {
	content := fmt.Sprintf(pssgConfigTemplate,
		siteName,   // site.name
		baseURL,    // site.base_url
		repoURL,    // site.repo_url
		repoName,   // site.description
		contentDir, // paths.data
		tplDir,     // paths.templates
		outputDir,  // paths.output
		sourceDir,  // paths.source_dir
		siteName,   // rss.title
		repoName,   // rss.description
		siteName,   // llms_txt.title
		repoName,   // llms_txt.description
	)
	return os.WriteFile(path, []byte(content), 0o600)
}

// extractPathPrefix returns the path component of a URL (e.g. "/myrepo" from
// "https://org.github.io/myrepo"). Returns "" if there is no path prefix.
func extractPathPrefix(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	p := strings.TrimRight(u.Path, "/")
	if p == "" || p == "/" {
		return ""
	}
	return p
}

// rewritePathPrefix rewrites root-relative paths in HTML and JS files to
// include prefix. Required for subdirectory deployments (e.g. GitHub Pages).
func rewritePathPrefix(dir, prefix string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".html" && ext != ".js" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		original := content
		content = strings.ReplaceAll(content, `href="/`, `href="`+prefix+`/`)
		content = strings.ReplaceAll(content, `src="/`, `src="`+prefix+`/`)
		content = strings.ReplaceAll(content, `fetch("/`, `fetch("`+prefix+`/`)
		content = strings.ReplaceAll(content, `window.location.href = "/"`, `window.location.href = "`+prefix+`/"`)
		content = strings.ReplaceAll(content, `window.location.href = "/" + `, `window.location.href = "`+prefix+`/" + `)
		if content != original {
			if err := os.WriteFile(path, []byte(content), info.Mode()); err != nil {
				return fmt.Errorf("writing %s: %w", path, err)
			}
		}
		return nil
	})
}

// countFiles counts files with the given extension under dir.
func countFiles(dir, ext string) int {
	count := 0
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ext) {
			count++
		}
		return nil
	})
	return count
}

// analyzeOrCachedRaw returns the raw JSON result from a repository analysis,
// hitting the fingerprint cache first to avoid re-uploading unchanged repos.
func analyzeOrCachedRaw(ctx context.Context, cfg *config.Config, repoDir string, force bool) (json.RawMessage, error) {
	if !force {
		if fp, err := cache.RepoFingerprint(repoDir); err == nil {
			key := cache.AnalysisKey(fp, "archdocs", build.Version)
			var cached json.RawMessage
			if hit, _ := cache.GetJSON(key, &cached); hit {
				ui.Success("Using cached analysis")
				return cached, nil
			}
		}
	}

	ui.Step("Creating repository archive…")
	zipPath, err := createZip(repoDir)
	if err != nil {
		return nil, fmt.Errorf("create archive: %w", err)
	}
	defer os.Remove(zipPath)

	hash, err := cache.HashFile(zipPath)
	if err != nil {
		return nil, fmt.Errorf("hash archive: %w", err)
	}

	client := api.New(cfg)
	spin := ui.Start("Uploading and analyzing repository…")
	raw, err := client.AnalyzeRaw(ctx, zipPath, "archdocs-"+hash[:16])
	spin.Stop()
	if err != nil {
		return nil, fmt.Errorf("API analysis: %w", err)
	}
	ui.Success("Analysis complete")

	if fp, err := cache.RepoFingerprint(repoDir); err == nil {
		key := cache.AnalysisKey(fp, "archdocs", build.Version)
		_ = cache.PutJSON(key, raw)
	}
	return raw, nil
}
