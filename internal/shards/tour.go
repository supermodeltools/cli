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

// BFSSeedStrategy walks the undirected import graph outward from a seed file
// in BFS order. Only files reachable from the seed are emitted.
type BFSSeedStrategy struct{ Seed string }

func (BFSSeedStrategy) Name() string { return "bfs-seed" }

func (s BFSSeedStrategy) Order(cache *Cache) []string {
	return seededTraversal(cache, s.Seed, true)
}

// DFSSeedStrategy walks the undirected import graph from a seed file in DFS
// order. Only files reachable from the seed are emitted.
type DFSSeedStrategy struct{ Seed string }

func (DFSSeedStrategy) Name() string { return "dfs-seed" }

func (s DFSSeedStrategy) Order(cache *Cache) []string {
	return seededTraversal(cache, s.Seed, false)
}

// CentralityStrategy orders files by transitive-dependent count descending
// (the "blast radius" of a change). Most-depended-on files come first;
// lex-ascending breaks ties.
type CentralityStrategy struct{}

func (CentralityStrategy) Name() string { return "centrality" }

func (CentralityStrategy) Order(cache *Cache) []string {
	files := cache.SourceFiles()
	sort.Strings(files)
	type scored struct {
		file  string
		score int
	}
	scores := make([]scored, len(files))
	for i, f := range files {
		scores[i] = scored{file: f, score: len(cache.TransitiveDependents(f))}
	}
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].score != scores[j].score {
			return scores[i].score > scores[j].score
		}
		return scores[i].file < scores[j].file
	})
	out := make([]string, len(scores))
	for i, s := range scores {
		out[i] = s.file
	}
	return out
}

// seededTraversal walks the undirected import graph from seed. bfs=true for
// BFS, false for DFS. Neighbors are visited in lex order for determinism.
// Returns empty slice if seed is not present in the cache.
func seededTraversal(cache *Cache, seed string, bfs bool) []string {
	files := cache.SourceFiles()
	present := make(map[string]bool, len(files))
	for _, f := range files {
		present[f] = true
	}
	if !present[seed] {
		return nil
	}

	visited := map[string]bool{seed: true}
	var out []string
	frontier := []string{seed}

	for len(frontier) > 0 {
		var current string
		if bfs {
			current = frontier[0]
			frontier = frontier[1:]
		} else {
			current = frontier[len(frontier)-1]
			frontier = frontier[:len(frontier)-1]
		}
		// Emit on pop so the output reflects visit order, not discovery order.
		// This is what makes DFS descend one branch fully before crossing.
		out = append(out, current)

		neighbors := map[string]bool{}
		for _, n := range cache.Imports[current] {
			if present[n] {
				neighbors[n] = true
			}
		}
		for _, n := range cache.Importers[current] {
			if present[n] {
				neighbors[n] = true
			}
		}
		sorted := make([]string, 0, len(neighbors))
		for n := range neighbors {
			sorted = append(sorted, n)
		}
		sort.Strings(sorted)
		if !bfs {
			// Reverse so lex-smallest neighbor is popped first after the push.
			for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
		for _, n := range sorted {
			if visited[n] {
				continue
			}
			visited[n] = true
			frontier = append(frontier, n)
		}
	}
	return out
}

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
	case "bfs-seed":
		return "breadth-first walk outward from the seed file"
	case "dfs-seed":
		return "depth-first walk outward from the seed file"
	case "centrality":
		return "files with the largest blast radius first (most transitively depended-on)"
	default:
		return "custom ordering"
	}
}

// approxTokens estimates the token count of s using the 4-chars-per-token
// heuristic. Good enough for sizing chapter boundaries — no tokenizer needed.
func approxTokens(s string) int {
	return (len(s) + 3) / 4
}

// ChunkTour splits a rendered tour body into chapters at file-entry boundaries
// so each chapter fits within budgetTokens. Each chapter gets a "Chapter N of M"
// header prepended. Returns a single-element slice when budgetTokens <= 0 or the
// body already fits.
func ChunkTour(body string, budgetTokens int) []string {
	if budgetTokens <= 0 || approxTokens(body) <= budgetTokens {
		return []string{body}
	}

	// File entries begin with "- **". Keep the preamble (everything before the
	// first entry) glued to the first chapter as a header, and split entries
	// into chapters.
	const entryMarker = "\n- **"
	idx := strings.Index(body, entryMarker)
	if idx < 0 {
		return []string{body}
	}
	preamble := body[:idx+1] // include trailing \n
	rest := body[idx+1:]

	// Split entries on blank line (entries are separated by "\n\n").
	type domainedEntry struct {
		heading string // most recent "## Domain" or "### Subdomain" heading block
		text    string // the "- **..." entry
	}
	var entries []domainedEntry
	var currentHeading strings.Builder
	for _, block := range strings.Split(rest, "\n\n") {
		block = strings.TrimRight(block, "\n")
		if block == "" {
			continue
		}
		if strings.HasPrefix(block, "## ") || strings.HasPrefix(block, "### ") {
			currentHeading.WriteString(block)
			currentHeading.WriteString("\n\n")
			continue
		}
		if strings.HasPrefix(block, "- **") {
			entries = append(entries, domainedEntry{heading: currentHeading.String(), text: block})
			currentHeading.Reset()
		}
	}

	var chapters [][]domainedEntry
	var currentChapter []domainedEntry
	currentSize := approxTokens(preamble)
	for _, e := range entries {
		entrySize := approxTokens(e.heading) + approxTokens(e.text) + 2
		if len(currentChapter) > 0 && currentSize+entrySize > budgetTokens {
			chapters = append(chapters, currentChapter)
			currentChapter = nil
			currentSize = approxTokens(preamble)
		}
		currentChapter = append(currentChapter, e)
		currentSize += entrySize
	}
	if len(currentChapter) > 0 {
		chapters = append(chapters, currentChapter)
	}

	out := make([]string, len(chapters))
	total := len(chapters)
	for i, chapter := range chapters {
		var b strings.Builder
		b.WriteString(preamble)
		fmt.Fprintf(&b, "> Chapter %d of %d", i+1, total)
		if i > 0 {
			fmt.Fprintf(&b, " · [prev](TOUR.%02d.md)", i)
		}
		if i < total-1 {
			fmt.Fprintf(&b, " · [next](TOUR.%02d.md)", i+2)
		}
		b.WriteString("\n\n")
		lastHeading := ""
		for _, e := range chapter {
			if e.heading != "" && e.heading != lastHeading {
				b.WriteString(e.heading)
				lastHeading = e.heading
			}
			b.WriteString(e.text)
			b.WriteString("\n\n")
		}
		out[i] = strings.TrimRight(b.String(), "\n") + "\n"
	}
	return out
}

// WriteTour writes TOUR.md to .supermodel/TOUR.md inside repoDir.
// When budgetTokens > 0 and the body exceeds the budget, the tour is split
// into TOUR.01.md, TOUR.02.md, ... and TOUR.md becomes an index file.
func WriteTour(repoDir string, cache *Cache, strategy TourStrategy, budgetTokens int, dryRun bool) (string, error) {
	outDir := filepath.Join(repoDir, ".supermodel")
	outPath := filepath.Join(outDir, "TOUR.md")

	// Shards live next to source files, so from .supermodel/TOUR.md the link to
	// src/foo.graph.ts is ../src/foo.graph.ts.
	body := RenderTour(cache, strategy, "..")
	chapters := ChunkTour(body, budgetTokens)

	if dryRun {
		if len(chapters) == 1 {
			fmt.Printf("  [dry-run] would write %s (%d bytes)\n", outPath, len(body))
		} else {
			fmt.Printf("  [dry-run] would write %s + %d chapters (%d bytes total)\n", outPath, len(chapters), len(body))
		}
		return outPath, nil
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create tour dir: %w", err)
	}

	writeFile := func(name, content string) error {
		full := filepath.Join(outDir, name)
		tmp := full + ".tmp"
		if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, full); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		return nil
	}

	if len(chapters) == 1 {
		if err := writeFile("TOUR.md", chapters[0]); err != nil {
			return "", fmt.Errorf("write tour: %w", err)
		}
		return outPath, nil
	}

	// Multi-chapter: write TOUR.NN.md files and an index TOUR.md.
	for i, chapter := range chapters {
		if err := writeFile(fmt.Sprintf("TOUR.%02d.md", i+1), chapter); err != nil {
			return "", fmt.Errorf("write chapter: %w", err)
		}
	}
	var idx strings.Builder
	fmt.Fprintf(&idx, "# Repository Tour — Index\n\n")
	fmt.Fprintf(&idx, "**Strategy:** `%s` · %d chapters\n\n", strategy.Name(), len(chapters))
	fmt.Fprintf(&idx, "Read chapters in order; each fits within the token budget.\n\n")
	for i := range chapters {
		fmt.Fprintf(&idx, "- [Chapter %d](TOUR.%02d.md)\n", i+1, i+1)
	}
	if err := writeFile("TOUR.md", idx.String()); err != nil {
		return "", fmt.Errorf("write tour index: %w", err)
	}
	return outPath, nil
}
