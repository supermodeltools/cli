package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
)

// MarkdownLoader loads entities from markdown files with YAML frontmatter.
type MarkdownLoader struct {
	Config *config.Config
}

// Load reads all .md files from the data directory and parses them into entities.
func (l *MarkdownLoader) Load() ([]*entity.Entity, error) {
	dataDir := l.Config.Paths.Data
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("reading data dir %s: %w", dataDir, err)
	}

	var entities []*entity.Entity
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dataDir, entry.Name())
		e, err := l.parseFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", entry.Name(), err)
			continue
		}
		entities = append(entities, e)
	}

	return entities, nil
}

func (l *MarkdownLoader) parseFile(path string) (*entity.Entity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)

	// Split frontmatter from body
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("splitting frontmatter: %w", err)
	}

	// Parse YAML frontmatter
	var fields map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatter), &fields); err != nil {
		return nil, fmt.Errorf("parsing frontmatter YAML: %w", err)
	}

	// Derive slug
	slug := l.deriveSlug(path, fields)

	// Parse body sections
	sections := l.parseSections(body)

	return &entity.Entity{
		Slug:       slug,
		SourceFile: path,
		Fields:     fields,
		Sections:   sections,
		Body:       body,
	}, nil
}

// splitFrontmatter separates YAML frontmatter (between --- delimiters) from the body.
func splitFrontmatter(content string) (string, string, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return "", content, nil
	}

	// Find closing ---
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", content, fmt.Errorf("no closing --- found for frontmatter")
	}

	fm := strings.TrimSpace(rest[:idx])
	body := strings.TrimSpace(rest[idx+4:])
	return fm, body, nil
}

func (l *MarkdownLoader) deriveSlug(path string, fields map[string]interface{}) string {
	source := l.Config.Data.EntitySlug.Source
	if strings.HasPrefix(source, "field:") {
		fieldName := source[6:]
		if v, ok := fields[fieldName]; ok {
			if s, ok := v.(string); ok {
				return entity.ToSlug(s)
			}
		}
	}
	// Default: derive from filename
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (l *MarkdownLoader) parseSections(body string) map[string]interface{} {
	sections := make(map[string]interface{})

	for _, sectionCfg := range l.Config.Data.BodySections {
		content := extractSection(body, sectionCfg.Header)
		if content == "" {
			continue
		}

		switch sectionCfg.Type {
		case "unordered_list":
			sections[sectionCfg.Name] = parseUnorderedList(content)
		case "ordered_list":
			sections[sectionCfg.Name] = parseOrderedList(content)
		case "faq":
			sections[sectionCfg.Name] = parseFAQs(content)
		case "markdown":
			sections[sectionCfg.Name] = content
		default:
			sections[sectionCfg.Name] = content
		}
	}

	return sections
}

// extractSection extracts the content under a ## heading until the next ## heading.
func extractSection(body, header string) string {
	marker := "## " + header
	idx := strings.Index(body, marker)
	if idx < 0 {
		return ""
	}

	// Start after the heading line
	start := idx + len(marker)
	// Find newline after heading
	nlIdx := strings.Index(body[start:], "\n")
	if nlIdx < 0 {
		return ""
	}
	start += nlIdx + 1

	// Find next ## heading or end of body
	rest := body[start:]
	nextH2 := strings.Index(rest, "\n## ")
	if nextH2 >= 0 {
		rest = rest[:nextH2]
	}

	return strings.TrimSpace(rest)
}

// parseUnorderedList extracts items from a markdown unordered list.
func parseUnorderedList(content string) []string {
	var items []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			items = append(items, strings.TrimPrefix(line, "- "))
		} else if strings.HasPrefix(line, "* ") {
			items = append(items, strings.TrimPrefix(line, "* "))
		}
	}
	return items
}

// parseOrderedList extracts items from a markdown ordered list.
func parseOrderedList(content string) []string {
	var items []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		// Match "1. ", "2. ", etc.
		if len(line) >= 3 {
			dotIdx := strings.Index(line, ". ")
			if dotIdx > 0 && dotIdx <= 4 {
				// Verify prefix is all digits
				prefix := line[:dotIdx]
				allDigits := true
				for _, c := range prefix {
					if c < '0' || c > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					items = append(items, line[dotIdx+2:])
				}
			}
		}
	}
	return items
}

// parseFAQs extracts FAQ pairs from ### headings and their following paragraphs.
func parseFAQs(content string) []entity.FAQ {
	var faqs []entity.FAQ

	parts := strings.Split("\n"+content, "\n### ")
	for _, part := range parts[1:] { // skip first empty split
		lines := strings.SplitN(part, "\n", 2)
		question := strings.TrimSpace(lines[0])
		answer := ""
		if len(lines) > 1 {
			answer = strings.TrimSpace(lines[1])
		}
		if question != "" {
			faqs = append(faqs, entity.FAQ{
				Question: question,
				Answer:   answer,
			})
		}
	}

	return faqs
}
