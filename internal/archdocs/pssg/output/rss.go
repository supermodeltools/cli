package output

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
)

type rssDoc struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Items         []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Category    string `xml:"category,omitempty"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
}

// RSSFeed represents a generated RSS feed file.
type RSSFeed struct {
	RelativePath string
	Content      string
}

// GenerateRSSFeeds generates the main RSS feed and optionally per-category feeds.
func GenerateRSSFeeds(entities []*entity.Entity, cfg *config.Config, taxonomyEntries map[string][]*entity.Entity) []RSSFeed {
	if !cfg.RSS.Enabled {
		return nil
	}

	buildDate := time.Now().UTC().Format(time.RFC1123Z)
	var feeds []RSSFeed

	// Main feed
	mainPath := cfg.RSS.MainFeed
	if mainPath == "" {
		mainPath = "feed.xml"
	}
	mainContent := generateFeed(
		cfg.Site.Name,
		cfg.Site.BaseURL,
		cfg.Site.Description,
		cfg.Site.Language,
		buildDate,
		entities,
		cfg.Site.BaseURL,
	)
	feeds = append(feeds, RSSFeed{
		RelativePath: mainPath,
		Content:      mainContent,
	})

	// Per-category feeds
	if cfg.RSS.CategoryFeeds && taxonomyEntries != nil {
		for slug, catEntities := range taxonomyEntries {
			title := fmt.Sprintf("%s — %s", cfg.Site.Name, slug)
			catContent := generateFeed(
				title,
				fmt.Sprintf("%s/%s/%s.html", cfg.Site.BaseURL, cfg.RSS.CategoryTaxonomy, slug),
				fmt.Sprintf("%s recipes", slug),
				cfg.Site.Language,
				buildDate,
				catEntities,
				cfg.Site.BaseURL,
			)
			feeds = append(feeds, RSSFeed{
				RelativePath: fmt.Sprintf("%s/%s/feed.xml", cfg.RSS.CategoryTaxonomy, slug),
				Content:      catContent,
			})
		}
	}

	return feeds
}

func generateFeed(title, link, description, language, buildDate string, entities []*entity.Entity, baseURL string) string {
	// xml.MarshalIndent escapes special characters (&, <, >, ") automatically.
	// Do NOT pre-escape with a manual xmlEscape — that would double-encode them
	// (e.g. "Beef & Rice" → "&amp;amp;" in the final XML).
	channel := rssChannel{
		Title:         title,
		Link:          link,
		Description:   description,
		Language:      language,
		LastBuildDate: buildDate,
	}

	for _, e := range entities {
		item := rssItem{
			Title:       e.GetString("title"),
			Link:        fmt.Sprintf("%s/%s.html", baseURL, e.Slug),
			Description: e.GetString("description"),
			GUID:        fmt.Sprintf("%s/%s.html", baseURL, e.Slug),
			PubDate:     buildDate,
		}
		if category := e.GetString("recipe_category"); category != "" {
			item.Category = category
		}
		channel.Items = append(channel.Items, item)
	}

	doc := rssDoc{
		Version: "2.0",
		Channel: channel,
	}

	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return ""
	}
	return xml.Header + string(data)
}
