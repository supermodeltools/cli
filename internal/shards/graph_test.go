package shards

import (
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func fileNode(id, path string) api.Node {
	return api.Node{ID: id, Labels: []string{"File"}, Properties: map[string]any{"filePath": path}}
}

func fnNode(id, name, filePath string) api.Node {
	return api.Node{ID: id, Labels: []string{"Function"}, Properties: map[string]any{"name": name, "filePath": filePath}}
}

func fnNodeWithLine(id, name, filePath string, line int) api.Node {
	return api.Node{ID: id, Labels: []string{"Function"}, Properties: map[string]any{"name": name, "filePath": filePath, "startLine": float64(line)}}
}

func rel(id, typ, start, end string) api.Relationship {
	return api.Relationship{ID: id, Type: typ, StartNode: start, EndNode: end}
}

func buildCache(nodes []api.Node, rels []api.Relationship) *Cache {
	ir := &api.ShardIR{Graph: api.ShardGraph{Nodes: nodes, Relationships: rels}}
	c := NewCache()
	c.Build(ir)
	return c
}

// ── isShardPath ───────────────────────────────────────────────────────────────

func TestIsShardPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"src/handler.graph.go", true},
		{"src/handler.graph.ts", true},
		{"lib/foo.graph.js", true},
		{"src/handler.go", false},
		{"src/handler.ts", false},
		{"graph.go", false},         // no double extension
		{"src/a.b.graph.go", true},  // any double extension with .graph
		{"src/file.graph", false},   // .graph alone is not a source ext
	}
	for _, tc := range cases {
		if got := isShardPath(tc.path); got != tc.want {
			t.Errorf("isShardPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// ── firstString ───────────────────────────────────────────────────────────────

func TestFirstString(t *testing.T) {
	props := map[string]any{"filePath": "src/a.go", "name": "myFile", "empty": ""}

	if got := firstString(props, "filePath", "fallback"); got != "src/a.go" {
		t.Errorf("got %q, want src/a.go", got)
	}
	if got := firstString(props, "missing", "name", "fallback"); got != "myFile" {
		t.Errorf("got %q, want myFile", got)
	}
	// empty string skipped
	if got := firstString(props, "empty", "name", "fallback"); got != "myFile" {
		t.Errorf("empty string should be skipped: got %q", got)
	}
	// literal fallback when no key matches
	if got := firstString(props, "missing", "fallback"); got != "fallback" {
		t.Errorf("got %q, want literal fallback", got)
	}
}

// ── intProp ───────────────────────────────────────────────────────────────────

func TestIntProp(t *testing.T) {
	n := api.Node{Properties: map[string]any{
		"line":    float64(42),
		"count":   int(7),
		"text":    "hello",
		"missing": nil,
	}}
	if got := intProp(n, "line"); got != 42 {
		t.Errorf("float64 prop: got %d, want 42", got)
	}
	if got := intProp(n, "count"); got != 7 {
		t.Errorf("int prop: got %d, want 7", got)
	}
	if got := intProp(n, "text"); got != 0 {
		t.Errorf("string prop should return 0: got %d", got)
	}
	if got := intProp(n, "absent"); got != 0 {
		t.Errorf("missing prop should return 0: got %d", got)
	}
}

// ── fnFile / fnLine ───────────────────────────────────────────────────────────

func TestFnFileAndLine_Nil(t *testing.T) {
	if got := fnFile(nil); got != "" {
		t.Errorf("fnFile(nil): got %q, want empty", got)
	}
	if got := fnLine(nil); got != 0 {
		t.Errorf("fnLine(nil): got %d, want 0", got)
	}
}

func TestFnFileAndLine_NonNil(t *testing.T) {
	fi := &FuncInfo{File: "src/a.go", Line: 10}
	if got := fnFile(fi); got != "src/a.go" {
		t.Errorf("got %q", got)
	}
	if got := fnLine(fi); got != 10 {
		t.Errorf("got %d", got)
	}
}

// ── Cache.Build ───────────────────────────────────────────────────────────────

func TestBuild_IndexesFunctions(t *testing.T) {
	c := buildCache(
		[]api.Node{fnNodeWithLine("fn1", "handleReq", "src/handler.go", 15)},
		nil,
	)
	fn, ok := c.FnByID["fn1"]
	if !ok {
		t.Fatal("fn1 not indexed")
	}
	if fn.Name != "handleReq" {
		t.Errorf("name: got %q", fn.Name)
	}
	if fn.File != "src/handler.go" {
		t.Errorf("file: got %q", fn.File)
	}
	if fn.Line != 15 {
		t.Errorf("line: got %d", fn.Line)
	}
}

func TestBuild_FuncNameFromID(t *testing.T) {
	// When "name" prop is absent, name extracted from ID like "fn:src/foo.ts:bar"
	n := api.Node{ID: "fn:src/foo.ts:bar", Labels: []string{"Function"}, Properties: map[string]any{"filePath": "src/foo.ts"}}
	c := buildCache([]api.Node{n}, nil)
	fn, ok := c.FnByID["fn:src/foo.ts:bar"]
	if !ok {
		t.Fatal("function not indexed")
	}
	if fn.Name != "bar" {
		t.Errorf("expected name 'bar', got %q", fn.Name)
	}
}

func TestBuild_IndexesCallEdges(t *testing.T) {
	c := buildCache(
		[]api.Node{
			fnNode("caller", "main", "src/main.go"),
			fnNode("callee", "handle", "src/handler.go"),
		},
		[]api.Relationship{rel("r1", "calls", "caller", "callee")},
	)
	callers := c.Callers["callee"]
	if len(callers) != 1 || callers[0].FuncID != "caller" {
		t.Errorf("callers of callee: got %+v", callers)
	}
	callees := c.Callees["caller"]
	if len(callees) != 1 || callees[0].FuncID != "callee" {
		t.Errorf("callees of caller: got %+v", callees)
	}
}

func TestBuild_IndexesImportEdges(t *testing.T) {
	c := buildCache(
		[]api.Node{
			fileNode("f1", "src/a.go"),
			fileNode("f2", "src/b.go"),
		},
		[]api.Relationship{rel("r1", "imports", "f1", "f2")},
	)
	if len(c.Imports["src/a.go"]) != 1 || c.Imports["src/a.go"][0] != "src/b.go" {
		t.Errorf("imports: got %v", c.Imports["src/a.go"])
	}
	if len(c.Importers["src/b.go"]) != 1 || c.Importers["src/b.go"][0] != "src/a.go" {
		t.Errorf("importers: got %v", c.Importers["src/b.go"])
	}
}

func TestBuild_SkipsExternalImports(t *testing.T) {
	c := buildCache(
		[]api.Node{
			fileNode("f1", "src/a.go"),
			{ID: "ext1", Labels: []string{"ExternalDependency"}, Properties: map[string]any{"name": "fmt"}},
		},
		[]api.Relationship{rel("r1", "imports", "f1", "ext1")},
	)
	if len(c.Imports["src/a.go"]) != 0 {
		t.Errorf("external imports should be skipped, got %v", c.Imports["src/a.go"])
	}
}

func TestBuild_DefinesFunctionSetsFile(t *testing.T) {
	// Function node has no filePath but is linked via defines_function
	fn := api.Node{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "doStuff"}}
	c := buildCache(
		[]api.Node{fileNode("file1", "src/util.go"), fn},
		[]api.Relationship{rel("r1", "defines_function", "file1", "fn1")},
	)
	if c.FnByID["fn1"].File != "src/util.go" {
		t.Errorf("defines_function should set fn.File; got %q", c.FnByID["fn1"].File)
	}
}

func TestBuild_LocalDependencyNode(t *testing.T) {
	// LocalDependency node → IDToPath uses filePath/name/ID
	n := api.Node{ID: "ld1", Labels: []string{"LocalDependency"}, Properties: map[string]any{"name": "@/components/button"}}
	c := buildCache([]api.Node{n}, nil)
	if c.IDToPath["ld1"] != "@/components/button" {
		t.Errorf("LocalDependency IDToPath: got %q", c.IDToPath["ld1"])
	}
}

func TestBuild_ExternalDependencyWithName(t *testing.T) {
	n := api.Node{ID: "ext1", Labels: []string{"ExternalDependency"}, Properties: map[string]any{"name": "react"}}
	c := buildCache([]api.Node{n}, nil)
	if c.IDToPath["ext1"] != "[ext]react" {
		t.Errorf("ExternalDependency with name: got %q", c.IDToPath["ext1"])
	}
}

func TestBuild_ExternalDependencyNoName(t *testing.T) {
	// ExternalDependency with empty name → falls back to node ID
	n := api.Node{ID: "ext-node-id", Labels: []string{"ExternalDependency"}, Properties: map[string]any{}}
	c := buildCache([]api.Node{n}, nil)
	if c.IDToPath["ext-node-id"] != "[ext]ext-node-id" {
		t.Errorf("ExternalDependency without name: got %q", c.IDToPath["ext-node-id"])
	}
}

func TestBuild_BelongsToWithFilePath(t *testing.T) {
	// belongsTo: node with filePath → FileDomain set via domain node name
	domainNode := api.Node{ID: "dom1", Labels: []string{"Domain"}, Properties: map[string]any{"name": "Auth"}}
	fileN := fileNode("f1", "src/auth/login.go")
	c := buildCache(
		[]api.Node{fileN, domainNode},
		[]api.Relationship{rel("r1", "belongsTo", "f1", "dom1")},
	)
	if c.FileDomain["src/auth/login.go"] != "Auth" {
		t.Errorf("belongsTo FileDomain: got %q", c.FileDomain["src/auth/login.go"])
	}
}

func TestBuild_BelongsToFallbackToFnFile(t *testing.T) {
	// belongsTo: no filePath on node → falls back to fn.File
	domainNode := api.Node{ID: "dom1", Labels: []string{"Domain"}, Properties: map[string]any{"name": "Core"}}
	fn := api.Node{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "doWork", "filePath": "src/core.go"}}
	c := buildCache(
		[]api.Node{fn, domainNode},
		[]api.Relationship{rel("r1", "belongsTo", "fn1", "dom1")},
	)
	if c.FileDomain["src/core.go"] != "Core" {
		t.Errorf("belongsTo via fn.File: got %q", c.FileDomain["src/core.go"])
	}
}

func TestBuild_BelongsToNoDomainName_ExtractFromID(t *testing.T) {
	// belongsTo: domain node has no name → extracts from ID using colon split
	domainNode := api.Node{ID: "domain:MyDomain", Labels: []string{"Domain"}, Properties: map[string]any{}}
	fileN := fileNode("f1", "src/x.go")
	c := buildCache(
		[]api.Node{fileN, domainNode},
		[]api.Relationship{rel("r1", "belongsTo", "f1", "domain:MyDomain")},
	)
	if c.FileDomain["src/x.go"] != "MyDomain" {
		t.Errorf("belongsTo ID extraction: got %q", c.FileDomain["src/x.go"])
	}
}

func TestBuild_DomainSubdomainFiles(t *testing.T) {
	// Subdomain with Files (not KeyFiles) → assigns domain/sub for each file
	ir := &api.ShardIR{
		Graph: api.ShardGraph{},
		Domains: []api.ShardDomain{
			{
				Name: "Web",
				Subdomains: []api.ShardSubdomain{
					{Name: "Routes", Files: []string{"src/routes/index.go", "src/routes/auth.go"}},
				},
			},
		},
	}
	c := NewCache()
	c.Build(ir)
	for _, f := range []string{"src/routes/index.go", "src/routes/auth.go"} {
		if c.FileDomain[f] != "Web/Routes" {
			t.Errorf("subdomain Files: FileDomain[%q] = %q, want 'Web/Routes'", f, c.FileDomain[f])
		}
	}
}

func TestBuild_BelongsToNoPathNoFn_Skipped(t *testing.T) {
	// Node has no filePath and is not in FnByID → nodePath stays "" → continue
	domainNode := api.Node{ID: "dom1", Labels: []string{"Domain"}, Properties: map[string]any{"name": "Auth"}}
	unknownNode := api.Node{ID: "unknown1", Labels: []string{"Unknown"}, Properties: map[string]any{}}
	c := buildCache(
		[]api.Node{unknownNode, domainNode},
		[]api.Relationship{rel("r1", "belongsTo", "unknown1", "dom1")},
	)
	// FileDomain should remain empty since nothing was added
	if len(c.FileDomain) != 0 {
		t.Errorf("belongsTo with no path should be skipped; got FileDomain: %v", c.FileDomain)
	}
}

func TestBuild_DomainSubdomainKeyFiles(t *testing.T) {
	// Subdomain with KeyFiles (no Files) → assigns domain/sub for each key file
	ir := &api.ShardIR{
		Graph: api.ShardGraph{},
		Domains: []api.ShardDomain{
			{
				Name: "Auth",
				Subdomains: []api.ShardSubdomain{
					{Name: "Login", KeyFiles: []string{"src/auth/login.go"}},
				},
			},
		},
	}
	c := NewCache()
	c.Build(ir)
	if c.FileDomain["src/auth/login.go"] != "Auth/Login" {
		t.Errorf("subdomain KeyFiles: FileDomain = %q, want 'Auth/Login'", c.FileDomain["src/auth/login.go"])
	}
}

func TestBuild_DomainAssignmentFromKeyFiles(t *testing.T) {
	ir := &api.ShardIR{
		Graph:   api.ShardGraph{Nodes: []api.Node{fileNode("f1", "src/auth/login.go")}},
		Domains: []api.ShardDomain{{Name: "auth", KeyFiles: []string{"src/auth/login.go"}}},
	}
	c := NewCache()
	c.Build(ir)
	if c.FileDomain["src/auth/login.go"] != "auth" {
		t.Errorf("domain assignment: got %q", c.FileDomain["src/auth/login.go"])
	}
}

// ── SourceFiles ───────────────────────────────────────────────────────────────

func TestSourceFiles_ReturnsSourceExts(t *testing.T) {
	c := buildCache(
		[]api.Node{
			fileNode("f1", "src/a.go"),
			fileNode("f2", "src/b.ts"),
			fileNode("f3", "src/README.md"), // not a source ext
		},
		nil,
	)
	files := c.SourceFiles()
	want := map[string]bool{"src/a.go": true, "src/b.ts": true}
	if len(files) != 2 {
		t.Errorf("want 2 source files, got %d: %v", len(files), files)
	}
	for _, f := range files {
		if !want[f] {
			t.Errorf("unexpected file %q", f)
		}
	}
}

func TestSourceFiles_ExcludesShards(t *testing.T) {
	c := buildCache(
		[]api.Node{
			fileNode("f1", "src/handler.go"),
			fileNode("f2", "src/handler.graph.go"),
		},
		nil,
	)
	files := c.SourceFiles()
	for _, f := range files {
		if isShardPath(f) {
			t.Errorf("shard path should be excluded: %q", f)
		}
	}
	if len(files) != 1 || files[0] != "src/handler.go" {
		t.Errorf("got %v", files)
	}
}

func TestSourceFiles_IncludesFromImports(t *testing.T) {
	c := buildCache(
		[]api.Node{
			fileNode("f1", "src/a.go"),
			fileNode("f2", "src/b.go"),
		},
		[]api.Relationship{rel("r1", "imports", "f1", "f2")},
	)
	files := c.SourceFiles()
	seen := map[string]bool{}
	for _, f := range files {
		seen[f] = true
	}
	if !seen["src/a.go"] || !seen["src/b.go"] {
		t.Errorf("expected both files, got %v", files)
	}
}

// ── FuncName ──────────────────────────────────────────────────────────────────

func TestFuncName_Known(t *testing.T) {
	c := buildCache([]api.Node{fnNode("fn1", "processRequest", "src/a.go")}, nil)
	if got := c.FuncName("fn1"); got != "processRequest" {
		t.Errorf("got %q", got)
	}
}

func TestFuncName_Unknown_ExtractsFromID(t *testing.T) {
	c := NewCache()
	if got := c.FuncName("pkg:file:methodName"); got != "methodName" {
		t.Errorf("got %q, want methodName", got)
	}
}

// ── TransitiveDependents ──────────────────────────────────────────────────────

func TestTransitiveDependents_Direct(t *testing.T) {
	// a imports b: b has one direct dependent
	c := buildCache(
		[]api.Node{fileNode("fa", "a.go"), fileNode("fb", "b.go")},
		[]api.Relationship{rel("r1", "imports", "fa", "fb")},
	)
	deps := c.TransitiveDependents("b.go")
	if len(deps) != 1 || !deps["a.go"] {
		t.Errorf("expected {a.go}, got %v", deps)
	}
}

func TestTransitiveDependents_Transitive(t *testing.T) {
	// a→b→c: c has two dependents (a, b)
	c := buildCache(
		[]api.Node{fileNode("fa", "a.go"), fileNode("fb", "b.go"), fileNode("fc", "c.go")},
		[]api.Relationship{
			rel("r1", "imports", "fa", "fb"),
			rel("r2", "imports", "fb", "fc"),
		},
	)
	deps := c.TransitiveDependents("c.go")
	if !deps["a.go"] || !deps["b.go"] {
		t.Errorf("expected a.go and b.go, got %v", deps)
	}
	if deps["c.go"] {
		t.Error("c.go should not be in its own dependents")
	}
}

func TestTransitiveDependents_Cycle(t *testing.T) {
	// a→b→a cycle must not infinite-loop
	c := buildCache(
		[]api.Node{fileNode("fa", "a.go"), fileNode("fb", "b.go")},
		[]api.Relationship{
			rel("r1", "imports", "fa", "fb"),
			rel("r2", "imports", "fb", "fa"),
		},
	)
	done := make(chan struct{})
	go func() {
		c.TransitiveDependents("a.go")
		close(done)
	}()
	select {
	case <-done:
	default:
		// immediate completion is fine
		<-done
	}
}

func TestTransitiveDependents_None(t *testing.T) {
	c := buildCache([]api.Node{fileNode("fa", "a.go")}, nil)
	deps := c.TransitiveDependents("a.go")
	if len(deps) != 0 {
		t.Errorf("expected empty, got %v", deps)
	}
}

// ── computeStats ─────────────────────────────────────────────────────────────

func TestComputeStats_Basic(t *testing.T) {
	ir := &api.ShardIR{Graph: api.ShardGraph{
		Nodes: []api.Node{
			fileNode("f1", "src/a.go"),
			fnNode("fn1", "foo", "src/a.go"),
			fnNode("fn2", "bar", "src/a.go"),
		},
		Relationships: []api.Relationship{
			rel("r1", "calls", "fn1", "fn2"),
		},
	}}
	c := NewCache()
	c.Build(ir)
	stats := computeStats(ir, c)

	if stats.SourceFiles != 1 {
		t.Errorf("SourceFiles: got %d, want 1", stats.SourceFiles)
	}
	if stats.Functions != 2 {
		t.Errorf("Functions: got %d, want 2", stats.Functions)
	}
	if stats.Relationships != 1 {
		t.Errorf("Relationships: got %d, want 1", stats.Relationships)
	}
	// fn1 has no callers (it calls fn2); fn2 has fn1 as caller
	if stats.DeadFunctionCount != 1 {
		t.Errorf("DeadFunctionCount: got %d, want 1 (fn1 has no callers)", stats.DeadFunctionCount)
	}
}

func TestComputeStats_FromCache(t *testing.T) {
	ir := &api.ShardIR{Graph: api.ShardGraph{}}
	c := NewCache()
	c.Build(ir)
	stats := computeStats(ir, c)
	stats.FromCache = true
	if !stats.FromCache {
		t.Error("FromCache should be settable")
	}
}

// TestBuild_BelongsToFnWithFileKey covers the L182 branch where a function node
// uses the "file" key (not "filePath") so IDToPath is empty but FnByID has it.
func TestBuild_BelongsToFnWithFileKey(t *testing.T) {
	// Use "file" property instead of "filePath" so IDToPath is not set for fn1,
	// but FnByID["fn1"].File is populated → the fallback `if fn, ok := c.FnByID[...]`
	// branch (L182) is reached.
	domainNode := api.Node{
		ID:     "dom1",
		Labels: []string{"Domain"},
		Properties: map[string]any{"name": "Core"},
	}
	fnWithFileKey := api.Node{
		ID:     "fn1",
		Labels: []string{"Function"},
		Properties: map[string]any{"name": "doWork", "file": "src/core.go"},
	}
	c := buildCache(
		[]api.Node{fnWithFileKey, domainNode},
		[]api.Relationship{rel("r1", "belongsTo", "fn1", "dom1")},
	)
	if c.FileDomain["src/core.go"] != "Core" {
		t.Errorf("belongsTo via fn.File (file key): FileDomain[src/core.go] = %q, want Core", c.FileDomain["src/core.go"])
	}
}

// TestSourceFiles_IncludesFromFunctions covers L225-226 in SourceFiles: functions
// in FnByID with non-empty File contribute their file to the result.
func TestSourceFiles_IncludesFromFunctions(t *testing.T) {
	c := buildCache(
		[]api.Node{
			// Function node with filePath — populates FnByID with File="src/a.go"
			fnNode("fn1", "doWork", "src/a.go"),
		},
		nil,
	)
	files := c.SourceFiles()
	found := false
	for _, f := range files {
		if f == "src/a.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("SourceFiles should include file from FnByID; got %v", files)
	}
}
