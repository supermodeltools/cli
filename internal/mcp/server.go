package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/cache"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
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
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
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
	Type       string              `json:"type"`
	Properties map[string]schemaProp `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
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
		Description: "List functions in the repository that have no callers. Returns function names and their source files.",
		InputSchema: toolSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"include_exports": {Type: "boolean", Description: "Include exported (public) functions, which may be called by external packages."},
				"force":           {Type: "boolean", Description: "Re-analyze even if a cached result exists."},
			},
		},
	},
	{
		Name:        "blast_radius",
		Description: "Given a file path, return all files that transitively import it — i.e., the set of files that would be affected by a change to that file.",
		InputSchema: toolSchema{
			Type:     "object",
			Required: []string{"file"},
			Properties: map[string]schemaProp{
				"file":  {Type: "string", Description: "Repo-relative path to the file (e.g. internal/api/client.go)."},
				"depth": {Type: "integer", Description: "Maximum traversal depth. 0 = unlimited."},
				"force": {Type: "boolean", Description: "Re-analyze even if a cached result exists."},
			},
		},
	},
	{
		Name:        "get_graph",
		Description: "Return a filtered slice of the dependency or call graph as JSON. Useful for understanding how a specific part of the codebase is connected.",
		InputSchema: toolSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"label": {Type: "string", Description: "Filter nodes by label: File, Function, Class, etc."},
				"rel_type": {Type: "string", Description: "Filter relationships by type: IMPORTS, CALLS, DEFINES_FUNCTION, etc."},
				"force": {Type: "boolean", Description: "Re-analyze even if a cached result exists."},
			},
		},
	},
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
	force := boolArg(args, "force")

	switch name {
	case "analyze":
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

	case "dead_code":
		g, _, err := s.getOrAnalyze(ctx, force)
		if err != nil {
			return "", err
		}
		includeExports := boolArg(args, "include_exports")
		results := findDeadFunctions(g, includeExports)
		if len(results) == 0 {
			return "No dead code detected.", nil
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "%d unreachable function(s):\n\n", len(results))
		for _, r := range results {
			fmt.Fprintf(&sb, "- %s  (%s)\n", r.name, r.file)
		}
		return sb.String(), nil

	case "blast_radius":
		fileArg, _ := args["file"].(string)
		if fileArg == "" {
			return "", fmt.Errorf("required argument 'file' is missing")
		}
		g, _, err := s.getOrAnalyze(ctx, force)
		if err != nil {
			return "", err
		}
		affected := findAffected(g, fileArg)
		if len(affected) == 0 {
			return fmt.Sprintf("No files are affected by changes to %s.", fileArg), nil
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "%d file(s) affected by changes to %s:\n\n", len(affected), fileArg)
		for _, f := range affected {
			fmt.Fprintf(&sb, "- %s  (depth %d)\n", f.file, f.depth)
		}
		return sb.String(), nil

	case "get_graph":
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

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// getOrAnalyze returns the cached graph or runs a fresh analysis.
func (s *server) getOrAnalyze(ctx context.Context, force bool) (*api.Graph, string, error) {
	if !force && s.graph != nil {
		return s.graph, s.hash, nil
	}

	if err := s.cfg.RequireAPIKey(); err != nil {
		return nil, "", err
	}

	zipPath, err := createZip(s.dir)
	if err != nil {
		return nil, "", err
	}
	defer os.Remove(zipPath)

	hash, err := cache.HashFile(zipPath)
	if err != nil {
		return nil, "", err
	}

	if !force {
		if g, _ := cache.Get(hash); g != nil {
			s.graph = g
			s.hash = hash
			return g, hash, nil
		}
	}

	ui.Step("Analyzing repository…")
	client := api.New(s.cfg)
	g, err := client.Analyze(ctx, zipPath, "mcp-"+hash[:16])
	if err != nil {
		return nil, hash, err
	}
	_ = cache.Put(hash, g)
	s.graph = g
	s.hash = hash
	return g, hash, nil
}

// --- Inline helpers (duplicated from slices to preserve VSA) -----------------

type deadFn struct{ name, file string }

func findDeadFunctions(g *api.Graph, includeExports bool) []deadFn {
	called := make(map[string]bool)
	for _, rel := range g.Rels() {
		if rel.Type == "CALLS" || rel.Type == "CONTAINS_CALL" {
			called[rel.EndNode] = true
		}
	}
	var out []deadFn
	for _, n := range g.NodesByLabel("Function") {
		if called[n.ID] {
			continue
		}
		name := n.Prop("name", "qualifiedName")
		file := n.Prop("file", "path")
		if isEntryPoint(name, file, includeExports) {
			continue
		}
		out = append(out, deadFn{name, file})
	}
	return out
}

type affected struct{ file string; depth int }

func findAffected(g *api.Graph, target string) []affected {
	importedBy := make(map[string][]string)
	for _, rel := range g.Rels() {
		if rel.Type == "IMPORTS" || rel.Type == "WILDCARD_IMPORTS" {
			importedBy[rel.EndNode] = append(importedBy[rel.EndNode], rel.StartNode)
		}
	}
	var seeds []string
	for _, n := range g.NodesByLabel("File") {
		if pathMatches(n.Prop("path", "name", "file"), target) {
			seeds = append(seeds, n.ID)
		}
	}
	visited := make(map[string]int)
	queue := append([]string(nil), seeds...)
	for _, s := range seeds {
		visited[s] = 0
	}
	var results []affected
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, parent := range importedBy[cur] {
			if _, seen := visited[parent]; seen {
				continue
			}
			d := visited[cur] + 1
			visited[parent] = d
			queue = append(queue, parent)
			n, ok := g.NodeByID(parent)
			if !ok {
				continue
			}
			f := n.Prop("path", "name", "file")
			if f != "" && !pathMatches(f, target) {
				results = append(results, affected{f, d})
			}
		}
	}
	return results
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

func isEntryPoint(name, file string, includeExports bool) bool {
	bare := name
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		bare = name[idx+1:]
	}
	if bare == "main" || bare == "init" {
		return true
	}
	for _, prefix := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
		if strings.HasPrefix(bare, prefix) {
			return true
		}
	}
	if !includeExports && len(bare) > 0 && bare[0] >= 'A' && bare[0] <= 'Z' {
		return true
	}
	return strings.HasSuffix(file, "_test.go")
}

func pathMatches(nodePath, target string) bool {
	target = strings.TrimPrefix(target, "./")
	nodePath = strings.TrimPrefix(nodePath, "./")
	return nodePath == target || strings.HasSuffix(nodePath, "/"+target)
}
