package memorygraph

import (
	"fmt"
	"strings"
	"time"
)

// NodePeek is a full snapshot of a node and all its edges, returned by Peek.
type NodePeek struct {
	Node     Node
	EdgesOut []EdgePeek // edges where this node is the source
	EdgesIn  []EdgePeek // edges where this node is the target
}

// EdgePeek is a human-readable summary of a single edge and its peer node.
type EdgePeek struct {
	Edge      Edge
	PeerID    string
	PeerLabel string
	PeerType  NodeType
}

// PeekOptions controls what Peek returns.
type PeekOptions struct {
	RootDir string
	// NodeID takes priority if set.
	NodeID string
	// Label is used for lookup when NodeID is empty (first match wins).
	Label string
}

// Peek returns a full NodePeek for the requested node: its content, metadata,
// access stats, and every inbound/outbound edge with peer labels resolved.
// Returns nil if the node cannot be found.
func Peek(opts PeekOptions) (*NodePeek, error) {
	g, err := load(opts.RootDir)
	if err != nil {
		return nil, err
	}

	nodeByID := indexNodes(g)

	// Resolve target node.
	var target *Node
	if opts.NodeID != "" {
		if n, ok := nodeByID[opts.NodeID]; ok {
			target = &n
		}
	} else if opts.Label != "" {
		for i := range g.Nodes {
			if strings.EqualFold(g.Nodes[i].Label, opts.Label) {
				n := g.Nodes[i]
				target = &n
				break
			}
		}
	}

	if target == nil {
		return nil, nil //nolint:nilnil // caller checks nil to detect not-found
	}

	peek := &NodePeek{Node: *target}

	for _, e := range g.Edges {
		switch {
		case e.Source == target.ID:
			peer := nodeByID[e.Target]
			peek.EdgesOut = append(peek.EdgesOut, EdgePeek{
				Edge:      e,
				PeerID:    e.Target,
				PeerLabel: peer.Label,
				PeerType:  peer.Type,
			})
		case e.Target == target.ID:
			peer := nodeByID[e.Source]
			peek.EdgesIn = append(peek.EdgesIn, EdgePeek{
				Edge:      e,
				PeerID:    e.Source,
				PeerLabel: peer.Label,
				PeerType:  peer.Type,
			})
		}
	}

	return peek, nil
}

// PeekList returns a lightweight summary of every node in the graph —
// ID, type, label, access count, age, and edge degree — sorted by access
// count descending. Useful for scanning the graph before pruning.
func PeekList(rootDir string) ([]NodePeek, error) {
	g, err := load(rootDir)
	if err != nil {
		return nil, err
	}

	nodeByID := indexNodes(g)

	edgesOut := make(map[string][]EdgePeek, len(g.Nodes))
	edgesIn := make(map[string][]EdgePeek, len(g.Nodes))

	for _, e := range g.Edges {
		peer := nodeByID[e.Target]
		edgesOut[e.Source] = append(edgesOut[e.Source], EdgePeek{
			Edge: e, PeerID: e.Target, PeerLabel: peer.Label, PeerType: peer.Type,
		})

		peer = nodeByID[e.Source]
		edgesIn[e.Target] = append(edgesIn[e.Target], EdgePeek{
			Edge: e, PeerID: e.Source, PeerLabel: peer.Label, PeerType: peer.Type,
		})
	}

	peeks := make([]NodePeek, 0, len(g.Nodes))
	for i := range g.Nodes {
		peeks = append(peeks, NodePeek{
			Node:     g.Nodes[i],
			EdgesOut: edgesOut[g.Nodes[i].ID],
			EdgesIn:  edgesIn[g.Nodes[i].ID],
		})
	}

	// Sort by access count desc, then label asc for stable output.
	sortNodePeeks(peeks)

	return peeks, nil
}

// FormatPeek renders a NodePeek as a human-readable block suitable for
// display in a terminal or MCP tool response.
func FormatPeek(p *NodePeek) string {
	if p == nil {
		return "❌ Node not found."
	}
	n := p.Node
	age := time.Since(n.CreatedAt).Round(time.Hour)

	var b strings.Builder
	fmt.Fprintf(&b, "┌─ [%s] %s\n", n.Type, n.Label)
	fmt.Fprintf(&b, "│  ID:      %s\n", n.ID)
	fmt.Fprintf(&b, "│  Accessed: %dx  │  Age: %s  │  Updated: %s\n",
		n.AccessCount,
		age,
		n.UpdatedAt.Format("2006-01-02 15:04"),
	)
	if len(n.Metadata) > 0 {
		fmt.Fprintf(&b, "│  Metadata: %s\n", formatMetadata(n.Metadata))
	}
	fmt.Fprintf(&b, "│\n│  Content:\n│    %s\n",
		strings.ReplaceAll(n.Content, "\n", "\n│    "))

	if len(p.EdgesOut) > 0 {
		fmt.Fprintf(&b, "│\n│  Out (%d):\n", len(p.EdgesOut))
		for i := range p.EdgesOut {
			ep := &p.EdgesOut[i]
			fmt.Fprintf(&b, "│    ──[%s w:%.2f]──▶ [%s] %s\n",
				ep.Edge.Relation, ep.Edge.Weight, ep.PeerType, ep.PeerLabel)
		}
	}
	if len(p.EdgesIn) > 0 {
		fmt.Fprintf(&b, "│\n│  In (%d):\n", len(p.EdgesIn))
		for i := range p.EdgesIn {
			ep := &p.EdgesIn[i]
			fmt.Fprintf(&b, "│    [%s] %s ──[%s w:%.2f]──▶\n",
				ep.PeerType, ep.PeerLabel, ep.Edge.Relation, ep.Edge.Weight)
		}
	}
	b.WriteString("└─")
	return b.String()
}

// FormatPeekList renders a PeekList result as a compact table with one node
// per line, suitable for scanning before a prune pass.
func FormatPeekList(peeks []NodePeek) string {
	if len(peeks) == 0 {
		return "Graph is empty."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-12s  %-10s  %-32s  %7s  %4s  %4s\n",
		"TYPE", "ID (short)", "LABEL", "ACCESSED", "OUT", "IN")
	b.WriteString(strings.Repeat("─", 76) + "\n")
	for i := range peeks {
		p := &peeks[i]
		shortID := p.Node.ID
		if len(shortID) > 10 {
			shortID = shortID[:10] + "…"
		}
		label := p.Node.Label
		if len(label) > 32 {
			label = label[:31] + "…"
		}
		fmt.Fprintf(&b, "%-12s  %-10s  %-32s  %7dx  %4d  %4d\n",
			p.Node.Type, shortID, label,
			p.Node.AccessCount, len(p.EdgesOut), len(p.EdgesIn))
	}
	fmt.Fprintf(&b, "\n%d node(s) total\n", len(peeks))
	return b.String()
}

// --- helpers -----------------------------------------------------------------

func formatMetadata(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "  ")
}

func sortNodePeeks(peeks []NodePeek) {
	// Insertion sort is fine for the typical small N here.
	for i := 1; i < len(peeks); i++ {
		for j := i; j > 0; j-- {
			a, b := peeks[j-1], peeks[j]
			if a.Node.AccessCount < b.Node.AccessCount ||
				(a.Node.AccessCount == b.Node.AccessCount && a.Node.Label > b.Node.Label) {
				peeks[j-1], peeks[j] = peeks[j], peeks[j-1]
			} else {
				break
			}
		}
	}
}
