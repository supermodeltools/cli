package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
)

// ── GenerateRobotsTxt ─────────────────────────────────────────────────────────

func TestGenerateRobotsTxt_AllowAll(t *testing.T) {
	cfg := &config.Config{
		Site:   config.SiteConfig{BaseURL: "https://example.com"},
		Robots: config.RobotsConfig{AllowAll: true},
	}
	got := GenerateRobotsTxt(cfg)
	if !strings.Contains(got, "User-agent: *") {
		t.Error("should contain wildcard user-agent")
	}
	if !strings.Contains(got, "Allow: /") {
		t.Error("should contain Allow: /")
	}
	if !strings.Contains(got, "Sitemap: https://example.com/sitemap.xml") {
		t.Errorf("should contain sitemap URL, got:\n%s", got)
	}
}

func TestGenerateRobotsTxt_StandardBots(t *testing.T) {
	cfg := &config.Config{
		Site: config.SiteConfig{BaseURL: "https://example.com"},
	}
	got := GenerateRobotsTxt(cfg)
	if !strings.Contains(got, "User-agent: Googlebot") {
		t.Error("should include Googlebot")
	}
	if !strings.Contains(got, "User-agent: Bingbot") {
		t.Error("should include Bingbot")
	}
}

func TestGenerateRobotsTxt_ExtraBots(t *testing.T) {
	cfg := &config.Config{
		Site:   config.SiteConfig{BaseURL: "https://example.com"},
		Robots: config.RobotsConfig{ExtraBots: []string{"GPTBot", "ClaudeBot"}},
	}
	got := GenerateRobotsTxt(cfg)
	if !strings.Contains(got, "User-agent: GPTBot") {
		t.Error("should include GPTBot")
	}
	if !strings.Contains(got, "User-agent: ClaudeBot") {
		t.Error("should include ClaudeBot")
	}
}

// ── GenerateManifest ──────────────────────────────────────────────────────────

func TestGenerateManifest_ValidJSON(t *testing.T) {
	cfg := &config.Config{
		Site: config.SiteConfig{
			Name:        "My Site",
			Description: "A test site",
		},
	}
	got := GenerateManifest(cfg)
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("GenerateManifest: invalid JSON: %v\n%s", err, got)
	}
	if m["name"] != "My Site" {
		t.Errorf("name: got %v", m["name"])
	}
	if m["description"] != "A test site" {
		t.Errorf("description: got %v", m["description"])
	}
	if m["display"] != "standalone" {
		t.Errorf("display: got %v", m["display"])
	}
}

// ── NewSitemapEntry ───────────────────────────────────────────────────────────

func TestNewSitemapEntry_Basic(t *testing.T) {
	e := NewSitemapEntry("https://example.com", "/recipes/soup", "2024-01-01", "0.8", "weekly")
	if e.Loc != "https://example.com/recipes/soup" {
		t.Errorf("Loc: got %q", e.Loc)
	}
	if e.Lastmod != "2024-01-01" {
		t.Errorf("Lastmod: got %q", e.Lastmod)
	}
	if e.Priority != "0.8" {
		t.Errorf("Priority: got %q", e.Priority)
	}
	if e.ChangeFreq != "weekly" {
		t.Errorf("ChangeFreq: got %q", e.ChangeFreq)
	}
}

func TestNewSitemapEntry_RootPath(t *testing.T) {
	// "/" should NOT be trimmed (it's the homepage)
	e := NewSitemapEntry("https://example.com", "/", "", "1.0", "daily")
	if e.Loc != "https://example.com/" {
		t.Errorf("root path: Loc = %q, want 'https://example.com/'", e.Loc)
	}
}

func TestNewSitemapEntry_TrailingSlash(t *testing.T) {
	// Non-root paths should have trailing slashes trimmed.
	e := NewSitemapEntry("https://example.com", "/about/", "", "", "")
	if strings.HasSuffix(e.Loc, "/") {
		t.Errorf("non-root trailing slash should be trimmed: got %q", e.Loc)
	}
}

// ── chunkEntries ──────────────────────────────────────────────────────────────

func TestChunkEntries_Basic(t *testing.T) {
	entries := make([]SitemapEntry, 5)
	for i := range entries {
		entries[i].Loc = "url"
	}
	chunks := chunkEntries(entries, 2)
	if len(chunks) != 3 {
		t.Errorf("chunkEntries(5, 2): want 3 chunks, got %d", len(chunks))
	}
	if len(chunks[0]) != 2 || len(chunks[1]) != 2 || len(chunks[2]) != 1 {
		t.Errorf("chunk sizes: got %v", []int{len(chunks[0]), len(chunks[1]), len(chunks[2])})
	}
}

func TestChunkEntries_ExactlyDivisible(t *testing.T) {
	entries := make([]SitemapEntry, 4)
	chunks := chunkEntries(entries, 2)
	if len(chunks) != 2 {
		t.Errorf("4÷2: want 2 chunks, got %d", len(chunks))
	}
}

func TestChunkEntries_Empty(t *testing.T) {
	chunks := chunkEntries(nil, 50)
	if len(chunks) != 0 {
		t.Errorf("empty: want 0 chunks, got %d", len(chunks))
	}
}

// ── GenerateSitemapFiles ──────────────────────────────────────────────────────

func TestGenerateSitemapFiles_SingleFile(t *testing.T) {
	entries := []SitemapEntry{
		{Loc: "https://example.com/a", Priority: "0.8"},
		{Loc: "https://example.com/b", Priority: "0.6"},
	}
	files := GenerateSitemapFiles(entries, "https://example.com", 0)
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d", len(files))
	}
	if files[0].Filename != "sitemap.xml" {
		t.Errorf("filename: got %q", files[0].Filename)
	}
	if !strings.Contains(files[0].Content, "https://example.com/a") {
		t.Error("sitemap should contain first URL")
	}
}

func TestGenerateSitemapFiles_MultipleFiles(t *testing.T) {
	entries := make([]SitemapEntry, 5)
	for i := range entries {
		entries[i].Loc = "https://example.com/page"
	}
	files := GenerateSitemapFiles(entries, "https://example.com", 2)
	// 5 entries at 2 per file = 3 chunk files + 1 index = 4 total
	if len(files) < 2 {
		t.Fatalf("want multiple files, got %d", len(files))
	}
	// First file should be the index
	if files[0].Filename != "sitemap.xml" {
		t.Errorf("first file should be index: got %q", files[0].Filename)
	}
	// Index should reference chunk files
	if !strings.Contains(files[0].Content, "sitemap-1.xml") {
		t.Error("index should reference sitemap-1.xml")
	}
}

func TestGenerateSitemapFiles_ValidXML(t *testing.T) {
	entries := []SitemapEntry{
		{Loc: "https://example.com/page", Lastmod: "2024-01-01", Priority: "0.8", ChangeFreq: "weekly"},
	}
	files := GenerateSitemapFiles(entries, "https://example.com", 0)
	if len(files) != 1 {
		t.Fatal("expected single file")
	}
	content := files[0].Content
	if !strings.HasPrefix(content, "<?xml") {
		t.Error("sitemap should start with XML declaration")
	}
	if !strings.Contains(content, "urlset") {
		t.Error("sitemap should contain urlset element")
	}
	if !strings.Contains(content, "https://example.com/page") {
		t.Error("sitemap should contain the URL")
	}
}

// ── GenerateRSSFeeds ──────────────────────────────────────────────────────────

func TestGenerateRSSFeeds_Disabled(t *testing.T) {
	cfg := &config.Config{RSS: config.RSSConfig{Enabled: false}}
	feeds := GenerateRSSFeeds(nil, cfg, nil)
	if feeds != nil {
		t.Errorf("expected nil when RSS disabled, got %v", feeds)
	}
}

func TestGenerateRSSFeeds_MainFeed(t *testing.T) {
	cfg := &config.Config{
		RSS:  config.RSSConfig{Enabled: true, MainFeed: "feed.xml"},
		Site: config.SiteConfig{Name: "My Site", BaseURL: "https://example.com", Description: "A site", Language: "en"},
	}
	entities := []*entity.Entity{
		{Slug: "recipe-soup", Fields: map[string]interface{}{"title": "Soup", "description": "A warm soup"}},
	}
	feeds := GenerateRSSFeeds(entities, cfg, nil)
	if len(feeds) != 1 {
		t.Fatalf("expected 1 feed, got %d", len(feeds))
	}
	if feeds[0].RelativePath != "feed.xml" {
		t.Errorf("path: got %q, want %q", feeds[0].RelativePath, "feed.xml")
	}
	if !strings.Contains(feeds[0].Content, "<?xml") {
		t.Error("feed content should start with XML declaration")
	}
	if !strings.Contains(feeds[0].Content, "Soup") {
		t.Error("feed should contain entity title")
	}
	if !strings.Contains(feeds[0].Content, "recipe-soup") {
		t.Error("feed should contain entity slug")
	}
}

func TestGenerateRSSFeeds_DefaultMainFeedPath(t *testing.T) {
	cfg := &config.Config{
		RSS:  config.RSSConfig{Enabled: true},
		Site: config.SiteConfig{Name: "Site", BaseURL: "https://example.com"},
	}
	feeds := GenerateRSSFeeds(nil, cfg, nil)
	if len(feeds) != 1 {
		t.Fatalf("expected 1 feed, got %d", len(feeds))
	}
	if feeds[0].RelativePath != "feed.xml" {
		t.Errorf("default path: got %q, want %q", feeds[0].RelativePath, "feed.xml")
	}
}

func TestGenerateRSSFeeds_CategoryFeeds(t *testing.T) {
	cfg := &config.Config{
		RSS: config.RSSConfig{
			Enabled:          true,
			CategoryFeeds:    true,
			CategoryTaxonomy: "cuisine",
		},
		Site: config.SiteConfig{Name: "Site", BaseURL: "https://example.com"},
	}
	taxEntries := map[string][]*entity.Entity{
		"italian": {
			{Slug: "pasta", Fields: map[string]interface{}{"title": "Pasta"}},
		},
	}
	feeds := GenerateRSSFeeds(nil, cfg, taxEntries)
	// main feed + 1 category feed
	if len(feeds) != 2 {
		t.Fatalf("expected 2 feeds, got %d", len(feeds))
	}
	var catFeed *RSSFeed
	for i := range feeds {
		if strings.HasPrefix(feeds[i].RelativePath, "cuisine/") {
			catFeed = &feeds[i]
		}
	}
	if catFeed == nil {
		t.Fatal("category feed not found")
	}
	if catFeed.RelativePath != "cuisine/italian/feed.xml" {
		t.Errorf("category feed path: got %q", catFeed.RelativePath)
	}
}
