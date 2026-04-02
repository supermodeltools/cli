package output

import (
	"fmt"
	"strings"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
)

// GenerateRobotsTxt generates a robots.txt file.
func GenerateRobotsTxt(cfg *config.Config) string {
	var lines []string

	lines = append(lines, "User-agent: *")
	if cfg.Robots.AllowAll {
		lines = append(lines, "Allow: /")
	}
	lines = append(lines, "")

	// Standard bots
	standardBots := []string{"Googlebot", "Bingbot"}
	for _, bot := range standardBots {
		lines = append(lines, fmt.Sprintf("User-agent: %s", bot))
		lines = append(lines, "Allow: /")
		lines = append(lines, "")
	}

	// Extra bots (AI crawlers etc.)
	for _, bot := range cfg.Robots.ExtraBots {
		lines = append(lines, fmt.Sprintf("User-agent: %s", bot))
		lines = append(lines, "Allow: /")
		lines = append(lines, "")
	}

	// Sitemap
	sitemapURL := fmt.Sprintf("%s/sitemap.xml", cfg.Site.BaseURL)
	lines = append(lines, fmt.Sprintf("Sitemap: %s", sitemapURL))

	return strings.Join(lines, "\n") + "\n"
}
