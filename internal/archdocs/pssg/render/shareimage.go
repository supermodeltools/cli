package render

import (
	"fmt"
	"strings"
)

// Share image constants
const (
	svgWidth  = 1200
	svgHeight = 630
	svgBG     = "#0f1117"
	svgText   = "#e4e4e7"
	svgMuted  = "#71717a"
	svgAccent = "#5B7B5E"
	svgAccent2 = "#C4956A"
)

// svgEscape escapes text for safe embedding in SVG.
func svgEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// truncate limits string length with ellipsis.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "\u2026"
}

// svgScaffold wraps content in the standard share image scaffold.
func svgScaffold(siteName, pageTitle, content string) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="%s"/>
  <text x="60" y="56" font-family="system-ui,sans-serif" font-size="18" font-weight="600" fill="%s">%s</text>
  <text x="60" y="110" font-family="system-ui,sans-serif" font-size="36" font-weight="700" fill="%s">%s</text>
  %s
  <rect x="0" y="%d" width="%d" height="8" fill="url(#accent-grad)"/>
  <defs>
    <linearGradient id="accent-grad" x1="0" y1="0" x2="1" y2="0">
      <stop offset="0" stop-color="%s"/>
      <stop offset="1" stop-color="%s"/>
    </linearGradient>
  </defs>
</svg>`,
		svgWidth, svgHeight, svgWidth, svgHeight,
		svgWidth, svgHeight, svgBG,
		svgMuted, svgEscape(siteName),
		svgText, svgEscape(truncate(pageTitle, 60)),
		content,
		svgHeight-8, svgWidth,
		svgAccent, svgAccent2,
	)
}

// renderBarsSVG renders horizontal bars as SVG elements.
func renderBarsSVG(bars []NameCount, x, y, maxW, barH, gap int) string {
	if len(bars) == 0 {
		return ""
	}
	maxVal := bars[0].Count
	for _, b := range bars {
		if b.Count > maxVal {
			maxVal = b.Count
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	var sb strings.Builder
	colors := []string{"#5B7B5E", "#C4956A", "#4A7B9B", "#7C5BB0", "#A68B2D", "#B94A4A", "#6B6B6B", "#3d8b6e"}
	for i, b := range bars {
		w := (b.Count * maxW) / maxVal
		if w < 4 {
			w = 4
		}
		cy := y + i*(barH+gap)
		color := colors[i%len(colors)]
		sb.WriteString(fmt.Sprintf(`  <rect x="%d" y="%d" width="%d" height="%d" rx="4" fill="%s" opacity="0.85"/>`, x, cy, w, barH, color))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf(`  <text x="%d" y="%d" font-family="system-ui,sans-serif" font-size="14" fill="%s">%s</text>`, x, cy-4, svgText, svgEscape(truncate(b.Name, 30))))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf(`  <text x="%d" y="%d" font-family="system-ui,sans-serif" font-size="13" fill="%s">%d</text>`, x+w+8, cy+barH-4, svgMuted, b.Count))
		sb.WriteString("\n")
	}
	return sb.String()
}

// GenerateHomepageShareSVG generates the homepage share image SVG.
func GenerateHomepageShareSVG(siteName, description string, taxStats []NameCount, totalEntities int) string {
	var content strings.Builder
	content.WriteString(fmt.Sprintf(`  <text x="60" y="160" font-family="system-ui,sans-serif" font-size="18" fill="%s">%s</text>`, svgMuted, svgEscape(truncate(description, 80))))
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf(`  <text x="60" y="200" font-family="system-ui,sans-serif" font-size="22" font-weight="600" fill="%s">%d total recipes</text>`, svgAccent, totalEntities))
	content.WriteString("\n")

	// Show taxonomy bars (max 8)
	limit := len(taxStats)
	if limit > 8 {
		limit = 8
	}
	bars := taxStats[:limit]
	content.WriteString(renderBarsSVG(bars, 60, 250, 900, 28, 14))

	return svgScaffold(siteName, siteName+" \u2014 Recipe Collection", content.String())
}

// GenerateEntityShareSVG generates the entity share image SVG.
func GenerateEntityShareSVG(siteName, title, category, cuisine, skillLevel string) string {
	var content strings.Builder

	// Pills for metadata
	pillX := 60
	pillY := 170
	pills := []struct{ label, color string }{
		{category, svgAccent},
		{cuisine, svgAccent2},
		{skillLevel, "#4A7B9B"},
	}
	for _, p := range pills {
		if p.label == "" {
			continue
		}
		w := len(p.label)*10 + 24
		content.WriteString(fmt.Sprintf(`  <rect x="%d" y="%d" width="%d" height="32" rx="16" fill="%s" opacity="0.2"/>`, pillX, pillY, w, p.color))
		content.WriteString("\n")
		content.WriteString(fmt.Sprintf(`  <text x="%d" y="%d" font-family="system-ui,sans-serif" font-size="14" font-weight="600" fill="%s">%s</text>`, pillX+12, pillY+21, p.color, svgEscape(p.label)))
		content.WriteString("\n")
		pillX += w + 12
	}

	// Large decorative title
	content.WriteString(fmt.Sprintf(`  <text x="600" y="380" text-anchor="middle" font-family="Georgia,serif" font-size="48" font-weight="700" fill="%s" opacity="0.15">%s</text>`, svgText, svgEscape(truncate(title, 40))))
	content.WriteString("\n")

	return svgScaffold(siteName, truncate(title, 55), content.String())
}

// GenerateHubShareSVG generates the hub page share image SVG.
func GenerateHubShareSVG(siteName, entryName, taxLabel string, count int, topTypes []NameCount) string {
	var content strings.Builder
	content.WriteString(fmt.Sprintf(`  <text x="60" y="160" font-family="system-ui,sans-serif" font-size="18" fill="%s">%s · %d recipes</text>`, svgMuted, svgEscape(taxLabel), count))
	content.WriteString("\n")

	limit := len(topTypes)
	if limit > 6 {
		limit = 6
	}
	bars := topTypes[:limit]
	content.WriteString(renderBarsSVG(bars, 60, 220, 900, 32, 16))

	return svgScaffold(siteName, entryName, content.String())
}

// GenerateTaxIndexShareSVG generates the taxonomy index share image SVG.
func GenerateTaxIndexShareSVG(siteName, taxLabel string, topEntries []NameCount) string {
	var content strings.Builder
	content.WriteString(fmt.Sprintf(`  <text x="60" y="160" font-family="system-ui,sans-serif" font-size="18" fill="%s">Browse all %s</text>`, svgMuted, svgEscape(taxLabel)))
	content.WriteString("\n")

	limit := len(topEntries)
	if limit > 10 {
		limit = 10
	}
	bars := topEntries[:limit]
	content.WriteString(renderBarsSVG(bars, 60, 210, 900, 26, 12))

	return svgScaffold(siteName, taxLabel, content.String())
}

// GenerateAllEntitiesShareSVG generates the all-entities share image SVG.
func GenerateAllEntitiesShareSVG(siteName string, totalCount int, typeDist []NameCount) string {
	var content strings.Builder
	content.WriteString(fmt.Sprintf(`  <text x="60" y="160" font-family="system-ui,sans-serif" font-size="22" font-weight="600" fill="%s">%d recipes</text>`, svgAccent, totalCount))
	content.WriteString("\n")

	// Proportional bar segments
	if len(typeDist) > 0 {
		totalForBar := 0
		for _, t := range typeDist {
			totalForBar += t.Count
		}
		if totalForBar == 0 {
			totalForBar = 1
		}
		colors := []string{"#5B7B5E", "#C4956A", "#4A7B9B", "#7C5BB0", "#A68B2D", "#B94A4A", "#6B6B6B", "#3d8b6e"}
		barX := 60
		barY := 200
		barW := 1080
		barH := 40
		cx := barX
		limit := len(typeDist)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			w := (typeDist[i].Count * barW) / totalForBar
			if w < 2 {
				w = 2
			}
			color := colors[i%len(colors)]
			content.WriteString(fmt.Sprintf(`  <rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`, cx, barY, w, barH, color))
			content.WriteString("\n")
			cx += w
		}

		// Legend
		ly := 280
		for i := 0; i < limit; i++ {
			lx := 60 + (i%4)*270
			if i > 0 && i%4 == 0 {
				ly += 30
			}
			color := colors[i%len(colors)]
			content.WriteString(fmt.Sprintf(`  <rect x="%d" y="%d" width="12" height="12" rx="2" fill="%s"/>`, lx, ly, color))
			content.WriteString(fmt.Sprintf(`  <text x="%d" y="%d" font-family="system-ui,sans-serif" font-size="13" fill="%s">%s (%d)</text>`, lx+18, ly+11, svgMuted, svgEscape(truncate(typeDist[i].Name, 25)), typeDist[i].Count))
			content.WriteString("\n")
		}
	}

	return svgScaffold(siteName, "All Recipes", content.String())
}

// GenerateLetterShareSVG generates the letter page share image SVG.
func GenerateLetterShareSVG(siteName, taxLabel, letter string, entryCount int) string {
	var content strings.Builder
	content.WriteString(fmt.Sprintf(`  <text x="60" y="160" font-family="system-ui,sans-serif" font-size="18" fill="%s">%s · %d entries</text>`, svgMuted, svgEscape(taxLabel), entryCount))
	content.WriteString("\n")

	// Large decorative letter
	content.WriteString(fmt.Sprintf(`  <text x="600" y="440" text-anchor="middle" font-family="Georgia,serif" font-size="220" font-weight="700" fill="%s" opacity="0.08">%s</text>`, svgText, svgEscape(letter)))
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf(`  <text x="600" y="440" text-anchor="middle" font-family="Georgia,serif" font-size="120" font-weight="700" fill="%s" opacity="0.25">%s</text>`, svgAccent, svgEscape(letter)))
	content.WriteString("\n")

	return svgScaffold(siteName, fmt.Sprintf("%s \u2014 %s", taxLabel, letter), content.String())
}
