package find

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/supermodeltools/cli/internal/analyze"
	"github.com/supermodeltools/cli/internal/api"
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
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	File      string   `json:"file"`
	Callers   []string `json:"callers,omitempty"`
	Callees   []string `json:"callees,omitempty"`
	DefinedIn string   `json:"defined_in,omitempty"`
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
	return printMatches(os.Stdout, matches, symbol, ui.ParseFormat(opts.Output))
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
		case "calls", "contains_call":
			if callerNode := nodeByID[rel.StartNode]; callerNode != nil {
				callers[rel.EndNode] = append(callers[rel.EndNode], callerNode.Prop("name", "qualifiedName"))
			}
			if calleeNode := nodeByID[rel.EndNode]; calleeNode != nil {
				callees[rel.StartNode] = append(callees[rel.StartNode], calleeNode.Prop("name", "qualifiedName"))
			}
		case "defines_function", "defines", "declares_class":
			if fileNode := nodeByID[rel.StartNode]; fileNode != nil {
				defFile[rel.EndNode] = fileNode.Prop("filePath", "path", "name")
			}
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
			File:      n.Prop("filePath", "file", "path"),
			DefinedIn: defFile[n.ID],
		}
		m.Callers = dedupSorted(callers[n.ID])
		m.Callees = dedupSorted(callees[n.ID])
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

func printMatches(w io.Writer, matches []Match, query string, fmt_ ui.Format) error {
	if fmt_ == ui.FormatJSON {
		return ui.JSON(w, matches)
	}
	for i := range matches {
		m := &matches[i]
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
	fmt.Fprintf(w, "\n%d match(es) for %q\n", len(matches), query)
	return nil
}

func getGraph(ctx context.Context, cfg *config.Config, dir string, force bool) (*api.Graph, error) {
	g, _, err := analyze.GetGraph(ctx, cfg, dir, force)
	return g, err
}

// dedupSorted returns a sorted, deduplicated copy of ss.
func dedupSorted(ss []string) []string {
	if len(ss) == 0 {
		return nil
	}
	cp := make([]string, len(ss))
	copy(cp, ss)
	sort.Strings(cp)
	out := cp[:1]
	for _, s := range cp[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
