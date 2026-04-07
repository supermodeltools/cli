package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CommentPrefix returns the language-appropriate comment prefix.
func CommentPrefix(ext string) string {
	switch ext {
	case ".py", ".rb":
		return "#"
	default:
		return "//"
	}
}

// SidecarFilename generates the .graph sidecar path.
// Example: "src/Foo.tsx" → "src/Foo.graph.tsx"
func SidecarFilename(sourcePath string) string {
	ext := filepath.Ext(sourcePath)
	stem := strings.TrimSuffix(sourcePath, ext)
	return stem + ".graph" + ext
}

// Header returns the @generated header line.
func Header(prefix string) string {
	return prefix + " @generated supermodel-sidecar — do not edit\n"
}

// RenderGraph produces a combined .graph sidecar with deps, calls, and impact sections.
func RenderGraph(filePath string, cache *Cache, prefix string) string {
	deps := renderDepsSection(filePath, cache, prefix)
	calls := renderCallsSection(filePath, cache, prefix)
	impact := renderImpactSection(filePath, cache, prefix)

	var sections []string
	if deps != "" {
		sections = append(sections, deps)
	}
	if calls != "" {
		sections = append(sections, calls)
	}
	if impact != "" {
		sections = append(sections, impact)
	}

	if len(sections) == 0 {
		return ""
	}
	return strings.Join(sections, "\n") + "\n"
}

func renderCallsSection(filePath string, cache *Cache, prefix string) string {
	type fnEntry struct {
		id   string
		name string
	}
	var fns []fnEntry
	for id, fn := range cache.FnByID {
		if fn.File == filePath {
			fns = append(fns, fnEntry{id, fn.Name})
		}
	}
	if len(fns) == 0 {
		return ""
	}

	sort.Slice(fns, func(i, j int) bool { return fns[i].name < fns[j].name })

	var lines []string
	lines = append(lines, fmt.Sprintf("%s [calls]", prefix))
	for _, fe := range fns {
		for _, caller := range cache.Callers[fe.id] {
			callerName := cache.FuncName(caller.FuncID)
			loc := formatLoc(caller.File, caller.Line)
			lines = append(lines, fmt.Sprintf("%s %s ← %s    %s", prefix, fe.name, callerName, loc))
		}
		for _, callee := range cache.Callees[fe.id] {
			calleeName := cache.FuncName(callee.FuncID)
			loc := formatLoc(callee.File, callee.Line)
			lines = append(lines, fmt.Sprintf("%s %s → %s    %s", prefix, fe.name, calleeName, loc))
		}
	}

	if len(lines) == 1 { // only the header
		return ""
	}
	return strings.Join(lines, "\n")
}

func renderDepsSection(filePath string, cache *Cache, prefix string) string {
	imported := cache.Imports[filePath]
	importedBy := cache.Importers[filePath]

	if len(imported) == 0 && len(importedBy) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s [deps]", prefix))
	for _, imp := range sortedUnique(imported) {
		lines = append(lines, fmt.Sprintf("%s imports     %s", prefix, imp))
	}
	for _, imp := range sortedUnique(importedBy) {
		lines = append(lines, fmt.Sprintf("%s imported-by %s", prefix, imp))
	}

	return strings.Join(lines, "\n")
}

func renderImpactSection(filePath string, cache *Cache, prefix string) string { //nolint:gocyclo // risk/domain/impact calculation has many branches by design; splitting would obscure the scoring logic
	directImporters := cache.Importers[filePath]
	directCallerFiles := make(map[string]bool)

	for id, fn := range cache.FnByID {
		if fn.File != filePath {
			continue
		}
		for _, caller := range cache.Callers[id] {
			if caller.File != "" && caller.File != filePath {
				directCallerFiles[caller.File] = true
			}
		}
	}

	directFiles := make(map[string]bool)
	for _, f := range directImporters {
		directFiles[f] = true
	}
	for f := range directCallerFiles {
		directFiles[f] = true
	}
	directCount := len(directFiles)

	transitive := cache.TransitiveDependents(filePath)
	transitiveCount := len(transitive)

	if directCount == 0 && transitiveCount == 0 {
		return ""
	}

	domains := make(map[string]bool)
	if d := cache.FileDomain[filePath]; d != "" {
		domains[d] = true
	}
	allAffected := make(map[string]bool)
	for f := range directFiles {
		allAffected[f] = true
	}
	for f := range transitive {
		allAffected[f] = true
	}
	for f := range allAffected {
		if d := cache.FileDomain[f]; d != "" {
			domains[d] = true
		}
	}

	var risk string
	switch {
	case transitiveCount > 20 || len(domains) > 2:
		risk = "HIGH"
	case transitiveCount > 5 || len(domains) > 1:
		risk = "MEDIUM"
	default:
		risk = "LOW"
	}

	lines := []string{
		fmt.Sprintf("%s [impact]", prefix),
		fmt.Sprintf("%s risk        %s", prefix, risk),
	}
	if len(domains) > 0 {
		lines = append(lines, fmt.Sprintf("%s domains     %s", prefix, strings.Join(sortedBoolKeys(domains), " · ")))
	}
	lines = append(lines,
		fmt.Sprintf("%s direct      %d", prefix, directCount),
		fmt.Sprintf("%s transitive  %d", prefix, transitiveCount),
	)
	if directCount > 0 {
		lines = append(lines, fmt.Sprintf("%s affects     %s", prefix, strings.Join(sortedBoolKeys(directFiles), " · ")))
	}

	return strings.Join(lines, "\n")
}

// WriteSidecar writes a sidecar file with path traversal protection.
func WriteSidecar(repoDir, sidecarPath, content string, dryRun bool) error {
	full, err := filepath.Abs(filepath.Join(repoDir, sidecarPath))
	if err != nil {
		return err
	}
	repoAbs, err := filepath.Abs(repoDir)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(full, repoAbs+string(filepath.Separator)) && full != repoAbs {
		return fmt.Errorf("path traversal blocked: %s", sidecarPath)
	}

	if dryRun {
		fmt.Printf("  [dry-run] would write %s\n", full)
		return nil
	}

	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, full)
}

// RenderAll generates and writes .graph sidecars for the given source files.
// Returns the count of sidecars written.
func RenderAll(repoDir string, cache *Cache, files []string, dryRun bool) (int, error) {
	sort.Strings(files)
	written := 0

	for _, srcFile := range files {
		ext := filepath.Ext(srcFile)
		prefix := CommentPrefix(ext)
		header := Header(prefix)

		content := RenderGraph(srcFile, cache, prefix)
		if content == "" {
			continue
		}

		fullContent := header + content
		if ext == ".go" {
			fullContent = "//go:build ignore\n\npackage ignore\n" + fullContent
		}

		scPath := SidecarFilename(srcFile)
		if err := WriteSidecar(repoDir, scPath, fullContent, dryRun); err != nil {
			if strings.Contains(err.Error(), "path traversal") {
				continue
			}
			return written, err
		}
		written++
	}

	return written, nil
}

func formatLoc(file string, line int) string {
	if file != "" && line > 0 {
		return fmt.Sprintf("%s:%d", file, line)
	}
	if file != "" {
		return file
	}
	return "?"
}

func sortedUnique(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func sortedBoolKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
