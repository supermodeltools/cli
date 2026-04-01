package graph

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/supermodeltools/cli/internal/analyze"
	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

// Options configures the graph command.
type Options struct {
	Force  bool   // bypass cache
	Output string // "human" | "json" | "dot"
	Filter string // filter by label: "File" | "Function" | etc.
}

// Run fetches or loads the graph for dir and renders it.
func Run(ctx context.Context, cfg *config.Config, dir string, opts Options) error {
	g, _, err := analyze.GetGraph(ctx, cfg, dir, opts.Force)
	if err != nil {
		return err
	}
	return printGraph(os.Stdout, g, opts)
}

func printGraph(w io.Writer, g *api.Graph, opts Options) error {
	switch opts.Output {
	case "json":
		return ui.JSON(w, g)

	case "dot":
		return writeDOT(w, g, opts.Filter)

	default:
		return writeHuman(w, g, opts.Filter)
	}
}

func writeHuman(w io.Writer, g *api.Graph, filter string) error {
	nodes := g.Nodes
	if filter != "" {
		nodes = g.NodesByLabel(filter)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})

	rows := make([][]string, 0, len(nodes))
	for _, n := range nodes {
		label := ""
		if len(n.Labels) > 0 {
			label = n.Labels[0]
		}
		name := n.Prop("name", "path", "file")
		rows = append(rows, []string{n.ID[:min(len(n.ID), 12)], label, name})
	}
	ui.Table(w, []string{"ID", "LABEL", "NAME"}, rows)

	rels := g.Rels()
	fmt.Fprintf(w, "\n%d nodes, %d relationships", len(nodes), len(rels))
	if filter != "" {
		fmt.Fprintf(w, " (filtered by label: %s)", filter)
	}
	fmt.Fprintln(w)
	return nil
}

func writeDOT(w io.Writer, g *api.Graph, filter string) error {
	fmt.Fprintln(w, "digraph supermodel {")
	fmt.Fprintln(w, "  rankdir=LR;")
	fmt.Fprintln(w, "  node [shape=box fontname=monospace];")

	// Node labels
	nodeLabel := make(map[string]string, len(g.Nodes))
	for _, n := range g.Nodes {
		if filter != "" && !n.HasLabel(filter) {
			continue
		}
		name := n.Prop("name", "path", "file")
		if name == "" {
			name = n.ID
		}
		safe := dotEscape(name)
		fmt.Fprintf(w, "  %q [label=%q];\n", n.ID, safe)
		nodeLabel[n.ID] = name
	}

	// Edges
	for _, rel := range g.Rels() {
		if _, ok := nodeLabel[rel.StartNode]; !ok {
			continue
		}
		if _, ok := nodeLabel[rel.EndNode]; !ok {
			continue
		}
		fmt.Fprintf(w, "  %q -> %q [label=%q];\n", rel.StartNode, rel.EndNode, rel.Type)
	}

	fmt.Fprintln(w, "}")
	return nil
}

func dotEscape(s string) string {
	if len(s) > 40 {
		s = "…" + s[len(s)-39:]
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
