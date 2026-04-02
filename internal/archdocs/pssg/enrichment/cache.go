package enrichment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CacheEntry represents the cached enrichment data for a single entity.
type CacheEntry struct {
	ContentHash string                 `json:"contentHash"`
	Enrichment  map[string]interface{} `json:"enrichment"`
	Timestamp   string                 `json:"timestamp"`
}

// ReadCache reads a single enrichment cache file for the given slug.
// Returns nil if the cache file doesn't exist or is invalid.
func ReadCache(cacheDir, slug string) map[string]interface{} {
	filePath := filepath.Join(cacheDir, slug+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil
	}

	return entry.Enrichment
}

// ReadAllCaches reads all cache files from the cache directory.
func ReadAllCaches(cacheDir string) (map[string]map[string]interface{}, error) {
	result := make(map[string]map[string]interface{})

	if cacheDir == "" {
		return result, nil
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, fmt.Errorf("reading cache dir %s: %w", cacheDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		slug := entry.Name()[:len(entry.Name())-5] // strip .json
		data := ReadCache(cacheDir, slug)
		if data != nil {
			result[slug] = data
		}
	}

	return result, nil
}

// GetIngredients extracts ingredient search terms from enrichment data.
func GetIngredients(data map[string]interface{}) []map[string]interface{} {
	v, ok := data["ingredients"]
	if !ok {
		return nil
	}
	items, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var result []map[string]interface{}
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}
	return result
}

// GetGear extracts gear items from enrichment data.
func GetGear(data map[string]interface{}) []map[string]interface{} {
	v, ok := data["gear"]
	if !ok {
		return nil
	}
	items, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var result []map[string]interface{}
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}
	return result
}

// GetCookingTips extracts cooking tips from enrichment data.
func GetCookingTips(data map[string]interface{}) []string {
	v, ok := data["cookingTips"]
	if !ok {
		return nil
	}
	items, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var result []string
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// GetCoachingPrompt extracts the coaching prompt from enrichment data.
func GetCoachingPrompt(data map[string]interface{}) string {
	v, ok := data["coachingPrompt"]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
