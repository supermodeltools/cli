package taxonomy

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
)

// Entry represents a single taxonomy value and its associated entities.
type Entry struct {
	Name     string
	Slug     string
	Entities []*entity.Entity
}

// Taxonomy holds all entries for a single taxonomy type.
type Taxonomy struct {
	Name          string
	Label         string
	LabelSingular string
	Config        config.TaxonomyConfig
	Entries       []Entry
}

// PaginationInfo holds pagination state for hub pages.
type PaginationInfo struct {
	CurrentPage int
	TotalPages  int
	TotalItems  int
	StartIndex  int
	EndIndex    int
	PrevURL     string
	NextURL     string
	PageURLs    []PageURL
}

// PageURL represents a single page link in pagination.
type PageURL struct {
	Number int
	URL    string
}

// LetterGroup groups taxonomy entries by their first letter.
type LetterGroup struct {
	Letter  string
	Entries []Entry
}

// BuildAll constructs all taxonomies from the given entities and config.
func BuildAll(entities []*entity.Entity, taxConfigs []config.TaxonomyConfig, enrichmentData map[string]map[string]interface{}) []Taxonomy {
	var taxonomies []Taxonomy

	for _, tc := range taxConfigs {
		tax := buildOne(entities, tc, enrichmentData)
		taxonomies = append(taxonomies, tax)
	}

	return taxonomies
}

func buildOne(entities []*entity.Entity, tc config.TaxonomyConfig, enrichmentData map[string]map[string]interface{}) Taxonomy {
	// Group entities by field values
	groups := make(map[string]*Entry)

	for _, e := range entities {
		values := extractValues(e, tc, enrichmentData)

		if tc.Invert {
			// Invert mode: for each possible value, add entities that DON'T have it
			// Not commonly used - skip for now, handled separately if needed
			continue
		}

		for _, val := range values {
			slug := entity.ToSlug(val)
			if slug == "" {
				continue
			}
			if _, ok := groups[slug]; !ok {
				groups[slug] = &Entry{
					Name: val,
					Slug: slug,
				}
			}
			groups[slug].Entities = append(groups[slug].Entities, e)
		}
	}

	// Convert to slice and filter by min_entities
	var entries []Entry
	for _, entry := range groups {
		if len(entry.Entities) >= tc.MinEntities {
			entries = append(entries, *entry)
		}
	}

	// Sort alphabetically by slug
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Slug < entries[j].Slug
	})

	return Taxonomy{
		Name:          tc.Name,
		Label:         tc.Label,
		LabelSingular: tc.LabelSingular,
		Config:        tc,
		Entries:       entries,
	}
}

// extractValues gets the taxonomy values from an entity's field.
func extractValues(e *entity.Entity, tc config.TaxonomyConfig, enrichmentData map[string]map[string]interface{}) []string {
	// Check for enrichment overrides
	if tc.EnrichmentOverrideField != "" && enrichmentData != nil {
		if ed, ok := enrichmentData[e.Slug]; ok {
			if overrides := getEnrichmentOverrides(ed, tc.EnrichmentOverrideField); len(overrides) > 0 {
				return overrides
			}
		}
	}

	v, ok := e.Fields[tc.Field]
	if !ok {
		return nil
	}

	if tc.MultiValue {
		return toStringSlice(v)
	}

	// Single value
	if s, ok := v.(string); ok && s != "" {
		return []string{s}
	}
	return nil
}

// getEnrichmentOverrides extracts override values from enrichment data.
// Supports paths like "ingredients[].normalizedName"
func getEnrichmentOverrides(data map[string]interface{}, field string) []string {
	// Parse path: "ingredients[].normalizedName"
	parts := strings.Split(field, "[].")
	if len(parts) != 2 {
		// Simple field
		if v, ok := data[field]; ok {
			return toStringSlice(v)
		}
		return nil
	}

	arrayField := parts[0]
	subField := parts[1]

	arr, ok := data[arrayField]
	if !ok {
		return nil
	}

	items, ok := arr.([]interface{})
	if !ok {
		return nil
	}

	var results []string
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			if v, ok := m[subField]; ok {
				if s, ok := v.(string); ok && s != "" {
					results = append(results, s)
				}
			}
		}
	}
	return results
}

func toStringSlice(v interface{}) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		return []string{val}
	}
	return nil
}

// ComputePagination calculates pagination for a given entry.
func ComputePagination(entry Entry, page, perPage int, taxonomyName string) PaginationInfo {
	total := len(entry.Entities)
	totalPages := (total + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}

	start := (page - 1) * perPage
	end := start + perPage
	if end > total {
		end = total
	}

	info := PaginationInfo{
		CurrentPage: page,
		TotalPages:  totalPages,
		TotalItems:  total,
		StartIndex:  start,
		EndIndex:    end,
	}

	// Build page URLs
	for p := 1; p <= totalPages; p++ {
		info.PageURLs = append(info.PageURLs, PageURL{
			Number: p,
			URL:    HubPageURL(taxonomyName, entry.Slug, p),
		})
	}

	if page > 1 {
		info.PrevURL = HubPageURL(taxonomyName, entry.Slug, page-1)
	}
	if page < totalPages {
		info.NextURL = HubPageURL(taxonomyName, entry.Slug, page+1)
	}

	return info
}

// HubPageURL returns the URL path for a hub page.
func HubPageURL(taxonomyName, entrySlug string, page int) string {
	if page == 1 {
		return fmt.Sprintf("/%s/%s.html", taxonomyName, entrySlug)
	}
	return fmt.Sprintf("/%s/%s-page-%d.html", taxonomyName, entrySlug, page)
}

// GroupByLetter groups taxonomy entries by their first letter for A-Z pages.
func GroupByLetter(entries []Entry) []LetterGroup {
	groups := make(map[string][]Entry)
	var letters []string

	for _, entry := range entries {
		if len(entry.Name) == 0 {
			continue
		}
		r, _ := utf8.DecodeRuneInString(entry.Name)
		first := unicode.ToUpper(r)
		var letter string
		if unicode.IsLetter(first) {
			letter = string(first)
		} else {
			letter = "#"
		}

		if _, ok := groups[letter]; !ok {
			letters = append(letters, letter)
		}
		groups[letter] = append(groups[letter], entry)
	}

	sort.Strings(letters)

	var result []LetterGroup
	for _, letter := range letters {
		result = append(result, LetterGroup{
			Letter:  letter,
			Entries: groups[letter],
		})
	}

	return result
}

// FindEntry returns the entry with the given slug, or nil.
func (t *Taxonomy) FindEntry(slug string) *Entry {
	for i := range t.Entries {
		if t.Entries[i].Slug == slug {
			return &t.Entries[i]
		}
	}
	return nil
}

// LetterPageURL returns the URL path for a letter page.
func LetterPageURL(taxonomyName, letter string) string {
	l := strings.ToLower(letter)
	if l == "#" {
		l = "num"
	}
	return fmt.Sprintf("/%s/letter-%s.html", taxonomyName, l)
}

// TopEntries returns the top N entries sorted by entity count (descending).
func TopEntries(entries []Entry, n int) []Entry {
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].Entities) > len(sorted[j].Entities)
	})
	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}
