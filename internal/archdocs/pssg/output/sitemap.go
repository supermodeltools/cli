package output

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// SitemapEntry represents a single URL in the sitemap.
type SitemapEntry struct {
	Loc        string
	Lastmod    string
	Priority   string
	ChangeFreq string
}

type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	XMLNS   string     `xml:"xmlns,attr"`
	URLs    []urlEntry `xml:"url"`
}

type urlEntry struct {
	Loc        string `xml:"loc"`
	Lastmod    string `xml:"lastmod,omitempty"`
	Priority   string `xml:"priority,omitempty"`
	ChangeFreq string `xml:"changefreq,omitempty"`
}

type sitemapIndex struct {
	XMLName  xml.Name        `xml:"sitemapindex"`
	XMLNS    string          `xml:"xmlns,attr"`
	Sitemaps []sitemapEntry  `xml:"sitemap"`
}

type sitemapEntry struct {
	Loc     string `xml:"loc"`
	Lastmod string `xml:"lastmod,omitempty"`
}

// GenerateSitemapFiles generates sitemap XML files, splitting at maxPerFile URLs.
func GenerateSitemapFiles(entries []SitemapEntry, baseURL string, maxPerFile int) []SitemapFile {
	if maxPerFile <= 0 {
		maxPerFile = 50000
	}

	if len(entries) <= maxPerFile {
		// Single sitemap
		content := generateSitemap(entries)
		return []SitemapFile{
			{Filename: "sitemap.xml", Content: content},
		}
	}

	// Multiple sitemaps with index
	var files []SitemapFile
	var indexEntries []sitemapEntry
	lastmod := ""
	if len(entries) > 0 {
		lastmod = entries[0].Lastmod
	}

	chunks := chunkEntries(entries, maxPerFile)
	for i, chunk := range chunks {
		filename := fmt.Sprintf("sitemap-%d.xml", i+1)
		content := generateSitemap(chunk)
		files = append(files, SitemapFile{
			Filename: filename,
			Content:  content,
		})
		indexEntries = append(indexEntries, sitemapEntry{
			Loc:     fmt.Sprintf("%s/%s", baseURL, filename),
			Lastmod: lastmod,
		})
	}

	// Generate index
	indexContent := generateSitemapIndex(indexEntries)
	files = append([]SitemapFile{{Filename: "sitemap.xml", Content: indexContent}}, files...)

	return files
}

// SitemapFile is a filename + content pair.
type SitemapFile struct {
	Filename string
	Content  string
}

func generateSitemap(entries []SitemapEntry) string {
	us := urlSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
	}
	for _, e := range entries {
		us.URLs = append(us.URLs, urlEntry{
			Loc:        e.Loc,
			Lastmod:    e.Lastmod,
			Priority:   e.Priority,
			ChangeFreq: e.ChangeFreq,
		})
	}

	data, err := xml.MarshalIndent(us, "", "  ")
	if err != nil {
		return ""
	}
	return xml.Header + string(data)
}

func generateSitemapIndex(entries []sitemapEntry) string {
	si := sitemapIndex{
		XMLNS:    "http://www.sitemaps.org/schemas/sitemap/0.9",
		Sitemaps: entries,
	}

	data, err := xml.MarshalIndent(si, "", "  ")
	if err != nil {
		return ""
	}
	return xml.Header + string(data)
}

func chunkEntries(entries []SitemapEntry, size int) [][]SitemapEntry {
	var chunks [][]SitemapEntry
	for i := 0; i < len(entries); i += size {
		end := i + size
		if end > len(entries) {
			end = len(entries)
		}
		chunks = append(chunks, entries[i:end])
	}
	return chunks
}

// NewSitemapEntry creates a sitemap entry with the given base URL.
func NewSitemapEntry(baseURL, path, lastmod, priority, changefreq string) SitemapEntry {
	loc := baseURL + path
	loc = strings.TrimRight(loc, "/")
	if path == "/" {
		loc = baseURL + "/"
	}
	return SitemapEntry{
		Loc:        loc,
		Lastmod:    lastmod,
		Priority:   priority,
		ChangeFreq: changefreq,
	}
}
