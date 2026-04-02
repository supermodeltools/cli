package blastradius

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/supermodeltools/cli/internal/analyze"
	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

// Options configures the blast-radius command.
type Options struct {
	Force  bool   // bypass cache
	Output string // "human" | "json"
	Depth  int    // max traversal depth; 0 = unlimited
}

// Result is a file affected by a change to the target.
type Result struct {
	File  string `json:"file"`
	Depth int    `json:"depth"` // hops from target
}

// Run finds all files transitively affected by a change to target and prints them.
func Run(ctx context.Context, cfg *config.Config, dir, target string, opts Options) error {
	g, _, err := analyze.GetGraph(ctx, cfg, dir, opts.Force)
	if err != nil {
		return err
	}
	results, err := findBlastRadius(g, dir, target, opts.Depth)
	if err != nil {
		return err
	}
	return printResults(os.Stdout, target, results, ui.ParseFormat(opts.Output))
}

// findBlastRadius performs a reverse BFS on IMPORTS edges starting from target.
// It returns all File nodes that transitively import the target file, sorted by
// hop distance from the origin.
func findBlastRadius(g *api.Graph, repoDir, target string, maxDepth int) ([]Result, error) {
	// Normalise target to a repo-relative slash path for comparison.
	targetRel := normalise(repoDir, target)

	// Find seed nodes: File nodes whose path matches the target.
	var seeds []string
	for _, n := range g.NodesByLabel("File") {
		if pathMatches(n.Prop("path", "name", "file"), targetRel) {
			seeds = append(seeds, n.ID)
		}
	}
	if len(seeds) == 0 {
		return nil, fmt.Errorf("file not found in graph: %s (run `supermodel analyze` to refresh)", target)
	}

	// Build reverse adjacency: nodeID → set of node IDs that import it.
	importedBy := make(map[string][]string)
	for _, rel := range g.Rels() {
		if rel.Type == "imports" || rel.Type == "wildcard_imports" {
			importedBy[rel.EndNode] = append(importedBy[rel.EndNode], rel.StartNode)
		}
	}

	// BFS from seeds following reverse IMPORTS edges.
	visited := make(map[string]int) // nodeID → depth first seen
	queue := append([]string(nil), seeds...)
	for _, s := range seeds {
		visited[s] = 0
	}

	var results []Result
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		depth := visited[cur]

		if maxDepth > 0 && depth >= maxDepth {
			continue
		}

		for _, parent := range importedBy[cur] {
			if _, seen := visited[parent]; seen {
				continue
			}
			visited[parent] = depth + 1
			queue = append(queue, parent)

			n, ok := g.NodeByID(parent)
			if !ok {
				continue
			}
			file := n.Prop("path", "name", "file")
			if file != "" && !pathMatches(file, targetRel) {
				results = append(results, Result{File: file, Depth: depth + 1})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Depth != results[j].Depth {
			return results[i].Depth < results[j].Depth
		}
		return results[i].File < results[j].File
	})
	return results, nil
}

func normalise(repoDir, path string) string {
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(repoDir, path)
		if err == nil {
			path = rel
		}
	}
	return filepath.ToSlash(strings.TrimPrefix(path, "./"))
}

func pathMatches(nodePath, target string) bool {
	nodePath = filepath.ToSlash(nodePath)
	return nodePath == target ||
		strings.HasSuffix(nodePath, "/"+target) ||
		strings.HasSuffix(nodePath, target)
}

func printResults(w io.Writer, target string, results []Result, fmt_ ui.Format) error {
	if fmt_ == ui.FormatJSON {
		return ui.JSON(w, map[string]any{"target": target, "affected": results})
	}
	if len(results) == 0 {
		fmt.Fprintf(w, "No files are affected by changes to %s.\n", target)
		return nil
	}
	rows := make([][]string, len(results))
	for i, r := range results {
		rows[i] = []string{r.File, fmt.Sprintf("%d", r.Depth)}
	}
	ui.Table(w, []string{"AFFECTED FILE", "HOPS"}, rows)
	fmt.Fprintf(w, "\n%d file(s) affected by changes to %s.\n", len(results), target)
	return nil
}
