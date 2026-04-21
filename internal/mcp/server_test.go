package mcp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/build"
	"github.com/supermodeltools/cli/internal/cache"
	"github.com/supermodeltools/cli/internal/config"
)

func TestFormatDeadCode_Empty(t *testing.T) {
	result := &api.DeadCodeResult{}
	got := formatDeadCode(result)
	if got != "No dead code detected." {
		t.Errorf("expected 'No dead code detected.', got: %s", got)
	}
}

func TestFormatDeadCode_WithCandidates(t *testing.T) {
	result := &api.DeadCodeResult{
		Metadata: api.DeadCodeMetadata{TotalDeclarations: 100, DeadCodeCandidates: 2},
		DeadCodeCandidates: []api.DeadCodeCandidate{
			{File: "src/a.ts", Line: 10, Name: "unused", Confidence: "high", Reason: "No callers"},
			{File: "src/b.ts", Line: 42, Name: "old", Confidence: "medium", Reason: "Transitively dead"},
		},
	}
	got := formatDeadCode(result)
	for _, want := range []string{
		"2 dead code candidate(s) out of 100 total declarations",
		"[high] src/a.ts:10 unused — No callers",
		"[medium] src/b.ts:42 old — Transitively dead",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestFormatImpact_Empty(t *testing.T) {
	result := &api.ImpactResult{}
	got := formatImpact(result)
	if got != "No impact detected." {
		t.Errorf("expected 'No impact detected.', got: %s", got)
	}
}

func TestFormatImpact_GlobalCouplingMap(t *testing.T) {
	result := &api.ImpactResult{
		GlobalMetrics: api.ImpactGlobalMetrics{
			MostCriticalFiles: []api.CriticalFileMetric{
				{File: "src/db.ts", DependentCount: 42},
			},
		},
	}
	got := formatImpact(result)
	if !strings.Contains(got, "Most critical files") {
		t.Errorf("expected global coupling header, got:\n%s", got)
	}
	if !strings.Contains(got, "src/db.ts (42 dependents)") {
		t.Errorf("expected file with count, got:\n%s", got)
	}
}

func TestFormatImpact_WithTarget(t *testing.T) {
	result := &api.ImpactResult{
		Metadata: api.ImpactMetadata{TargetsAnalyzed: 1, TotalFiles: 100, TotalFunctions: 500},
		Impacts: []api.ImpactTarget{
			{
				Target: api.ImpactTargetInfo{File: "src/auth.ts", Type: "file"},
				BlastRadius: api.BlastRadius{
					DirectDependents: 10, TransitiveDependents: 30, AffectedFiles: 5,
					RiskScore: "high", RiskFactors: []string{"High fan-in"},
				},
				AffectedFiles: []api.AffectedFile{
					{File: "src/routes.ts", DirectDependencies: 3, TransitiveDependencies: 7},
				},
				EntryPointsAffected: []api.AffectedEntryPoint{
					{File: "src/routes.ts", Name: "/login", Type: "route_handler"},
				},
			},
		},
	}
	got := formatImpact(result)
	for _, want := range []string{
		"Target: src/auth.ts",
		"Risk: high",
		"Direct: 10",
		"Transitive: 30",
		"High fan-in",
		"src/routes.ts (direct: 3, transitive: 7)",
		"/login (route_handler)",
		"1 target(s) analyzed across 100 files and 500 functions",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestFilterGraph_NoFilter(t *testing.T) {
	g := makeTestGraph()
	result := filterGraph(g, "", "")
	if len(result.Nodes) != len(g.Nodes) {
		t.Errorf("no filter: want %d nodes, got %d", len(g.Nodes), len(result.Nodes))
	}
	if len(result.Relationships) != len(g.Rels()) {
		t.Errorf("no filter: want %d rels, got %d", len(g.Rels()), len(result.Relationships))
	}
}

func TestFilterGraph_LabelOnly(t *testing.T) {
	g := makeTestGraph()
	result := filterGraph(g, "File", "")
	for _, n := range result.Nodes {
		if !n.HasLabel("File") {
			t.Errorf("label filter: expected only File nodes, got label %v", n.Labels)
		}
	}
	// Relationships must only reference nodes that are in the result.
	visible := make(map[string]bool)
	for _, n := range result.Nodes {
		visible[n.ID] = true
	}
	for _, r := range result.Relationships {
		if !visible[r.StartNode] || !visible[r.EndNode] {
			t.Errorf("label filter: relationship %s→%s references a node not in the filtered set", r.StartNode, r.EndNode)
		}
	}
}

func TestFilterGraph_LabelExcludesCrossLabelRels(t *testing.T) {
	// File nodes + Function nodes; one file→file rel, one function→function rel.
	// Filtering by File should yield only the file→file rel.
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "foo"}},
			{ID: "fn2", Labels: []string{"Function"}, Properties: map[string]any{"name": "bar"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "f1", EndNode: "f2"},
			{ID: "r2", Type: "calls", StartNode: "fn1", EndNode: "fn2"},
		},
	}
	result := filterGraph(g, "File", "")
	if len(result.Relationships) != 1 {
		t.Errorf("want 1 rel (file→file), got %d", len(result.Relationships))
	}
	if len(result.Relationships) > 0 && result.Relationships[0].ID != "r1" {
		t.Errorf("want rel r1 (imports), got %s", result.Relationships[0].ID)
	}
}

func TestFilterGraph_RelTypeOnly(t *testing.T) {
	g := makeTestGraph()
	result := filterGraph(g, "", "calls")
	for _, r := range result.Relationships {
		if r.Type != "calls" {
			t.Errorf("relType filter: expected only 'calls', got %q", r.Type)
		}
	}
	// All nodes should be returned when only relType is filtered.
	if len(result.Nodes) != len(g.Nodes) {
		t.Errorf("relType filter: want all %d nodes, got %d", len(g.Nodes), len(result.Nodes))
	}
}

func TestFilterGraph_BothFilters(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "foo"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "f1", EndNode: "f2"},
			{ID: "r2", Type: "calls", StartNode: "fn1", EndNode: "f2"},        // fn→file, excluded by label filter
			{ID: "r3", Type: "contains_call", StartNode: "f1", EndNode: "f2"}, // file→file, wrong relType
		},
	}
	result := filterGraph(g, "File", "imports")
	if len(result.Nodes) != 2 {
		t.Errorf("both filters: want 2 File nodes, got %d", len(result.Nodes))
	}
	if len(result.Relationships) != 1 || result.Relationships[0].ID != "r1" {
		t.Errorf("both filters: want only r1 (imports between Files), got %v", result.Relationships)
	}
}

// makeTestGraph builds a small mixed graph for filter tests.
func makeTestGraph() *api.Graph {
	return &api.Graph{
		Nodes: []api.Node{
			{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "a.go"}},
			{ID: "f2", Labels: []string{"File"}, Properties: map[string]any{"path": "b.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "handleReq"}},
			{ID: "fn2", Labels: []string{"Function"}, Properties: map[string]any{"name": "parse"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "f1", EndNode: "f2"},
			{ID: "r2", Type: "calls", StartNode: "fn1", EndNode: "fn2"},
			{ID: "r3", Type: "defines_function", StartNode: "f1", EndNode: "fn1"},
		},
	}
}

// ── boolArg / intArg ──────────────────────────────────────────────────────────

func TestBoolArg(t *testing.T) {
	args := map[string]any{"flag": true, "off": false, "num": 42}
	if !boolArg(args, "flag") {
		t.Error("boolArg(flag=true) should return true")
	}
	if boolArg(args, "off") {
		t.Error("boolArg(off=false) should return false")
	}
	if boolArg(args, "num") {
		t.Error("boolArg(num=42) should return false (wrong type)")
	}
	if boolArg(args, "absent") {
		t.Error("boolArg(absent) should return false")
	}
}

func TestIntArg(t *testing.T) {
	args := map[string]any{"count": float64(5), "zero": float64(0), "str": "hello"}
	if got := intArg(args, "count"); got != 5 {
		t.Errorf("intArg(count=5.0) = %d, want 5", got)
	}
	if got := intArg(args, "zero"); got != 0 {
		t.Errorf("intArg(zero=0.0) = %d, want 0", got)
	}
	if got := intArg(args, "str"); got != 0 {
		t.Errorf("intArg(str='hello') = %d, want 0 (wrong type)", got)
	}
	if got := intArg(args, "absent"); got != 0 {
		t.Errorf("intArg(absent) = %d, want 0", got)
	}
}

func TestFormatImpact_NoEntryPoints(t *testing.T) {
	result := &api.ImpactResult{
		Metadata: api.ImpactMetadata{TargetsAnalyzed: 1, TotalFiles: 50, TotalFunctions: 200},
		Impacts: []api.ImpactTarget{
			{
				Target:      api.ImpactTargetInfo{File: "src/util.ts", Type: "file"},
				BlastRadius: api.BlastRadius{DirectDependents: 2, RiskScore: "low"},
			},
		},
	}
	got := formatImpact(result)
	if strings.Contains(got, "Entry points") {
		t.Error("should not contain entry points section when none affected")
	}
}

// ── server.dispatch ───────────────────────────────────────────────────────────

func newTestServer() *server {
	return &server{cfg: &config.Config{}, dir: "/tmp/test-repo"}
}

func TestDispatch_Initialize(t *testing.T) {
	s := newTestServer()
	result, rpcErr := s.dispatch(context.Background(), "initialize", nil)
	if rpcErr != nil {
		t.Fatalf("dispatch initialize: unexpected rpcError: %v", rpcErr)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["protocolVersion"] == nil {
		t.Error("expected protocolVersion in result")
	}
}

func TestDispatch_ToolsList(t *testing.T) {
	s := newTestServer()
	result, rpcErr := s.dispatch(context.Background(), "tools/list", nil)
	if rpcErr != nil {
		t.Fatalf("dispatch tools/list: unexpected rpcError: %v", rpcErr)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["tools"] == nil {
		t.Error("expected 'tools' key in result")
	}
}

func TestDispatch_NotificationsInitialized(t *testing.T) {
	s := newTestServer()
	result, rpcErr := s.dispatch(context.Background(), "notifications/initialized", nil)
	if rpcErr != nil {
		t.Fatalf("notifications/initialized: unexpected rpcError: %v", rpcErr)
	}
	if result != nil {
		t.Errorf("notifications/initialized: expected nil result, got %v", result)
	}
}

func TestDispatch_UnknownMethod(t *testing.T) {
	s := newTestServer()
	_, rpcErr := s.dispatch(context.Background(), "unknown/method", nil)
	if rpcErr == nil {
		t.Fatal("expected rpcError for unknown method")
		return
	}
	if rpcErr.Code != codeMethodNotFound {
		t.Errorf("expected codeMethodNotFound (%d), got %d", codeMethodNotFound, rpcErr.Code)
	}
}

func TestDispatch_ToolsCall_UnknownTool(t *testing.T) {
	// Covers the "tools/call" dispatch branch and callTool error path via handleToolCall.
	s := newTestServer()
	params := json.RawMessage(`{"name":"nonexistent_tool","arguments":{}}`)
	_, rpcErr := s.dispatch(context.Background(), "tools/call", params)
	if rpcErr == nil {
		t.Fatal("expected rpcError for unknown tool name in tools/call")
		return
	}
	if rpcErr.Code != codeInternalError {
		t.Errorf("expected codeInternalError (%d), got %d", codeInternalError, rpcErr.Code)
	}
}

// ── handleInitialize ──────────────────────────────────────────────────────────

func TestHandleInitialize_Fields(t *testing.T) {
	s := newTestServer()
	result := s.handleInitialize()
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if _, ok := m["protocolVersion"]; !ok {
		t.Error("missing protocolVersion")
	}
	if _, ok := m["capabilities"]; !ok {
		t.Error("missing capabilities")
	}
	if _, ok := m["serverInfo"]; !ok {
		t.Error("missing serverInfo")
	}
}

// ── callTool ──────────────────────────────────────────────────────────────────

func TestCallTool_UnknownTool(t *testing.T) {
	s := newTestServer()
	_, err := s.callTool(context.Background(), "nonexistent_tool", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("error should mention 'unknown tool': %v", err)
	}
}

// TestCallTool_KnownToolsReachSwitch exercises each case branch in callTool.
// The tools themselves fail (no API key / repo), but the switch cases are covered.
func TestCallTool_AnalyzeCase(t *testing.T) {
	s := newTestServer()
	// analyze fails (no API key / zip) but covers the case branch.
	_, err := s.callTool(context.Background(), "analyze", map[string]any{})
	if err == nil {
		t.Error("expected error from analyze without API key")
	}
}

func TestCallTool_DeadCodeCase(t *testing.T) {
	s := newTestServer()
	_, err := s.callTool(context.Background(), "dead_code", map[string]any{})
	if err == nil {
		t.Error("expected error from dead_code without API key")
	}
}

func TestCallTool_BlastRadiusCase(t *testing.T) {
	s := newTestServer()
	_, err := s.callTool(context.Background(), "blast_radius", map[string]any{"targets": []any{"src/a.ts"}})
	if err == nil {
		t.Error("expected error from blast_radius without API key")
	}
}

func TestCallTool_GetGraphCase(t *testing.T) {
	s := newTestServer()
	_, err := s.callTool(context.Background(), "get_graph", map[string]any{})
	if err == nil {
		t.Error("expected error from get_graph without API key")
	}
}

// newServerWithGraph returns a server pre-loaded with a cached graph so that
// getOrAnalyze returns immediately without needing an API key.
func newServerWithGraph() *server {
	return &server{
		cfg: &config.Config{},
		dir: "/tmp/test-repo",
		graph: &api.Graph{
			Metadata: map[string]any{"repoId": "test-repo-123"},
			Nodes: []api.Node{
				{ID: "f1", Labels: []string{"File"}, Properties: map[string]any{"path": "main.go"}},
				{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "main"}},
			},
			Relationships: []api.Relationship{
				{ID: "r1", Type: "DEFINES", StartNode: "f1", EndNode: "fn1"},
			},
		},
		hash: "testhash123",
	}
}

func TestCallTool_AnalyzeWithCachedGraph(t *testing.T) {
	// Pre-load graph so getOrAnalyze returns immediately — covers success path of toolAnalyze.
	s := newServerWithGraph()
	result, err := s.callTool(context.Background(), "analyze", map[string]any{})
	if err != nil {
		t.Fatalf("analyze with cached graph: %v", err)
	}
	if !strings.Contains(result, "Analysis complete") {
		t.Errorf("expected 'Analysis complete', got:\n%s", result)
	}
}

func TestCallTool_GetGraphWithCachedGraph(t *testing.T) {
	// Pre-load graph so toolGetGraph returns a JSON slice — covers success path.
	s := newServerWithGraph()
	result, err := s.callTool(context.Background(), "get_graph", map[string]any{})
	if err != nil {
		t.Fatalf("get_graph with cached graph: %v", err)
	}
	if !strings.Contains(result, "nodes") {
		t.Errorf("expected JSON with 'nodes' key, got:\n%s", result)
	}
}

func TestCallTool_GetGraphWithLabelFilter(t *testing.T) {
	s := newServerWithGraph()
	result, err := s.callTool(context.Background(), "get_graph", map[string]any{"label": "File"})
	if err != nil {
		t.Fatalf("get_graph with label filter: %v", err)
	}
	if !strings.Contains(result, "main.go") {
		t.Errorf("expected File node in result, got:\n%s", result)
	}
}

func TestHandleToolCall_SuccessPath(t *testing.T) {
	// Pre-load graph so handleToolCall succeeds and covers the return-content path.
	s := newServerWithGraph()
	params := json.RawMessage(`{"name":"analyze","arguments":{}}`)
	result, rpcErr := s.handleToolCall(context.Background(), params)
	if rpcErr != nil {
		t.Fatalf("expected success, got rpcError: %v", rpcErr)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["content"] == nil {
		t.Error("expected 'content' key in result")
	}
}

func TestGetOrAnalyze_CachedGraph(t *testing.T) {
	// force=false, graph pre-set → returns immediately without API call.
	s := newServerWithGraph()
	g, hash, err := s.getOrAnalyze(context.Background(), false)
	if err != nil {
		t.Fatalf("getOrAnalyze with cached graph: %v", err)
	}
	if g == nil {
		t.Fatal("expected non-nil graph")
	}
	if hash != "testhash123" {
		t.Errorf("expected hash 'testhash123', got %q", hash)
	}
}

func TestGetOrAnalyze_ForceNoAPIKey(t *testing.T) {
	// force=true with no API key → RequireAPIKey returns error (covers L422-423).
	s := newTestServer()
	_, _, err := s.getOrAnalyze(context.Background(), true)
	if err == nil {
		t.Fatal("expected error when force=true without API key")
	}
	if !strings.Contains(err.Error(), "authenticated") {
		t.Errorf("error should mention authentication: %v", err)
	}
}

func TestEnsureZip_NoAPIKey(t *testing.T) {
	// No API key → RequireAPIKey fails (covers L438-439).
	s := newTestServer()
	_, _, err := s.ensureZip()
	if err == nil {
		t.Fatal("expected error without API key")
	}
	if !strings.Contains(err.Error(), "authenticated") {
		t.Errorf("error should mention authentication: %v", err)
	}
}

func TestEnsureZip_CreateZipError(t *testing.T) {
	// API key set but dir doesn't exist → createZip fails (covers L439-441).
	s := &server{
		cfg: &config.Config{APIKey: "smsk_live_test123"},
		dir: "/nonexistent/dir/that/does/not/exist",
	}
	_, _, err := s.ensureZip()
	if err == nil {
		t.Fatal("expected error for non-existent dir")
	}
}

func TestEnsureZip_SuccessPath(t *testing.T) {
	// API key + valid dir → createZip succeeds, HashFile succeeds (covers L443-448).
	dir := t.TempDir()
	// Write a dummy file so the zip is non-empty.
	if err := os.WriteFile(dir+"/main.go", []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &server{
		cfg: &config.Config{APIKey: "smsk_live_fake"},
		dir: dir,
	}
	zipPath, hash, err := s.ensureZip()
	if err != nil {
		t.Fatalf("ensureZip: %v", err)
	}
	defer os.Remove(zipPath)
	if hash == "" {
		t.Error("expected non-empty hash from ensureZip")
	}
}

func TestGetOrAnalyze_ForceWithAPIKeyButNoServer(t *testing.T) {
	// API key is set but no server available → analyze.GetGraph fails (covers L422-425).
	s := &server{
		cfg: &config.Config{
			APIKey:  "smsk_live_fake_key_for_test",
			APIBase: "http://127.0.0.1:1", // unreachable address
		},
		dir: t.TempDir(),
	}
	_, _, err := s.getOrAnalyze(context.Background(), true)
	if err == nil {
		t.Fatal("expected error from unreachable API server")
	}
}

// ── handleToolCall ────────────────────────────────────────────────────────────

func TestHandleToolCall_ParseError(t *testing.T) {
	s := newTestServer()
	badParams := json.RawMessage(`{not valid json`)
	_, rpcErr := s.handleToolCall(context.Background(), badParams)
	if rpcErr == nil {
		t.Fatal("expected rpcError for invalid params JSON")
		return
	}
	if rpcErr.Code != codeParseError {
		t.Errorf("expected codeParseError (%d), got %d", codeParseError, rpcErr.Code)
	}
}

// ── server.run ────────────────────────────────────────────────────────────────

func TestRun_EmptyInput(t *testing.T) {
	s := newTestServer()
	r := strings.NewReader("")
	var w strings.Builder
	err := s.run(context.Background(), r, &w)
	if err != nil {
		t.Fatalf("run with empty input: %v", err)
	}
}

func TestRun_ParseErrorLine(t *testing.T) {
	s := newTestServer()
	r := strings.NewReader("{not valid json}\n")
	var w strings.Builder
	if err := s.run(context.Background(), r, &w); err != nil {
		t.Fatalf("run: %v", err)
	}
	// Should have written a parse error response
	if !strings.Contains(w.String(), "parse error") {
		t.Errorf("expected parse error response, got: %s", w.String())
	}
}

func TestRun_BlankLines(t *testing.T) {
	s := newTestServer()
	r := strings.NewReader("\n  \n\n")
	var w strings.Builder
	if err := s.run(context.Background(), r, &w); err != nil {
		t.Fatalf("run: %v", err)
	}
	// No output for blank lines
	if w.String() != "" {
		t.Errorf("blank lines should produce no output, got: %s", w.String())
	}
}

func TestRun_InitializeRequest(t *testing.T) {
	s := newTestServer()
	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	r := strings.NewReader(req + "\n")
	var w strings.Builder
	if err := s.run(context.Background(), r, &w); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(w.String(), "protocolVersion") {
		t.Errorf("expected protocolVersion in response, got: %s", w.String())
	}
}

func TestRun_ContextCancelled(t *testing.T) {
	s := newTestServer()
	// Use a pipe so we can block on reading
	pr, pw := strings.NewReader(""), &strings.Builder{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	// Even with cancelled context, empty input should return ctx.Err or nil
	_ = s.run(ctx, pr, pw)
}

func TestRun_ContextCancelledWithPendingInput(t *testing.T) {
	// Context is cancelled before run; scanner.Scan() succeeds but ctx.Done()
	// fires in the select — covers the ctx.Done() branch in the run loop.
	s := newTestServer()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel
	req := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	r := strings.NewReader(req + "\n")
	var w strings.Builder
	err := s.run(ctx, r, &w)
	if err == nil {
		t.Error("expected non-nil error when context is pre-cancelled with pending input")
	}
}

func TestRun_UnknownMethod(t *testing.T) {
	// dispatch returns rpcError for unknown method; run should encode an error response
	s := newTestServer()
	req := `{"jsonrpc":"2.0","id":1,"method":"unknown/method"}`
	r := strings.NewReader(req + "\n")
	var w strings.Builder
	if err := s.run(context.Background(), r, &w); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(w.String(), "method not found") {
		t.Errorf("expected 'method not found' in response, got: %s", w.String())
	}
}

func TestRun_ToolsCall_ParseError(t *testing.T) {
	// tools/call with invalid params JSON should return codeParseError via handleToolCall
	s := newTestServer()
	req := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{not valid`
	r := strings.NewReader(req + "\n")
	var w strings.Builder
	if err := s.run(context.Background(), r, &w); err != nil {
		t.Fatalf("run: %v", err)
	}
	// Should have encoded a parse error
	if !strings.Contains(w.String(), "parse error") && !strings.Contains(w.String(), "error") {
		t.Errorf("expected error response for invalid params, got: %s", w.String())
	}
}

// ── toolDeadCode / toolBlastRadius cache-hit paths ────────────────────────────

// repoDir returns the root of the git repo (two levels up from internal/mcp).
func repoDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

func TestToolDeadCode_CacheHit(t *testing.T) {
	// Redirect the cache to an isolated temp dir.
	t.Setenv("HOME", t.TempDir())

	dir := repoDir(t)
	fp, err := cache.RepoFingerprint(dir)
	if err != nil {
		t.Skipf("cannot fingerprint repo: %v", err)
	}

	// toolDeadCode computes: fmt.Sprintf("dead-code:%s:%d", minConfidence, limit)
	// With no args: minConfidence="", limit=0 → "dead-code::0"
	key := cache.AnalysisKey(fp, "dead-code::0", build.Version)
	preloaded := &api.DeadCodeResult{
		Metadata: api.DeadCodeMetadata{TotalDeclarations: 20, DeadCodeCandidates: 1},
		DeadCodeCandidates: []api.DeadCodeCandidate{
			{File: "internal/api/client.go", Line: 42, Name: "cachedDeadFn", Confidence: "high", Reason: "No callers"},
		},
	}
	if err := cache.PutJSON(key, preloaded); err != nil {
		t.Fatalf("PutJSON: %v", err)
	}

	s := &server{cfg: &config.Config{}, dir: dir}
	result, err := s.toolDeadCode(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("toolDeadCode cache hit: %v", err)
	}
	if !strings.Contains(result, "cachedDeadFn") {
		t.Errorf("expected cached result containing 'cachedDeadFn', got:\n%s", result)
	}
}

func TestToolDeadCode_CacheHitWithArgs(t *testing.T) {
	// Same as above but with min_confidence and limit args.
	t.Setenv("HOME", t.TempDir())

	dir := repoDir(t)
	fp, err := cache.RepoFingerprint(dir)
	if err != nil {
		t.Skipf("cannot fingerprint repo: %v", err)
	}

	// minConfidence="high", limit=5 → "dead-code:high:5"
	key := cache.AnalysisKey(fp, "dead-code:high:5", build.Version)
	preloaded := &api.DeadCodeResult{
		Metadata: api.DeadCodeMetadata{TotalDeclarations: 10, DeadCodeCandidates: 1},
		DeadCodeCandidates: []api.DeadCodeCandidate{
			{File: "src/main.go", Line: 10, Name: "highConfFn", Confidence: "high", Reason: "Unreachable"},
		},
	}
	if err := cache.PutJSON(key, preloaded); err != nil {
		t.Fatalf("PutJSON: %v", err)
	}

	s := &server{cfg: &config.Config{}, dir: dir}
	result, err := s.toolDeadCode(context.Background(), map[string]any{
		"min_confidence": "high",
		"limit":          float64(5),
	})
	if err != nil {
		t.Fatalf("toolDeadCode cache hit with args: %v", err)
	}
	if !strings.Contains(result, "highConfFn") {
		t.Errorf("expected 'highConfFn' in result, got:\n%s", result)
	}
}

func TestToolBlastRadius_CacheHit(t *testing.T) {
	// toolBlastRadius with no file arg: analysisType = "impact"
	t.Setenv("HOME", t.TempDir())

	dir := repoDir(t)
	fp, err := cache.RepoFingerprint(dir)
	if err != nil {
		t.Skipf("cannot fingerprint repo: %v", err)
	}

	key := cache.AnalysisKey(fp, "impact", build.Version)
	preloaded := &api.ImpactResult{
		Metadata: api.ImpactMetadata{TargetsAnalyzed: 0, TotalFiles: 50, TotalFunctions: 200},
		GlobalMetrics: api.ImpactGlobalMetrics{
			MostCriticalFiles: []api.CriticalFileMetric{
				{File: "core/db.go", DependentCount: 15},
			},
		},
	}
	if err := cache.PutJSON(key, preloaded); err != nil {
		t.Fatalf("PutJSON: %v", err)
	}

	s := &server{cfg: &config.Config{}, dir: dir}
	result, err := s.toolBlastRadius(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("toolBlastRadius cache hit: %v", err)
	}
	if !strings.Contains(result, "core/db.go") {
		t.Errorf("expected 'core/db.go' in result, got:\n%s", result)
	}
}

func TestToolBlastRadius_CacheHitWithTarget(t *testing.T) {
	// toolBlastRadius with file arg: analysisType = "impact:<file>"
	t.Setenv("HOME", t.TempDir())

	dir := repoDir(t)
	fp, err := cache.RepoFingerprint(dir)
	if err != nil {
		t.Skipf("cannot fingerprint repo: %v", err)
	}

	target := "internal/api/client.go"
	key := cache.AnalysisKey(fp, "impact:"+target, build.Version)
	preloaded := &api.ImpactResult{
		Metadata: api.ImpactMetadata{TargetsAnalyzed: 1, TotalFiles: 80, TotalFunctions: 400},
		Impacts: []api.ImpactTarget{
			{
				Target: api.ImpactTargetInfo{File: target, Type: "file"},
				BlastRadius: api.BlastRadius{
					DirectDependents: 3, TransitiveDependents: 10, AffectedFiles: 5,
					RiskScore: "medium",
				},
			},
		},
	}
	if err := cache.PutJSON(key, preloaded); err != nil {
		t.Fatalf("PutJSON: %v", err)
	}

	s := &server{cfg: &config.Config{}, dir: dir}
	result, err := s.toolBlastRadius(context.Background(), map[string]any{"file": target})
	if err != nil {
		t.Fatalf("toolBlastRadius cache hit with target: %v", err)
	}
	if !strings.Contains(result, target) {
		t.Errorf("expected target %q in result, got:\n%s", target, result)
	}
}
