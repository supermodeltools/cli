package find

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/cache"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

// Options configures the find command.
type Options struct {
	Force  bool
	Output string // "human" | "json"
	Kind   string // filter by node label: Function, File, Class, …
}

// Match is a node in the graph that matches the query.
type Match struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	File       string   `json:"file"`
	Callers    []string `json:"callers,omitempty"`
	Callees    []string `json:"callees,omitempty"`
	DefinedIn  string   `json:"defined_in,omitempty"`
}

// Run finds all graph nodes matching symbol and prints them.
func Run(ctx context.Context, cfg *config.Config, dir, symbol string, opts Options) error {
	g, err := getGraph(ctx, cfg, dir, opts.Force)
	if err != nil {
		return err
	}
	matches := search(g, symbol, opts.Kind)
	if len(matches) == 0 {
		fmt.Fprintf(os.Stderr, "No matches for %q\n", symbol)
		return nil
	}
	return printMatches(os.Stdout, matches, ui.ParseFormat(opts.Output))
}

func search(g *api.Graph, symbol, kind string) []Match {
	nodeByID := make(map[string]*api.Node, len(g.Nodes))
	for i := range g.Nodes {
		nodeByID[g.Nodes[i].ID] = &g.Nodes[i]
	}
	rels := g.Rels()

	// Build caller and callee index.
	callers := make(map[string][]string) // nodeID → [caller names]
	callees := make(map[string][]string) // nodeID → [callee names]
	defFile := make(map[string]string)   // nodeID → file that defines it

	for _, rel := range rels {
		switch rel.Type {
		case "CALLS", "CONTAINS_CALL":
			if n := nodeByID[rel.EndNode]; n != nil {
				callerNode := nodeByID[rel.StartNode]
				if callerNode != nil {
					callers[rel.EndNode] = append(callers[rel.EndNode], callerNode.Prop("name", "qualifiedName"))
				}
			}
			if n := nodeByID[rel.StartNode]; n != nil {
				calleeNode := nodeByID[rel.EndNode]
				if calleeNode != nil {
					callees[rel.StartNode] = append(callees[rel.StartNode], calleeNode.Prop("name", "qualifiedName"))
				}
			}
		case "DEFINES_FUNCTION", "DEFINES", "DECLARES_CLASS":
			defFile[rel.EndNode] = nodeByID[rel.StartNode].Prop("path", "name", "file")
		}
	}

	lower := strings.ToLower(symbol)
	var matches []Match
	for _, n := range g.Nodes {
		if kind != "" && !n.HasLabel(kind) {
			continue
		}
		name := n.Prop("name", "qualifiedName", "path")
		if !strings.Contains(strings.ToLower(name), lower) {
			continue
		}
		label := ""
		if len(n.Labels) > 0 {
			label = n.Labels[0]
		}
		m := Match{
			ID:        n.ID,
			Kind:      label,
			Name:      name,
			File:      n.Prop("file", "path"),
			DefinedIn: defFile[n.ID],
		}
		cs := callers[n.ID]
		sort.Strings(cs)
		m.Callers = cs
		ces := callees[n.ID]
		sort.Strings(ces)
		m.Callees = ces
		matches = append(matches, m)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Kind != matches[j].Kind {
			return matches[i].Kind < matches[j].Kind
		}
		return matches[i].Name < matches[j].Name
	})
	return matches
}

func printMatches(w io.Writer, matches []Match, fmt_ ui.Format) error {
	if fmt_ == ui.FormatJSON {
		return ui.JSON(w, matches)
	}
	for _, m := range matches {
		fmt.Fprintf(w, "%s  %s", m.Kind, m.Name)
		if m.File != "" {
			fmt.Fprintf(w, "  (%s)", m.File)
		}
		fmt.Fprintln(w)
		if len(m.Callers) > 0 {
			fmt.Fprintf(w, "  callers:  %s\n", strings.Join(m.Callers, ", "))
		}
		if len(m.Callees) > 0 {
			fmt.Fprintf(w, "  calls:    %s\n", strings.Join(m.Callees, ", "))
		}
	}
	fmt.Fprintf(w, "\n%d match(es) for %q\n", len(matches), matches[0].Name)
	return nil
}

// --- Graph retrieval ---------------------------------------------------------

func getGraph(ctx context.Context, cfg *config.Config, dir string, force bool) (*api.Graph, error) {
	zipPath, err := createZip(dir)
	if err != nil {
		return nil, err
	}
	defer os.Remove(zipPath)

	hash, err := cache.HashFile(zipPath)
	if err != nil {
		return nil, err
	}
	if !force {
		if g, _ := cache.Get(hash); g != nil {
			return g, nil
		}
	}

	spin := ui.Start("Analyzing repository…")
	defer spin.Stop()
	g, err := api.New(cfg).Analyze(ctx, zipPath, "find-"+hash[:16])
	if err != nil {
		return nil, err
	}
	_ = cache.Put(hash, g)
	return g, nil
}
