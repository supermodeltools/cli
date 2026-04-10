package focus

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/supermodeltools/cli/internal/analyze"
	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

// Options configures the focus command.
type Options struct {
	Force        bool   // bypass cache
	Output       string // "markdown" | "json"
	Depth        int    // import traversal depth (default 1)
	IncludeTypes bool   // include type/class nodes
}

// Slice is the token-efficient graph slice for a single file.
type Slice struct {
	File      string     `json:"file"`
	Imports   []string   `json:"imports"`
	Functions []Function `json:"functions"`
	CalledBy  []Call     `json:"called_by"`
	Types     []Type     `json:"types,omitempty"`
	TokenHint int        `json:"approx_tokens"` // rough estimate
}

// Function is a function defined in the focused file.
type Function struct {
	Name    string   `json:"name"`
	Callees []string `json:"calls,omitempty"` // functions this fn calls
}

// Call is a reference to this file from an external caller.
type Call struct {
	Caller string `json:"caller"`
	File   string `json:"file"`
}

// Type is a type/class declared in or used by the focused file.
type Type struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "class" | "interface" | "type"
	File string `json:"file"`
}

// Run extracts a focused graph slice for target and prints it.
func Run(ctx context.Context, cfg *config.Config, dir, target string, opts Options) error {
	g, _, err := getGraph(ctx, cfg, dir, opts.Force)
	if err != nil {
		return err
	}
	depth := opts.Depth
	if depth == 0 {
		depth = 1
	}
	sl := extract(g, target, depth, opts.IncludeTypes)
	if sl == nil {
		return fmt.Errorf("file not found in graph: %s (run `supermodel analyze` first)", target)
	}
	return render(os.Stdout, sl, opts.Output)
}

// extract builds the Slice for the target file.
func extract(g *api.Graph, target string, depth int, includeTypes bool) *Slice {
	target = strings.TrimPrefix(target, "./")

	// Find the target file node.
	var fileNode *api.Node
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if !n.HasLabel("File") {
			continue
		}
		p := n.Prop("path", "name", "file")
		if pathMatches(p, target) {
			fileNode = n
			break
		}
	}
	if fileNode == nil {
		return nil
	}

	sl := &Slice{File: fileNode.Prop("path", "name", "file")}

	// Build lookup maps once.
	nodeByID := make(map[string]*api.Node, len(g.Nodes))
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}

	rels := g.Rels()

	// 1. Direct imports (imports edges from file node, up to depth hops).
	sl.Imports = reachableImports(g, fileNode.ID, nodeByID, rels, depth)

	// 2. Functions defined in this file.
	fnIDs := functionNodesForFile(fileNode.ID, rels)
	calleesOf := buildCalleesOf(rels)

	for _, fnID := range fnIDs {
		n := nodeByID[fnID]
		if n == nil {
			continue
		}
		fn := Function{Name: n.Prop("name", "qualifiedName")}
		for _, calleeID := range calleesOf[fnID] {
			if c := nodeByID[calleeID]; c != nil {
				fn.Callees = append(fn.Callees, c.Prop("name", "qualifiedName"))
			}
		}
		sort.Strings(fn.Callees)
		sl.Functions = append(sl.Functions, fn)
	}
	sort.Slice(sl.Functions, func(i, j int) bool {
		return sl.Functions[i].Name < sl.Functions[j].Name
	})

	// 3. External callers (calls edges whose target is one of our functions).
	fnIDSet := make(map[string]bool, len(fnIDs))
	for _, id := range fnIDs {
		fnIDSet[id] = true
	}
	seenCallers := make(map[string]bool)
	for _, rel := range rels {
		if rel.Type != "calls" && rel.Type != "contains_call" {
			continue
		}
		if !fnIDSet[rel.EndNode] {
			continue
		}
		callerNode := nodeByID[rel.StartNode]
		if callerNode == nil {
			continue
		}
		callerFile := callerNode.Prop("filePath", "file", "path")
		if pathMatches(callerFile, target) {
			continue // skip self-calls
		}
		key := callerNode.ID
		if seenCallers[key] {
			continue
		}
		seenCallers[key] = true
		sl.CalledBy = append(sl.CalledBy, Call{
			Caller: callerNode.Prop("name", "qualifiedName"),
			File:   callerFile,
		})
	}
	sort.Slice(sl.CalledBy, func(i, j int) bool {
		return sl.CalledBy[i].File < sl.CalledBy[j].File
	})

	// 4. Types (optional).
	if includeTypes {
		sl.Types = extractTypes(g, fileNode.ID, nodeByID, rels)
	}

	sl.TokenHint = estimateTokens(sl)
	return sl
}

// reachableImports does a BFS on IMPORTS edges from seed, up to maxDepth hops,
// and returns the file/package paths of the imported nodes.
func reachableImports(g *api.Graph, seedID string, nodeByID map[string]*api.Node, rels []api.Relationship, maxDepth int) []string {
	// Pre-index imports edges by source node to avoid O(queue × rels) inner loop.
	importEdges := make(map[string][]string, len(rels)/2)
	for _, rel := range rels {
		if rel.Type == "imports" || rel.Type == "wildcard_imports" {
			importEdges[rel.StartNode] = append(importEdges[rel.StartNode], rel.EndNode)
		}
	}

	visited := map[string]bool{seedID: true}
	queue := []string{seedID}
	var imports []string

	for depth := 0; depth < maxDepth && len(queue) > 0; depth++ {
		next := make([]string, 0)
		for _, cur := range queue {
			for _, endNode := range importEdges[cur] {
				if visited[endNode] {
					continue
				}
				visited[endNode] = true
				next = append(next, endNode)
				if n := nodeByID[endNode]; n != nil {
					p := n.Prop("path", "name", "importPath")
					if p != "" {
						imports = append(imports, p)
					}
				}
			}
		}
		queue = next
	}
	sort.Strings(imports)
	return imports
}

// functionNodesForFile returns the IDs of Function nodes associated with fileID
// via defines_function or defines relationships.
func functionNodesForFile(fileID string, rels []api.Relationship) []string {
	var ids []string
	for _, rel := range rels {
		if (rel.Type == "defines_function" || rel.Type == "defines") && rel.StartNode == fileID {
			ids = append(ids, rel.EndNode)
		}
	}
	return ids
}

// buildCalleesOf returns a map from function node ID to the IDs of functions it calls.
func buildCalleesOf(rels []api.Relationship) map[string][]string {
	m := make(map[string][]string)
	for _, rel := range rels {
		t := rel.Type
		if t == "calls" || t == "contains_call" {
			m[rel.StartNode] = append(m[rel.StartNode], rel.EndNode)
		}
	}
	return m
}

func extractTypes(g *api.Graph, fileID string, nodeByID map[string]*api.Node, rels []api.Relationship) []Type {
	var types []Type
	for _, rel := range rels {
		if rel.Type != "declares_class" && rel.Type != "defines" {
			continue
		}
		if rel.StartNode != fileID {
			continue
		}
		n := nodeByID[rel.EndNode]
		if n == nil {
			continue
		}
		kind := "type"
		for _, l := range n.Labels {
			if l == "Class" {
				kind = "class"
			}
		}
		types = append(types, Type{
			Name: n.Prop("name"),
			Kind: kind,
			File: n.Prop("file", "path"),
		})
	}
	return types
}

// estimateTokens gives a rough token count (≈ chars/4) for the slice.
func estimateTokens(sl *Slice) int {
	s := sl.File
	for _, imp := range sl.Imports {
		s += imp
	}
	for _, fn := range sl.Functions {
		s += fn.Name + strings.Join(fn.Callees, "")
	}
	for _, c := range sl.CalledBy {
		s += c.Caller + c.File
	}
	return utf8.RuneCountInString(s) / 4
}

func pathMatches(nodePath, target string) bool {
	nodePath = strings.TrimPrefix(nodePath, "./")
	target = strings.TrimPrefix(target, "./")
	return nodePath == target || strings.HasSuffix(nodePath, "/"+target)
}

// --- Output ------------------------------------------------------------------

func render(w io.Writer, sl *Slice, format string) error {
	if format == "json" {
		return ui.JSON(w, sl)
	}
	return printMarkdown(w, sl)
}

func printMarkdown(w io.Writer, sl *Slice) error {
	fmt.Fprintf(w, "## Context: %s\n\n", sl.File)

	if len(sl.Imports) > 0 {
		fmt.Fprintln(w, "### Imports")
		for _, imp := range sl.Imports {
			fmt.Fprintf(w, "- %s\n", imp)
		}
		fmt.Fprintln(w)
	}

	if len(sl.Functions) > 0 {
		fmt.Fprintln(w, "### Functions defined here")
		for _, fn := range sl.Functions {
			if len(fn.Callees) > 0 {
				fmt.Fprintf(w, "- %s  →  %s\n", fn.Name, strings.Join(fn.Callees, ", "))
			} else {
				fmt.Fprintf(w, "- %s\n", fn.Name)
			}
		}
		fmt.Fprintln(w)
	}

	if len(sl.CalledBy) > 0 {
		fmt.Fprintln(w, "### Called by")
		for _, c := range sl.CalledBy {
			if c.Caller != "" {
				fmt.Fprintf(w, "- %s  (%s)\n", c.Caller, c.File)
			} else {
				fmt.Fprintf(w, "- %s\n", c.File)
			}
		}
		fmt.Fprintln(w)
	}

	if len(sl.Types) > 0 {
		fmt.Fprintln(w, "### Types")
		for _, typ := range sl.Types {
			fmt.Fprintf(w, "- %s (%s)\n", typ.Name, typ.Kind)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "<!-- approx %d tokens -->\n", sl.TokenHint)
	return nil
}

func getGraph(ctx context.Context, cfg *config.Config, dir string, force bool) (*api.Graph, string, error) {
	return analyze.GetGraph(ctx, cfg, dir, force)
}
