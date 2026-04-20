// Package memorygraph — MCP tool wrappers.
// Each exported Tool* function is the Go equivalent of the TypeScript tool
// functions in tools/memory-tools.ts and follows the same output format so
// that callers get identical text responses.
package memorygraph

import (
	"fmt"
	"strings"
)

// --- Tool option structs ------------------------------------------------------

// UpsertMemoryNodeOptions mirrors UpsertMemoryNodeOptions in the TS source.
type UpsertMemoryNodeOptions struct {
	RootDir  string
	Type     NodeType
	Label    string
	Content  string
	Metadata map[string]string
}

// CreateRelationOptions mirrors CreateRelationOptions in the TS source.
type CreateRelationOptions struct {
	RootDir  string
	SourceID string
	TargetID string
	Relation RelationType
	Weight   float64
	Metadata map[string]string
}

// SearchMemoryGraphOptions mirrors SearchMemoryGraphOptions in the TS source.
type SearchMemoryGraphOptions struct {
	RootDir    string
	Query      string
	MaxDepth   int
	TopK       int
	EdgeFilter []RelationType
}

// PruneStaleLinksOptions mirrors PruneStaleLinksOptions in the TS source.
type PruneStaleLinksOptions struct {
	RootDir   string
	Threshold float64
}

// InterlinkedItem is a single entry for AddInterlinkedContext.
type InterlinkedItem struct {
	Type     NodeType
	Label    string
	Content  string
	Metadata map[string]string
}

// AddInterlinkedContextOptions mirrors AddInterlinkedContextOptions in the TS source.
type AddInterlinkedContextOptions struct {
	RootDir  string
	Items    []InterlinkedItem
	AutoLink bool
}

// RetrieveWithTraversalOptions mirrors RetrieveWithTraversalOptions in the TS source.
type RetrieveWithTraversalOptions struct {
	RootDir     string
	StartNodeID string
	MaxDepth    int
	EdgeFilter  []RelationType
}

// --- Formatters --------------------------------------------------------------

func formatTraversalResult(r TraversalResult) string {
	content := r.Node.Content
	if len(content) > 120 {
		content = content[:120] + "..."
	}
	lines := []string{
		fmt.Sprintf("  [%s] %s (depth: %d, score: %.2f)", r.Node.Type, r.Node.Label, r.Depth, r.RelevanceScore),
		fmt.Sprintf("    Content: %s", content),
	}
	if len(r.PathRelations) > 1 {
		lines = append(lines, fmt.Sprintf("    Path: %s", strings.Join(r.PathRelations, " ")))
	}
	lines = append(lines, fmt.Sprintf("    ID: %s | Accessed: %dx", r.Node.ID, r.Node.AccessCount))
	return strings.Join(lines, "\n")
}

// --- Tool implementations ----------------------------------------------------

// ToolUpsertMemoryNode creates or updates a memory node and returns a
// human-readable summary including updated graph stats.
func ToolUpsertMemoryNode(opts UpsertMemoryNodeOptions) (string, error) {
	node, err := UpsertNode(opts.RootDir, opts.Type, opts.Label, opts.Content, opts.Metadata)
	if err != nil {
		return "", err
	}
	stats, err := GetGraphStats(opts.RootDir)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		fmt.Sprintf("✅ Memory node upserted: %s", node.Label),
		fmt.Sprintf("  ID: %s", node.ID),
		fmt.Sprintf("  Type: %s", node.Type),
		fmt.Sprintf("  Access count: %d", node.AccessCount),
		fmt.Sprintf("\nGraph: %d nodes, %d edges", stats.Nodes, stats.Edges),
	}, "\n"), nil
}

// ToolCreateRelation adds a directed edge between two existing nodes.
func ToolCreateRelation(opts CreateRelationOptions) (string, error) {
	edge, err := CreateRelation(opts.RootDir, opts.SourceID, opts.TargetID, opts.Relation, opts.Weight, opts.Metadata)
	if err != nil {
		return "", err
	}
	if edge == nil {
		return fmt.Sprintf("❌ Failed: one or both node IDs not found (source: %s, target: %s)",
			opts.SourceID, opts.TargetID), nil
	}
	stats, err := GetGraphStats(opts.RootDir)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		fmt.Sprintf("✅ Relation created: %s --[%s]--> %s", opts.SourceID, edge.Relation, opts.TargetID),
		fmt.Sprintf("  Edge ID: %s", edge.ID),
		fmt.Sprintf("  Weight: %.2f", edge.Weight),
		fmt.Sprintf("\nGraph: %d nodes, %d edges", stats.Nodes, stats.Edges),
	}, "\n"), nil
}

// ToolSearchMemoryGraph searches the graph and returns direct matches plus
// one-hop neighbors, formatted identically to the TypeScript version.
func ToolSearchMemoryGraph(opts SearchMemoryGraphOptions) (string, error) {
	result, err := SearchGraph(opts.RootDir, opts.Query, opts.MaxDepth, opts.TopK, opts.EdgeFilter)
	if err != nil {
		return "", err
	}
	if len(result.Direct) == 0 {
		return fmt.Sprintf("No memory nodes found for: %q\nGraph has %d nodes, %d edges.",
			opts.Query, result.TotalNodes, result.TotalEdges), nil
	}

	sections := []string{
		fmt.Sprintf("Memory Graph Search: %q", opts.Query),
		fmt.Sprintf("Graph: %d nodes, %d edges\n", result.TotalNodes, result.TotalEdges),
		"Direct Matches:",
	}
	for _, hit := range result.Direct {
		sections = append(sections, formatTraversalResult(hit))
	}
	if len(result.Neighbors) > 0 {
		sections = append(sections, "\nLinked Neighbors:")
		for _, neighbor := range result.Neighbors {
			sections = append(sections, formatTraversalResult(neighbor))
		}
	}
	return strings.Join(sections, "\n"), nil
}

// ToolPruneStaleLinks removes weak edges and orphaned nodes.
func ToolPruneStaleLinks(opts PruneStaleLinksOptions) (string, error) {
	result, err := PruneStaleLinks(opts.RootDir, opts.Threshold)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"🧹 Pruning complete",
		fmt.Sprintf("  Removed: %d stale links/orphan nodes", result.Removed),
		fmt.Sprintf("  Remaining edges: %d", result.Remaining),
	}, "\n"), nil
}

// ToolAddInterlinkedContext bulk-upserts nodes and optionally auto-links them
// by content similarity (threshold ≥ 0.72).
func ToolAddInterlinkedContext(opts AddInterlinkedContextOptions) (string, error) {
	items := make([]struct {
		Type     NodeType
		Label    string
		Content  string
		Metadata map[string]string
	}, len(opts.Items))
	for i, it := range opts.Items {
		items[i].Type = it.Type
		items[i].Label = it.Label
		items[i].Content = it.Content
		items[i].Metadata = it.Metadata
	}

	result, err := AddInterlinkedContext(opts.RootDir, items, opts.AutoLink)
	if err != nil {
		return "", err
	}

	sections := []string{fmt.Sprintf("✅ Added %d interlinked nodes", len(result.Nodes))}
	if len(result.Edges) > 0 {
		sections = append(sections, fmt.Sprintf("  Auto-linked: %d similarity edges (threshold ≥ 0.72)", len(result.Edges)))
	} else {
		sections = append(sections, "  No auto-links above threshold")
	}
	sections = append(sections, "\nNodes:")
	for _, n := range result.Nodes {
		sections = append(sections, fmt.Sprintf("  [%s] %s → %s", n.Type, n.Label, n.ID))
	}
	if len(result.Edges) > 0 {
		sections = append(sections, "\nEdges:")
		for _, e := range result.Edges {
			sections = append(sections, fmt.Sprintf("  %s --[%s w:%.2f]--> %s",
				e.Source, e.Relation, e.Weight, e.Target))
		}
	}

	stats, err := GetGraphStats(opts.RootDir)
	if err != nil {
		return "", err
	}
	sections = append(sections, fmt.Sprintf("\nGraph total: %d nodes, %d edges", stats.Nodes, stats.Edges))
	return strings.Join(sections, "\n"), nil
}

// ToolRetrieveWithTraversal starts a BFS from startNodeID and returns all
// reachable nodes up to maxDepth, formatted with path context and scores.
func ToolRetrieveWithTraversal(opts RetrieveWithTraversalOptions) (string, error) {
	results, err := RetrieveWithTraversal(opts.RootDir, opts.StartNodeID, opts.MaxDepth, opts.EdgeFilter)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return fmt.Sprintf("❌ Node not found: %s", opts.StartNodeID), nil
	}

	maxDepth := opts.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 2
	}

	sections := []string{
		fmt.Sprintf("Traversal from: %s (depth limit: %d)\n", results[0].Node.Label, maxDepth),
	}
	for _, r := range results {
		sections = append(sections, formatTraversalResult(r))
	}
	return strings.Join(sections, "\n"), nil
}
