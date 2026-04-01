package deadcode

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/supermodeltools/cli/internal/analyze"
	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

// Options configures the dead-code command.
type Options struct {
	Force          bool   // bypass cache
	Output         string // "human" | "json"
	IncludeExports bool   // include exported (public) functions in results
}

// Result is a function with no detected callers.
type Result struct {
	Name string `json:"name"`
	File string `json:"file"`
}

// Run finds functions with no incoming call edges and prints them.
func Run(ctx context.Context, cfg *config.Config, dir string, opts Options) error {
	g, _, err := analyze.GetGraph(ctx, cfg, dir, opts.Force)
	if err != nil {
		return err
	}
	results := findDeadCode(g, opts.IncludeExports)
	return printResults(os.Stdout, results, ui.ParseFormat(opts.Output))
}

// findDeadCode returns Function nodes that have no incoming CALLS relationships.
// Entry points (main, init, test functions, and — by default — exported
// functions) are excluded because they are reachable by definition.
func findDeadCode(g *api.Graph, includeExports bool) []Result {
	// Build set of function node IDs that receive at least one CALLS edge.
	called := make(map[string]bool)
	for _, rel := range g.Rels() {
		if rel.Type == "CALLS" || rel.Type == "CONTAINS_CALL" {
			called[rel.EndNode] = true
		}
	}

	var results []Result
	for _, n := range g.NodesByLabel("Function") {
		if called[n.ID] {
			continue
		}
		name := n.Prop("name", "qualifiedName")
		file := n.Prop("file", "path")

		if isEntryPoint(name, file, includeExports) {
			continue
		}

		results = append(results, Result{Name: name, File: file})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].File != results[j].File {
			return results[i].File < results[j].File
		}
		return results[i].Name < results[j].Name
	})
	return results
}

// isEntryPoint reports whether a function should be excluded from dead-code results.
func isEntryPoint(name, file string, includeExports bool) bool {
	// Strip any receiver prefix (e.g. "(*Server).Start" → "Start")
	bare := name
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		bare = name[idx+1:]
	}
	bare = strings.TrimPrefix(bare, "*")

	if bare == "main" || bare == "init" {
		return true
	}
	// Test functions (TestX, BenchmarkX, FuzzX, ExampleX)
	for _, prefix := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
		if strings.HasPrefix(bare, prefix) {
			return true
		}
	}
	// Exported functions are reachable by callers outside the repo
	if !includeExports && len(bare) > 0 && unicode.IsUpper(rune(bare[0])) {
		return true
	}
	// Files ending in _test.go — everything in there is an entry point
	if strings.HasSuffix(file, "_test.go") {
		return true
	}
	return false
}

func printResults(w io.Writer, results []Result, fmt_ ui.Format) error {
	if fmt_ == ui.FormatJSON {
		return ui.JSON(w, results)
	}
	if len(results) == 0 {
		fmt.Fprintln(w, "No dead code detected.")
		return nil
	}
	rows := make([][]string, len(results))
	for i, r := range results {
		rows[i] = []string{r.Name, r.File}
	}
	ui.Table(w, []string{"FUNCTION", "FILE"}, rows)
	fmt.Fprintf(w, "\n%d unreachable function(s) found.\n", len(results))
	return nil
}
