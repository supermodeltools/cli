package schema

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
)

// Generator creates JSON-LD structured data.
type Generator struct {
	SiteConfig config.SiteConfig
	Schema     config.SchemaConfig
}

// NewGenerator creates a new JSON-LD generator.
func NewGenerator(siteCfg config.SiteConfig, schemaCfg config.SchemaConfig) *Generator {
	return &Generator{
		SiteConfig: siteCfg,
		Schema:     schemaCfg,
	}
}

// GenerateRecipeSchema generates Recipe JSON-LD for an entity.
func (g *Generator) GenerateRecipeSchema(e *entity.Entity, entityURL string) map[string]interface{} {
	schema := map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "Recipe",
		"name":        e.GetString("title"),
		"description": e.GetString("description"),
		"url":         entityURL,
	}

	// Author
	authorName := e.GetString("author")
	if authorName != "" {
		authorSlug := entity.ToSlug(authorName)
		schema["author"] = map[string]interface{}{
			"@type": "Person",
			"name":  authorName,
			"url":   fmt.Sprintf("%s/author/%s.html", g.SiteConfig.BaseURL, authorSlug),
		}
	}

	// Date published
	schema["datePublished"] = g.Schema.DatePublished

	// Times
	prepTime := e.GetString("prep_time")
	cookTime := e.GetString("cook_time")
	if prepTime != "" {
		schema["prepTime"] = prepTime
	}
	if cookTime != "" {
		schema["cookTime"] = cookTime
	}
	if prepTime != "" && cookTime != "" {
		schema["totalTime"] = computeTotalTime(prepTime, cookTime)
	}

	// Servings
	if servings := e.GetInt("servings"); servings > 0 {
		schema["recipeYield"] = fmt.Sprintf("%d servings", servings)
	}

	// Category & cuisine
	if cat := e.GetString("recipe_category"); cat != "" {
		schema["recipeCategory"] = cat
	}
	if cuisine := e.GetString("cuisine"); cuisine != "" {
		schema["recipeCuisine"] = cuisine
	}

	// Image
	if img := e.GetString("image"); img != "" {
		schema["image"] = []string{img}
	}

	// Nutrition
	if cal := e.GetInt("calories"); cal > 0 {
		schema["nutrition"] = map[string]interface{}{
			"@type":    "NutritionInformation",
			"calories": fmt.Sprintf("%d calories", cal),
		}
	}

	// Ingredients
	if ingredients := e.GetIngredients(); len(ingredients) > 0 {
		schema["recipeIngredient"] = ingredients
	}

	// Instructions as HowToSteps
	if instructions := e.GetInstructions(); len(instructions) > 0 {
		var steps []map[string]interface{}
		for i, inst := range instructions {
			steps = append(steps, map[string]interface{}{
				"@type":    "HowToStep",
				"text":     inst,
				"name":     stepName(inst),
				"position": i + 1,
			})
		}
		schema["recipeInstructions"] = steps
	}

	// Keywords
	keywords := e.GetStringSlice("keywords")
	extra := g.Schema.ExtraKeywords
	allKeywords := append(keywords, extra...)
	if len(allKeywords) > 0 {
		schema["keywords"] = strings.Join(allKeywords, ", ")
	}

	// Pairings as isRelatedTo
	if pairings := e.GetStringSlice("pairings"); len(pairings) > 0 {
		var related []map[string]interface{}
		for _, slug := range pairings {
			related = append(related, map[string]interface{}{
				"@type": "Recipe",
				"name":  slug, // Will be resolved to title by the builder
				"url":   fmt.Sprintf("%s/%s.html", g.SiteConfig.BaseURL, slug),
			})
		}
		schema["isRelatedTo"] = related
	}

	return schema
}

// GenerateBreadcrumbSchema generates BreadcrumbList JSON-LD.
func (g *Generator) GenerateBreadcrumbSchema(items []BreadcrumbItem) map[string]interface{} {
	var listItems []map[string]interface{}
	for i, item := range items {
		li := map[string]interface{}{
			"@type":    "ListItem",
			"position": i + 1,
			"name":     item.Name,
		}
		if item.URL != "" {
			li["item"] = item.URL
		}
		listItems = append(listItems, li)
	}

	return map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": listItems,
	}
}

// BreadcrumbItem is a single breadcrumb entry.
type BreadcrumbItem struct {
	Name string
	URL  string
}

// GenerateFAQSchema generates FAQPage JSON-LD from FAQs.
func (g *Generator) GenerateFAQSchema(faqs []entity.FAQ) map[string]interface{} {
	if len(faqs) == 0 {
		return nil
	}

	var mainEntity []map[string]interface{}
	for _, faq := range faqs {
		mainEntity = append(mainEntity, map[string]interface{}{
			"@type": "Question",
			"name":  faq.Question,
			"acceptedAnswer": map[string]interface{}{
				"@type": "Answer",
				"text":  faq.Answer,
			},
		})
	}

	return map[string]interface{}{
		"@context":   "https://schema.org",
		"@type":      "FAQPage",
		"mainEntity": mainEntity,
	}
}

// GenerateWebSiteSchema generates WebSite JSON-LD.
func (g *Generator) GenerateWebSiteSchema(imageURL string) map[string]interface{} {
	s := map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "WebSite",
		"name":        g.SiteConfig.Name,
		"url":         g.SiteConfig.BaseURL,
		"description": g.SiteConfig.Description,
		"publisher": map[string]interface{}{
			"@type": "Organization",
			"name":  g.SiteConfig.Name,
			"url":   g.SiteConfig.BaseURL,
		},
	}
	if imageURL != "" {
		s["image"] = imageURL
	}
	return s
}

// GenerateItemListSchema generates ItemList JSON-LD.
func (g *Generator) GenerateItemListSchema(name, description string, items []ItemListEntry, imageURL string) map[string]interface{} {
	var listItems []map[string]interface{}
	for i, item := range items {
		listItems = append(listItems, map[string]interface{}{
			"@type":    "ListItem",
			"position": i + 1,
			"url":      item.URL,
			"name":     item.Name,
		})
	}

	s := map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "ItemList",
		"name":            name,
		"description":     description,
		"numberOfItems":   len(items),
		"itemListElement": listItems,
	}
	if imageURL != "" {
		s["image"] = imageURL
	}
	return s
}

// ItemListEntry is a single item in an ItemList.
type ItemListEntry struct {
	Name string
	URL  string
}

// GenerateCollectionPageSchema generates CollectionPage JSON-LD.
func (g *Generator) GenerateCollectionPageSchema(name, description, pageURL string, items []ItemListEntry, imageURL string) map[string]interface{} {
	var listItems []map[string]interface{}
	for i, item := range items {
		listItems = append(listItems, map[string]interface{}{
			"@type":    "ListItem",
			"position": i + 1,
			"url":      item.URL,
			"name":     item.Name,
		})
	}

	s := map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "CollectionPage",
		"name":        name,
		"url":         pageURL,
		"description": description,
		"mainEntity": map[string]interface{}{
			"@type":           "ItemList",
			"numberOfItems":   len(items),
			"itemListElement": listItems,
		},
	}
	if imageURL != "" {
		s["image"] = imageURL
	}
	return s
}

// MarshalSchemas encodes one or more schemas as a JSON-LD script block.
func MarshalSchemas(schemas ...map[string]interface{}) string {
	var parts []string
	for _, s := range schemas {
		if s == nil {
			continue
		}
		data, err := json.Marshal(s)
		if err != nil {
			continue
		}
		parts = append(parts, fmt.Sprintf(`<script type="application/ld+json">%s</script>`, string(data)))
	}
	return strings.Join(parts, "\n")
}

// stepName extracts a short name from an instruction step.
func stepName(step string) string {
	runes := []rune(step)
	// Take first sentence if it fits within 80 runes.
	for _, sep := range []string{". ", ".\n"} {
		if idx := strings.Index(step, sep); idx > 0 {
			// idx is a byte offset; compute rune length of the candidate name.
			if len([]rune(step[:idx+1])) < 80 {
				return step[:idx+1]
			}
		}
	}
	// Truncate if too long (rune-aware to avoid splitting multi-byte chars).
	if len(runes) > 80 {
		return string(runes[:77]) + "..."
	}
	return step
}

var durationRegex = regexp.MustCompile(`PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?`)

// parseDurationMinutes parses an ISO 8601 duration to minutes, rounding seconds down.
func parseDurationMinutes(d string) int {
	matches := durationRegex.FindStringSubmatch(d)
	if matches == nil {
		return 0
	}
	hours, _ := strconv.Atoi(matches[1])
	minutes, _ := strconv.Atoi(matches[2])
	seconds, _ := strconv.Atoi(matches[3])
	return hours*60 + minutes + seconds/60
}

// computeTotalTime adds two ISO 8601 durations and returns the result.
func computeTotalTime(d1, d2 string) string {
	total := parseDurationMinutes(d1) + parseDurationMinutes(d2)
	hours := total / 60
	minutes := total % 60
	if hours > 0 && minutes > 0 {
		return fmt.Sprintf("PT%dH%dM", hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("PT%dH", hours)
	}
	return fmt.Sprintf("PT%dM", minutes)
}
