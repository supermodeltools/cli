package shards

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TourStrategy chooses a linear reading order over the source files.
type TourStrategy interface {
	Name() string
	Order(cache *Cache) []string
}

// TopoStrategy orders files by reverse topological order over the import graph
// (leaves first, roots last). Cycles are broken by lexicographic file path.
type TopoStrategy struct{}

func (TopoStrategy) Name() string { return "topo" }

func (TopoStrategy) Order(cache *Cache) []string {
	files := cache.SourceFiles()
	sort.Strings(files) // deterministic tiebreak

	inDegree := make(map[string]int, len(files))
	present := make(map[string]bool, len(files))
	for _, f := range files {
		present[f] = true
	}
	// inDegree[f] = number of files that f depends on (imports).
	// Leaves (no imports to other tracked files) have inDegree 0 → emitted first.
	for _, f := range files {
		for _, dep := range cache.Imports[f] {
			if present[dep] && dep != f {
				inDegree[f]++
			}
		}
	}

	var queue []string
	for _, f := range files {
		if inDegree[f] == 0 {
			queue = append(queue, f)
		}
	}
	sort.Strings(queue)

	var out []string
	emitted := make(map[string]bool, len(files))

	for len(queue) > 0 {
		f := queue[0]
		queue = queue[1:]
		if emitted[f] {
			continue
		}
		emitted[f] = true
		out = append(out, f)

		// Anything that imports f loses one unresolved dep.
		importers := cache.Importers[f]
		var unlocked []string
		for _, imp := range importers {
			if !present[imp] || emitted[imp] {
				continue
			}
			inDegree[imp]--
			if inDegree[imp] <= 0 {
				unlocked = append(unlocked, imp)
			}
		}
		sort.Strings(unlocked)
		queue = append(queue, unlocked...)
	}

	// Any files left unemitted are in cycles; append them lex-sorted so the tour
	// is total.
	var leftover []string
	for _, f := range files {
		if !emitted[f] {
			leftover = append(leftover, f)
		}
	}
	sort.Strings(leftover)
	return append(out, leftover...)
}

// RenderTour builds a TOUR.md body for the given strategy and cache.
// repoDir is used only to compute relative shard links.
func RenderTour(cache *Cache, strategy TourStrategy, repoRelPrefix string) string {
	order := strategy.Order(cache)

	var b strings.Builder
	fmt.Fprintf(&b, "# Repository Tour\n\n")
	fmt.Fprintf(&b, "**Strategy:** `%s` — %s\n\n", strategy.Name(), strategyBlurb(strategy.Name()))
	fmt.Fprintf(&b, "Read top-to-bottom. Each entry points to the file's shard, which"+
		" contains the structured [deps] / [calls] / [impact] view.\n\n")

	// Group by domain then subdomain while preserving order within each group.
	type entry struct {
		file string
		dom  string
		sub  string
	}
	entries := make([]entry, 0, len(order))
	for _, f := range order {
		dom, sub := splitDomain(cache.FileDomain[f])
		entries = append(entries, entry{file: f, dom: dom, sub: sub})
	}

	lastDom, lastSub := "", ""
	for _, e := range entries {
		if e.dom != lastDom {
			fmt.Fprintf(&b, "## Domain: %s\n\n", displayOrUnassigned(e.dom))
			lastDom = e.dom
			lastSub = ""
		}
		if e.sub != lastSub {
			if e.sub != "" {
				fmt.Fprintf(&b, "### Subdomain: %s\n\n", e.sub)
			}
			lastSub = e.sub
		}
		writeTourEntry(&b, e.file, cache, repoRelPrefix)
	}

	return b.String()
}

func writeTourEntry(b *strings.Builder, file string, cache *Cache, repoRelPrefix string) {
	imports := sortedUnique(cache.Imports[file])
	importers := sortedUnique(cache.Importers[file])
	risk := riskFor(file, cache)

	fmt.Fprintf(b, "- **%s**\n", file)
	if len(imports) > 0 {
		fmt.Fprintf(b, "  reads: %s\n", joinTrunc(imports, 4))
	}
	if len(importers) > 0 {
		fmt.Fprintf(b, "  read by: %s\n", joinTrunc(importers, 4))
	}
	fmt.Fprintf(b, "  risk: %s · [shard](%s)\n\n", risk, shardLink(file, repoRelPrefix))
}

func shardLink(file, prefix string) string {
	if prefix == "" {
		return ShardFilename(file)
	}
	return filepath.ToSlash(filepath.Join(prefix, ShardFilename(file)))
}

func joinTrunc(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:n], ", ") + fmt.Sprintf(", … (+%d)", len(items)-n)
}

func splitDomain(d string) (string, string) {
	if d == "" {
		return "", ""
	}
	if i := strings.Index(d, "/"); i >= 0 {
		return d[:i], d[i+1:]
	}
	return d, ""
}

func displayOrUnassigned(s string) string {
	if s == "" {
		return "Unassigned"
	}
	return s
}

// riskFor is a narrow re-derivation of the impact section's risk tier for the
// tour line. It matches renderImpactSection's thresholds so tour and shard stay
// in agreement.
func riskFor(file string, cache *Cache) string {
	transitive := cache.TransitiveDependents(file)
	domains := map[string]bool{}
	if d := cache.FileDomain[file]; d != "" {
		domains[d] = true
	}
	for f := range transitive {
		if d := cache.FileDomain[f]; d != "" {
			domains[d] = true
		}
	}
	switch {
	case len(transitive) > 20 || len(domains) > 2:
		return "HIGH"
	case len(transitive) > 5 || len(domains) > 1:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func strategyBlurb(name string) string {
	switch name {
	case "topo":
		return "reverse-topological over the import graph (leaves first, roots last)"
	default:
		return "custom ordering"
	}
}

// WriteTour writes TOUR.md to .supermodel/TOUR.md inside repoDir.
func WriteTour(repoDir string, cache *Cache, strategy TourStrategy, dryRun bool) (string, error) {
	outDir := filepath.Join(repoDir, ".supermodel")
	outPath := filepath.Join(outDir, "TOUR.md")

	// Shards live next to source files, so from .supermodel/TOUR.md the link to
	// src/foo.graph.ts is ../src/foo.graph.ts.
	body := RenderTour(cache, strategy, "..")

	if dryRun {
		fmt.Printf("  [dry-run] would write %s (%d bytes)\n", outPath, len(body))
		return outPath, nil
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create tour dir: %w", err)
	}
	tmp := outPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return "", fmt.Errorf("write tour: %w", err)
	}
	if err := os.Rename(tmp, outPath); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("finalize tour: %w", err)
	}
	return outPath, nil
}
