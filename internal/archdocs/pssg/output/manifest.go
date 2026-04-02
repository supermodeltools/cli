package output

import (
	"encoding/json"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
)

// GenerateManifest generates a PWA manifest.json.
func GenerateManifest(cfg *config.Config) string {
	manifest := map[string]interface{}{
		"name":             cfg.Site.Name,
		"short_name":       cfg.Site.Name,
		"description":      cfg.Site.Description,
		"start_url":        "/",
		"display":          "standalone",
		"background_color": "#FAFAF7",
		"theme_color":      "#5B7B5E",
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}
