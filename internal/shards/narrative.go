package shards

import (
	"fmt"
	"sort"
	"strings"
)

// RenderNarrative produces a prose preamble describing a file's place in the
// graph as sentences rather than structured arrows. The output is one comment
// block (each line prefixed with `prefix`) covering: domain/subdomain,
// imports/importers counts with a few named examples, intra-file functions
// and their call adjacency, and risk tier.
//
// Returns "" if the file has no meaningful prose to render (no imports,
// importers, or functions). The result does NOT include a trailing blank line;
// callers compose it with the structured sections.
func RenderNarrative(filePath string, cache *Cache, prefix string) string {
	imports := sortedUnique(cache.Imports[filePath])
	importers := sortedUnique(cache.Importers[filePath])

	var fnNames []string
	var fnByName []*FuncInfo
	for _, fn := range cache.FnByID {
		if fn.File == filePath {
			fnByName = append(fnByName, fn)
		}
	}
	sort.Slice(fnByName, func(i, j int) bool {
		if fnByName[i].Name != fnByName[j].Name {
			return fnByName[i].Name < fnByName[j].Name
		}
		return fnByName[i].ID < fnByName[j].ID
	})
	for _, fn := range fnByName {
		fnNames = append(fnNames, fn.Name)
	}

	if len(imports) == 0 && len(importers) == 0 && len(fnNames) == 0 {
		return ""
	}

	var sentences []string

	domain := cache.FileDomain[filePath]
	openSentence := fmt.Sprintf("This file (%s) sits in the graph as follows:", filePath)
	if domain != "" {
		dom, sub := splitDomain(domain)
		if sub != "" {
			openSentence = fmt.Sprintf("This file (%s) belongs to domain %s / subdomain %s.", filePath, dom, sub)
		} else {
			openSentence = fmt.Sprintf("This file (%s) belongs to domain %s.", filePath, dom)
		}
	}
	sentences = append(sentences, openSentence)

	if len(imports) > 0 {
		sentences = append(sentences, fmt.Sprintf(
			"It imports %d file(s): %s.", len(imports), joinTrunc(imports, 3)))
	}
	if len(importers) > 0 {
		sentences = append(sentences, fmt.Sprintf(
			"It is imported by %d file(s): %s.", len(importers), joinTrunc(importers, 3)))
	}

	if len(fnByName) > 0 {
		sentences = append(sentences, fmt.Sprintf(
			"It defines %d function(s): %s.", len(fnByName), joinTrunc(fnNames, 5)))
		// Add call adjacency as prose for up to the first few functions.
		maxFns := 4
		if len(fnByName) < maxFns {
			maxFns = len(fnByName)
		}
		for _, fn := range fnByName[:maxFns] {
			fnProse := fnAdjacencySentence(fn, cache)
			if fnProse != "" {
				sentences = append(sentences, fnProse)
			}
		}
	}

	risk := riskFor(filePath, cache)
	transitiveCount := len(cache.TransitiveDependents(filePath))
	sentences = append(sentences, fmt.Sprintf(
		"Risk: %s (%d transitive dependent(s)).", risk, transitiveCount))

	// Render as a comment block.
	var b strings.Builder
	b.WriteString(prefix + " Narrative:\n")
	for _, s := range sentences {
		b.WriteString(prefix + " " + s + "\n")
	}
	return b.String()
}

func fnAdjacencySentence(fn *FuncInfo, cache *Cache) string {
	callers := cache.Callers[fn.ID]
	callees := cache.Callees[fn.ID]
	if len(callers) == 0 && len(callees) == 0 {
		return fmt.Sprintf("  %s has no recorded callers or callees.", fn.Name)
	}
	var parts []string
	if len(callers) > 0 {
		names := uniqueCallerNames(callers, cache)
		parts = append(parts, fmt.Sprintf("is called by %s", joinTrunc(names, 3)))
	}
	if len(callees) > 0 {
		names := uniqueCallerNames(callees, cache)
		parts = append(parts, fmt.Sprintf("calls %s", joinTrunc(names, 3)))
	}
	return fmt.Sprintf("  %s %s.", fn.Name, strings.Join(parts, " and "))
}

func uniqueCallerNames(refs []CallerRef, cache *Cache) []string {
	seen := make(map[string]bool, len(refs))
	var out []string
	for _, r := range refs {
		n := cache.FuncName(r.FuncID)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
