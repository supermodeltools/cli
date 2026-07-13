// embeddingcache/embedding_cache.go
// Semantic Tool Output Caching with embedding similarity (inspired by Context+ patterns)

package embeddingcache

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"supermodeltools/cli/embeddings" // Adjust to your actual embeddings package (Context+-style)
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const (
	SimilarityThreshold = 0.95
	DefaultTTL          = 300 * time.Second // 5 minutes
	MaxEntriesPerTool   = 50
)

var ToolTTL = map[string]time.Duration{
	"web_search":     10 * time.Minute,
	"web_fetch":      15 * time.Minute,
	"kb_search":      2 * time.Minute,
	"file_read":      1 * time.Minute,
	"file_list":      1 * time.Minute,
	"ref_lookup":     10 * time.Minute,
	"chart_snapshot": 5 * time.Minute,
}

var UncacheableTools = map[string]struct{}{
	"file_write":       {},
	"file_patch":       {},
	"file_append":      {},
	"file_delete":      {},
	"file_rename":      {},
	"file_copy":        {},
	"code_run":         {},
	"kb_save":          {},
	"kb_update":        {},
	"kb_delete":        {},
	"agent_msg":        {},
	"inbox_send":       {},
	"agent_create":     {},
	"agent_deactivate": {},
	"schedule_task":    {},
	"schedule_cancel":  {},
	"plan_create":      {},
	"plan_update":      {},
	"comfyui_generate": {},
	"git_commit":       {},
	"issue_create":     {},
	"issue_update":     {},
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type CacheEntry struct {
	ParamsVec  []float32 `json:"params_vec"`
	ParamsText string    `json:"params_text"`
	Result     string    `json:"result"`
	Timestamp  time.Time `json:"timestamp"`
}

type Stats struct {
	Hits              int64     `json:"hits"`
	Misses            int64     `json:"misses"`
	SkippedUncacheable int64    `json:"skipped_uncacheable"`
	mu                sync.Mutex
}

type Cache struct {
	mu    sync.RWMutex
	data  map[string][]CacheEntry
	stats Stats
}

// New creates a new semantic embedding cache.
func New() *Cache {
	return &Cache{
		data: make(map[string][]CacheEntry),
	}
}

// ---------------------------------------------------------------------------
// Core Methods
// ---------------------------------------------------------------------------

// CheckCache returns cached result if a semantically similar call exists, else nil.
func (c *Cache) CheckCache(toolName, paramsText string) *string {
	if _, ok := UncacheableTools[toolName]; ok {
		c.stats.incSkipped()
		return nil
	}

	c.mu.RLock()
	entries, exists := c.data[toolName]
	c.mu.RUnlock()

	if !exists || len(entries) == 0 {
		c.stats.incMiss()
		return nil
	}

	if !embeddings.IsEnabled() {
		c.stats.incMiss()
		return nil
	}

	// Create cache key (same as Python)
	cacheKey := fmt.Sprintf("%s: %s", toolName, truncate(paramsText, 500))
	vec, err := embeddings.EmbedText(cacheKey)
	if err != nil || vec == nil {
		c.stats.incMiss()
		return nil
	}

	ttl := getTTL(toolName)
	now := time.Now()

	var bestSim float32
	var bestResult *string

	c.mu.RLock()
	for i := range entries {
		e := &entries[i]
		if now.Sub(e.Timestamp) > ttl {
			continue
		}
		sim := embeddings.CosineSimilarity(vec, e.ParamsVec)
		if sim > bestSim {
			bestSim = sim
			if sim >= SimilarityThreshold {
				bestResult = &e.Result
			}
		}
	}
	c.mu.RUnlock()

	if bestResult != nil {
		c.stats.incHit()
		age := int(time.Since(entries[0].Timestamp).Seconds()) // approximate
		fmt.Printf(" [EmbCache] HIT for %s (sim=%.3f, age=%ds)\n", toolName, bestSim, age)
		return bestResult
	}

	c.stats.incMiss()
	return nil
}

// StoreResult stores the result if it's cacheable.
func (c *Cache) StoreResult(toolName, paramsText, result string) {
	if _, ok := UncacheableTools[toolName]; ok {
		return
	}
	if isErrorResult(result) {
		return
	}
	if !embeddings.IsEnabled() {
		return
	}

	cacheKey := fmt.Sprintf("%s: %s", toolName, truncate(paramsText, 500))
	vec, err := embeddings.EmbedText(cacheKey)
	if err != nil || vec == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.data[toolName]; !exists {
		c.data[toolName] = make([]CacheEntry, 0, MaxEntriesPerTool)
	}

	entries := c.data[toolName]
	entries = append(entries, CacheEntry{
		ParamsVec:  vec,
		ParamsText: truncate(paramsText, 200),
		Result:     result,
		Timestamp:  time.Now(),
	})

	if len(entries) > MaxEntriesPerTool {
		entries = entries[len(entries)-MaxEntriesPerTool:]
	}

	c.data[toolName] = entries
}

// Clear clears cache for a specific tool or entirely.
func (c *Cache) Clear(toolName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if toolName != "" {
		delete(c.data, toolName)
	} else {
		c.data = make(map[string][]CacheEntry)
	}
}

// EvictExpired removes stale entries (call periodically).
func (c *Cache) EvictExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for tool, entries := range c.data {
		ttl := getTTL(tool)
		filtered := entries[:0]
		for _, e := range entries {
			if now.Sub(e.Timestamp) <= ttl {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			delete(c.data, tool)
		} else {
			c.data[tool] = filtered
		}
	}
}

// GetStats returns current statistics.
func (c *Cache) GetStats() map[string]any {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()

	total := c.stats.Hits + c.stats.Misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.stats.Hits) / float64(total) * 100
	}

	c.mu.RLock()
	cachedTools := make([]string, 0, len(c.data))
	totalEntries := 0
	for t, e := range c.data {
		cachedTools = append(cachedTools, t)
		totalEntries += len(e)
	}
	c.mu.RUnlock()

	return map[string]any{
		"hits":                 c.stats.Hits,
		"misses":               c.stats.Misses,
		"skipped_uncacheable":  c.stats.SkippedUncacheable,
		"hit_rate_pct":         math.Round(hitRate*10) / 10,
		"cached_tools":         cachedTools,
		"total_entries":        totalEntries,
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func getTTL(toolName string) time.Duration {
	if d, ok := ToolTTL[toolName]; ok {
		return d
	}
	return DefaultTTL
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func isErrorResult(s string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	for _, word := range []string{"error", "traceback", "failed", "exception"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// Stats helpers
func (s *Stats) incHit() {
	s.mu.Lock()
	s.Hits++
	s.mu.Unlock()
}

func (s *Stats) incMiss() {
	s.mu.Lock()
	s.Misses++
	s.mu.Unlock()
}

func (s *Stats) incSkipped() {
	s.mu.Lock()
	s.SkippedUncacheable++
	s.mu.Unlock()
}
