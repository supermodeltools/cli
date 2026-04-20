package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/supermodeltools/cli/internal/Memorygraph"
	"github.com/supermodeltools/cli/internal/analyze"
	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/build"
	"github.com/supermodeltools/cli/internal/cache"
	"github.com/supermodeltools/cli/internal/config"
)

// Serve starts the MCP stdio server. It reads JSON-RPC 2.0 messages from stdin
// and writes responses to stdout until the context is cancelled or stdin closes.
func Serve(ctx context.Context, cfg *config.Config, repoDir string) error {
	s := &server{cfg: cfg, dir: repoDir}
	return s.run(ctx, os.Stdin, os.Stdout)
}

// --- JSON-RPC 2.0 types ------------------------------------------------------

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	codeParseError     = -32700
	codeMethodNotFound = -32601
	codeInternalError  = -32603
)

// --- Tool definitions --------------------------------------------------------

type tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema toolSchema `json:"inputSchema"`
}

type toolSchema struct {
	Type       string                `json:"type"`
	Properties map[string]schemaProp `json:"properties,omitempty"`
	Required   []string              `json:"required,omitempty"`
}

type schemaProp struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

var tools = []tool{
	{
		Name:        "analyze",
		Description: "Upload the repository and run the full Supermodel analysis pipeline (call graph, dependency graph, domain classification). Call this first before using other tools. Results are cached locally.",
		InputSchema: toolSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"force": {Type: "boolean", Description: "Re-analyze even if a cached result exists."},
			},
		},
	},
	{
		Name:        "dead_code",
		Description: "Find unreachable functions using multi-phase static analysis. Returns candidates with confidence levels (high/medium/low), line numbers, and explanations.",
		InputSchema: toolSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"min_confidence": {Type: "string", Description: "Minimum confidence level: high, medium, or low."},
				"limit":          {Type: "integer", Description: "Maximum number of candidates to return. 0 = all."},
			},
		},
	},
	{
		Name:        "blast_radius",
		Description: "Analyze the impact of changing a file or function. Returns risk score, affected files and functions, entry points impacted, and risk factors.",
		InputSchema: toolSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"file": {Type: "string", Description: "Repo-relative path to the file (e.g. internal/api/client.go). Omit for global coupling map."},
			},
		},
	},
	{
		Name:        "get_graph",
		Description: "Return a filtered slice of the dependency or call graph as JSON. Useful for understanding how a specific part of the codebase is connected.",
		InputSchema: toolSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"label":    {Type: "string", Description: "Filter nodes by label: File, Function, Class, etc."},
				"rel_type": {Type: "string", Description: "Filter relationships by type: imports, calls, defines_function, etc."},
				"force":    {Type: "boolean", Description: "Re-analyze even if a cached result exists."},
			},
		},
	},
	 	{
 		Name:        "upsert_memory_node",
 		Description: "Upsert a typed knowledge node into the persistent memory graph.",
 		InputSchema: toolSchema{
 			Type: "object",
 			Properties: map[string]schemaProp{
 				"type":    {Type: "string", Description: "Node type: fact, concept, entity, event, procedure, context."},
 				"label":   {Type: "string", Description: "Short unique label for the node."},
 				"content": {Type: "string", Description: "Full content body of the node."},
 			},
 			Required: []string{"type", "label", "content"},
 		},
 	},
 	{
 		Name:        "create_relation",
 		Description: "Create a directed weighted edge between two memory graph nodes.",
 		InputSchema: toolSchema{
 			Type: "object",
 			Properties: map[string]schemaProp{
 				"source_id": {Type: "string", Description: "ID of the source node."},
 				"target_id": {Type: "string", Description: "ID of the target node."},
 				"relation":  {Type: "string", Description: "Relation type, e.g. related_to, depends_on, part_of."},
 				"weight":    {Type: "number", Description: "Edge weight between 0 and 1 (default 1.0)."},
 			},
 			Required: []string{"source_id", "target_id", "relation"},
 		},
 	},
 	{
		Name:        "search_memory_graph",
 		Description: "Score and retrieve nodes from the memory graph matching a query, with optional one-hop neighbor expansion.",
 		InputSchema: toolSchema{
 			Type: "object",
 			Properties: map[string]schemaProp{
 				"query":     {Type: "string", Description: "Search query string."},
 				"max_depth": {Type: "integer", Description: "Max BFS depth for neighbor expansion (default 1)."},
 				"top_k":     {Type: "integer", Description: "Maximum number of direct results to return (default 5)."},
 			},
 		Required: []string{"query"},
 		},
 	},
 	{
 		Name:        "retrieve_with_traversal",
 		Description: "BFS traversal from a start node up to maxDepth, returning visited nodes with decayed relevance scores.",
 		InputSchema: toolSchema{
 			Type: "object",
 			Properties: map[string]schemaProp{
 				"start_node_id": {Type: "string", Description: "ID of the node to start traversal from."},
 				"max_depth":     {Type: "integer", Description: "Maximum BFS depth (default 3)."},
 			},
 			Required: []string{"start_node_id"},
 		},
 	},
 	{
 		Name:        "prune_stale_links",
 		Description: "Remove edges below a weight threshold and orphaned nodes from the memory graph.",
 		InputSchema: toolSchema{
 			Type:       "object",
 			Properties: map[string]schemaProp{
 				"threshold": {Type: "number", Description: "Minimum edge weight to retain (default 0.1)."},
 			},
 		},
 	},
 	{
 		Name:        "add_interlinked_context",
 		Description: "Bulk-insert nodes and optionally auto-create similarity edges (Jaccard ≥ 0.72) between them.",
 		InputSchema: toolSchema{
 			Type: "object",
 			Properties: map[string]schemaProp{
 				"items":     {Type: "array", Description: "Array of {type, label, content, metadata} node objects to insert."},
 				"auto_link": {Type: "boolean", Description: "If true, auto-create similarity edges between inserted nodes."},
 			},
 			Required: []string{"items"},
 		},
b	},
}

// --- Server ------------------------------------------------------------------

type server struct {
	cfg   *config.Config
	dir   string
	graph *api.Graph
	hash  string
}

func (s *server) run(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4<<20), 4<<20)
	enc := json.NewEncoder(w)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{
				JSONRPC: "2.0",
				Error:   &rpcError{Code: codeParseError, Message: "parse error: " + err.Error()},
			})
			continue
		}

		result, rpcErr := s.dispatch(ctx, req.Method, req.Params)
		resp := response{JSONRPC: "2.0", ID: req.ID}
		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resp.Result = result
		}
		_ = enc.Encode(resp)
	}
	return scanner.Err()
}

func (s *server) dispatch(ctx context.Context, method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		return s.handleInitialize(), nil

	case "tools/list":
		return map[string]any{"tools": tools}, nil

	case "tools/call":
		return s.handleToolCall(ctx, params)

	case "notifications/initialized":
		return nil, nil // acknowledged, no response needed

	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + method}
	}
}

func (s *server) handleInitialize() any {
	return map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "supermodel",
			"version": "0.1.0",
		},
	}
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func (s *server) handleToolCall(ctx context.Context, params json.RawMessage) (any, *rpcError) {
	var p toolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: codeParseError, Message: err.Error()}
	}

	text, err := s.callTool(ctx, p.Name, p.Arguments)
	if err != nil {
		return nil, &rpcError{Code: codeInternalError, Message: err.Error()}
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	}, nil
}

func (s *server) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	switch name {
	case "analyze":
		return s.toolAnalyze(ctx, args)
	case "dead_code":
		return s.toolDeadCode(ctx, args)
	case "blast_radius":
		return s.toolBlastRadius(ctx, args)
	case "get_graph":
		return s.toolGetGraph(ctx, args)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	 func (s *server) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
 	switch name {
 	case "analyze":
 		return s.toolAnalyze(ctx, args)
 	case "dead_code":
 		return s.toolDeadCode(ctx, args)
 	case "blast_radius":
 		return s.toolBlastRadius(ctx, args)
 	case "get_graph":
 		return s.toolGetGraph(ctx, args)
 	case "upsert_memory_node":
 		return memorygraph.ToolUpsertMemoryNode(memorygraph.UpsertMemoryNodeOptions{
 			RootDir: s.dir,
 			Type:    memorygraph.NodeType(strArg(args, "type")),
 			Label:   strArg(args, "label"),
 			Content: strArg(args, "content"),
 		})
 	case "create_relation":
 		w := floatArg(args, "weight")
 		if w == 0 {
 			w = 1.0
 		}
 		return memorygraph.ToolCreateRelation(memorygraph.CreateRelationOptions{
 			RootDir:  s.dir,
 			SourceID: strArg(args, "source_id"),
 			TargetID: strArg(args, "target_id"),
 			Relation: memorygraph.RelationType(strArg(args, "relation")),
 			Weight:   w,
 		})
 	case "search_memory_graph":
 		topK := intArg(args, "top_k")
 		if topK == 0 {
 			topK = 5
 		}
 		return memorygraph.ToolSearchMemoryGraph(memorygraph.SearchMemoryGraphOptions{
 			RootDir:  s.dir,
 			Query:    strArg(args, "query"),
 			MaxDepth: intArg(args, "max_depth"),
 			TopK:     topK,
 		})
 	case "retrieve_with_traversal":
 		return memorygraph.ToolRetrieveWithTraversal(memorygraph.RetrieveWithTraversalOptions{
 			RootDir:     s.dir,
 			StartNodeID: strArg(args, "start_node_id"),
 			MaxDepth:    intArg(args, "max_depth"),
 		})
 	case "prune_stale_links":
 		return memorygraph.ToolPruneStaleLinks(memorygraph.PruneStaleLinksOptions{
 			RootDir:   s.dir,
 			Threshold: floatArg(args, "threshold"),
 		})
 	case "add_interlinked_context":
 		items, err := parseInterlinkedItems(args)
 		if err != nil {
 			return "", fmt.Errorf("add_interlinked_context: invalid items: %w", err)
 		}
 		return memorygraph.ToolAddInterlinkedContext(memorygraph.AddInterlinkedContextOptions{
 			RootDir:  s.dir,
 			Items:    items,
 			AutoLink: boolArg(args, "auto_link"),
 		})
 	default:
 		return "", fmt.Errorf("unknown tool: %s", name)
 	}
 }
}

// toolAnalyze uploads the repo and runs the full analysis pipeline.
func (s *server) toolAnalyze(ctx context.Context, args map[string]any) (string, error) {
	force := boolArg(args, "force")
	g, hash, err := s.getOrAnalyze(ctx, force)
	if err != nil {
		return "", err
	}
	s.graph = g
	s.hash = hash
	return fmt.Sprintf("Analysis complete.\nRepo ID: %s\nFiles: %d\nFunctions: %d\nRelationships: %d",
		g.RepoID(),
		len(g.NodesByLabel("File")),
		len(g.NodesByLabel("Function")),
		len(g.Rels()),
	), nil
}

// toolDeadCode calls the dedicated /v1/analysis/dead-code endpoint.
func (s *server) toolDeadCode(ctx context.Context, args map[string]any) (string, error) {
	minConfidence, _ := args["min_confidence"].(string)
	limit := intArg(args, "limit")

	// Check fingerprint cache (shared with `supermodel dead-code` CLI command).
	if fp, err := cache.RepoFingerprint(s.dir); err == nil {
		key := cache.AnalysisKey(fp, fmt.Sprintf("dead-code:%s:%d", minConfidence, limit), build.Version)
		var cached api.DeadCodeResult
		if hit, _ := cache.GetJSON(key, &cached); hit {
			return formatDeadCode(&cached), nil
		}
	}

	zipPath, hash, err := s.ensureZip()
	if err != nil {
		return "", err
	}
	defer os.Remove(zipPath)

	client := api.New(s.cfg)
	result, err := client.DeadCode(ctx, zipPath, "mcp-dc-"+hash[:16], minConfidence, limit)
	if err != nil {
		return "", err
	}

	if fp, err := cache.RepoFingerprint(s.dir); err == nil {
		key := cache.AnalysisKey(fp, fmt.Sprintf("dead-code:%s:%d", minConfidence, limit), build.Version)
		_ = cache.PutJSON(key, result)
	}

	return formatDeadCode(result), nil
}

// toolBlastRadius calls the dedicated /v1/analysis/impact endpoint.
func (s *server) toolBlastRadius(ctx context.Context, args map[string]any) (string, error) {
	target, _ := args["file"].(string)

	// Check fingerprint cache (shared with `supermodel blast-radius` CLI command).
	if fp, err := cache.RepoFingerprint(s.dir); err == nil {
		analysisType := "impact"
		if target != "" {
			analysisType += ":" + target
		}
		key := cache.AnalysisKey(fp, analysisType, build.Version)
		var cached api.ImpactResult
		if hit, _ := cache.GetJSON(key, &cached); hit {
			return formatImpact(&cached), nil
		}
	}

	zipPath, hash, err := s.ensureZip()
	if err != nil {
		return "", err
	}
	defer os.Remove(zipPath)

	idempotencyKey := "mcp-impact-" + hash[:16]
	if target != "" {
		idempotencyKey += "-" + target
	}

	client := api.New(s.cfg)
	result, err := client.Impact(ctx, zipPath, idempotencyKey, target, "")
	if err != nil {
		return "", err
	}

	if fp, err := cache.RepoFingerprint(s.dir); err == nil {
		analysisType := "impact"
		if target != "" {
			analysisType += ":" + target
		}
		key := cache.AnalysisKey(fp, analysisType, build.Version)
		_ = cache.PutJSON(key, result)
	}

	return formatImpact(result), nil
}

// toolGetGraph returns a filtered graph slice.
func (s *server) toolGetGraph(ctx context.Context, args map[string]any) (string, error) {
	force := boolArg(args, "force")
	g, _, err := s.getOrAnalyze(ctx, force)
	if err != nil {
		return "", err
	}
	label, _ := args["label"].(string)
	relType, _ := args["rel_type"].(string)

	out := filterGraph(g, label, relType)
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// --- Formatting helpers ------------------------------------------------------

// formatDeadCode formats a DeadCodeResult as human-readable text.
func formatDeadCode(result *api.DeadCodeResult) string {
	if len(result.DeadCodeCandidates) == 0 {
		return "No dead code detected."
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d dead code candidate(s) out of %d total declarations:\n\n",
		result.Metadata.DeadCodeCandidates, result.Metadata.TotalDeclarations)
	for i := range result.DeadCodeCandidates {
		c := &result.DeadCodeCandidates[i]
		fmt.Fprintf(&sb, "- [%s] %s:%d %s — %s\n", c.Confidence, c.File, c.Line, c.Name, c.Reason)
	}
	return sb.String()
}

// formatImpact formats an ImpactResult as human-readable text.
func formatImpact(result *api.ImpactResult) string {
	if len(result.Impacts) == 0 {
		if len(result.GlobalMetrics.MostCriticalFiles) > 0 {
			var sb strings.Builder
			sb.WriteString("Most critical files (by dependent count):\n\n")
			for i := range result.GlobalMetrics.MostCriticalFiles {
				f := &result.GlobalMetrics.MostCriticalFiles[i]
				fmt.Fprintf(&sb, "- %s (%d dependents)\n", f.File, f.DependentCount)
			}
			return sb.String()
		}
		return "No impact detected."
	}

	var sb strings.Builder
	for i := range result.Impacts {
		imp := &result.Impacts[i]
		br := &imp.BlastRadius
		fmt.Fprintf(&sb, "Target: %s\n", imp.Target.File)
		fmt.Fprintf(&sb, "Risk: %s | Direct: %d | Transitive: %d | Files: %d\n",
			br.RiskScore, br.DirectDependents, br.TransitiveDependents, br.AffectedFiles)
		for _, rf := range br.RiskFactors {
			fmt.Fprintf(&sb, "  → %s\n", rf)
		}
		if len(imp.AffectedFiles) > 0 {
			sb.WriteString("\nAffected files:\n")
			for j := range imp.AffectedFiles {
				f := &imp.AffectedFiles[j]
				fmt.Fprintf(&sb, "- %s (direct: %d, transitive: %d)\n", f.File, f.DirectDependencies, f.TransitiveDependencies)
			}
		}
		if len(imp.EntryPointsAffected) > 0 {
			sb.WriteString("\nEntry points affected:\n")
			for j := range imp.EntryPointsAffected {
				ep := &imp.EntryPointsAffected[j]
				fmt.Fprintf(&sb, "- %s %s (%s)\n", ep.File, ep.Name, ep.Type)
			}
		}
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "%d target(s) analyzed across %d files and %d functions.\n",
		result.Metadata.TargetsAnalyzed, result.Metadata.TotalFiles, result.Metadata.TotalFunctions)
	return sb.String()
}

// --- Shared helpers ----------------------------------------------------------

// getOrAnalyze returns the cached graph or runs a fresh analysis.
func (s *server) getOrAnalyze(ctx context.Context, force bool) (*api.Graph, string, error) {
	if !force && s.graph != nil {
		return s.graph, s.hash, nil
	}

	if err := s.cfg.RequireAPIKey(); err != nil {
		return nil, "", err
	}

	g, hash, err := analyze.GetGraph(ctx, s.cfg, s.dir, force)
	if err != nil {
		return nil, "", err
	}
	s.graph = g
	s.hash = hash
	return g, hash, nil
}

// ensureZip creates a repo zip and returns its path and hash.
// The caller is responsible for removing the zip file.
func (s *server) ensureZip() (zipPath, hash string, err error) {
	if err := s.cfg.RequireAPIKey(); err != nil {
		return "", "", err
	}

	zipPath, err = createZip(s.dir)
	if err != nil {
		return "", "", err
	}

	hash, err = cache.HashFile(zipPath)
	if err != nil {
		os.Remove(zipPath)
		return "", "", err
	}
	return zipPath, hash, nil
}

type graphSlice struct {
	Nodes         []api.Node         `json:"nodes"`
	Relationships []api.Relationship `json:"relationships"`
}

func filterGraph(g *api.Graph, label, relType string) graphSlice {
	nodes := g.Nodes
	if label != "" {
		nodes = g.NodesByLabel(label)
	}
	rels := g.Rels()
	// When a label filter is set, restrict relationships to only those where
	// both endpoints are within the filtered node set. Without this, the
	// returned JSON would contain relationships referencing node IDs that
	// are not present in the nodes list.
	if label != "" {
		visible := make(map[string]bool, len(nodes))
		for _, n := range nodes {
			visible[n.ID] = true
		}
		var inLabel []api.Relationship
		for _, r := range rels {
			if visible[r.StartNode] && visible[r.EndNode] {
				inLabel = append(inLabel, r)
			}
		}
		rels = inLabel
	}
	if relType != "" {
		var filtered []api.Relationship
		for _, r := range rels {
			if r.Type == relType {
				filtered = append(filtered, r)
			}
		}
		rels = filtered
	}
	return graphSlice{Nodes: nodes, Relationships: rels}
}

func boolArg(args map[string]any, key string) bool {
	v, _ := args[key].(bool)
	return v
}

func intArg(args map[string]any, key string) int {
	v, _ := args[key].(float64)
	return int(v)
}
 func strArg(args map[string]any, key string) string {
 	v, _ := args[key].(string)
 	return v
 }
 
 func floatArg(args map[string]any, key string) float64 {
 	v, _ := args[key].(float64)
 	return v
 }
 
 // parseInterlinkedItems re-encodes the raw args["items"] array and decodes it
 // into the strongly-typed slice expected by ToolAddInterlinkedContext.
 func parseInterlinkedItems(args map[string]any) ([]memorygraph.InterlinkedItem, error) {
 	raw, _ := args["items"]
 	b, err := json.Marshal(raw)
 	if err != nil {
 		return nil, err
 	}
 	var items []memorygraph.InterlinkedItem
 	if err := json.Unmarshal(b, &items); err != nil {
 		return nil, err
 	}
 	return items, nil
 }
