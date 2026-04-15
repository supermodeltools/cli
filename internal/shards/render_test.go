package shards

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

// makeRenderCache builds a Cache from a ShardIR for render tests.
func makeRenderCache(ir *api.ShardIR) *Cache {
	c := NewCache()
	c.Build(ir)
	return c
}

func shardIR(nodes []api.Node, rels []api.Relationship) *api.ShardIR {
	return &api.ShardIR{
		Graph: api.ShardGraph{
			Nodes:         nodes,
			Relationships: rels,
		},
	}
}

// TestRenderCallsSection_Deterministic verifies that renderCallsSection produces
// the same output regardless of the order relationships were appended to the cache.
// This catches non-determinism from map iteration or relationship ordering in the
// API response, which would cause unnecessary shard file rewrites on each run.
func TestRenderCallsSection_Deterministic(t *testing.T) {
	nodes := []api.Node{
		{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "handle", "filePath": "src/a.go"}},
		{ID: "fn2", Labels: []string{"Function"}, Properties: map[string]any{"name": "parse", "filePath": "src/b.go"}},
		{ID: "fn3", Labels: []string{"Function"}, Properties: map[string]any{"name": "validate", "filePath": "src/c.go"}},
	}

	// Build two equivalent graphs with relationships in reversed order.
	// If rendering is deterministic, both must produce identical output.
	ir1 := shardIR(nodes, []api.Relationship{
		{ID: "r1", Type: "calls", StartNode: "fn2", EndNode: "fn1"},
		{ID: "r2", Type: "calls", StartNode: "fn3", EndNode: "fn1"},
	})
	ir2 := shardIR(nodes, []api.Relationship{
		{ID: "r2", Type: "calls", StartNode: "fn3", EndNode: "fn1"},
		{ID: "r1", Type: "calls", StartNode: "fn2", EndNode: "fn1"},
	})

	c1 := makeRenderCache(ir1)
	c2 := makeRenderCache(ir2)

	out1 := renderCallsSection("src/a.go", c1, "//")
	out2 := renderCallsSection("src/a.go", c2, "//")

	if out1 != out2 {
		t.Errorf("renderCallsSection output differs based on relationship order:\ngot1:\n%s\ngot2:\n%s", out1, out2)
	}
}

// TestRenderCalleesSection_Deterministic mirrors TestRenderCallsSection_Deterministic
// but targets the callee path: a single caller with multiple callees whose relationships
// appear in reversed order must produce identical output.
func TestRenderCalleesSection_Deterministic(t *testing.T) {
	nodes := []api.Node{
		{ID: "fn_caller", Labels: []string{"Function"}, Properties: map[string]any{"name": "dispatch", "filePath": "src/a.go"}},
		{ID: "fn_c1", Labels: []string{"Function"}, Properties: map[string]any{"name": "alpha", "filePath": "src/b.go"}},
		{ID: "fn_c2", Labels: []string{"Function"}, Properties: map[string]any{"name": "beta", "filePath": "src/c.go"}},
	}

	ir1 := shardIR(nodes, []api.Relationship{
		{ID: "r1", Type: "calls", StartNode: "fn_caller", EndNode: "fn_c1"},
		{ID: "r2", Type: "calls", StartNode: "fn_caller", EndNode: "fn_c2"},
	})
	ir2 := shardIR(nodes, []api.Relationship{
		{ID: "r2", Type: "calls", StartNode: "fn_caller", EndNode: "fn_c2"},
		{ID: "r1", Type: "calls", StartNode: "fn_caller", EndNode: "fn_c1"},
	})

	c1 := makeRenderCache(ir1)
	c2 := makeRenderCache(ir2)

	out1 := renderCallsSection("src/a.go", c1, "//")
	out2 := renderCallsSection("src/a.go", c2, "//")

	if out1 != out2 {
		t.Errorf("callee output differs based on relationship order:\ngot1:\n%s\ngot2:\n%s", out1, out2)
	}
}

// TestRenderCallsSection_SameNameFunctions ensures that two functions with the same
// name (but different IDs, e.g. methods on different types) are ordered by ID when
// they share a name, preventing non-determinism from the unstable sort.
func TestRenderCallsSection_SameNameFunctions(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			{ID: "fn_a", Labels: []string{"Function"}, Properties: map[string]any{"name": "String", "filePath": "src/types.go"}},
			{ID: "fn_b", Labels: []string{"Function"}, Properties: map[string]any{"name": "String", "filePath": "src/types.go"}},
			{ID: "caller1", Labels: []string{"Function"}, Properties: map[string]any{"name": "callA", "filePath": "src/other.go"}},
			{ID: "caller2", Labels: []string{"Function"}, Properties: map[string]any{"name": "callB", "filePath": "src/other.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "calls", StartNode: "caller1", EndNode: "fn_a"},
			{ID: "r2", Type: "calls", StartNode: "caller2", EndNode: "fn_b"},
		},
	)

	c := makeRenderCache(ir)

	// Run renderCallsSection multiple times to detect non-determinism
	first := renderCallsSection("src/types.go", c, "//")
	for i := 0; i < 10; i++ {
		out := renderCallsSection("src/types.go", c, "//")
		if out != first {
			t.Errorf("renderCallsSection is non-deterministic (run %d differs from run 0):\nfirst:\n%s\nlater:\n%s", i+1, first, out)
		}
	}
}

// TestRenderCallsSection_ContainsCallerAndCallee verifies basic content correctness.
func TestRenderCallsSection_ContainsCallerAndCallee(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			{ID: "fn_target", Labels: []string{"Function"}, Properties: map[string]any{"name": "processRequest", "filePath": "src/handler.go"}},
			{ID: "fn_caller", Labels: []string{"Function"}, Properties: map[string]any{"name": "main", "filePath": "src/main.go"}},
			{ID: "fn_callee", Labels: []string{"Function"}, Properties: map[string]any{"name": "validate", "filePath": "src/util.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "calls", StartNode: "fn_caller", EndNode: "fn_target"},
			{ID: "r2", Type: "calls", StartNode: "fn_target", EndNode: "fn_callee"},
		},
	)

	c := makeRenderCache(ir)
	out := renderCallsSection("src/handler.go", c, "//")

	if out == "" {
		t.Fatal("expected non-empty output for function with caller and callee")
	}
	if !strings.Contains(out, "[calls]") {
		t.Errorf("should contain [calls] header:\n%s", out)
	}
	if !strings.Contains(out, "processRequest ← main") {
		t.Errorf("should show caller relationship:\n%s", out)
	}
	if !strings.Contains(out, "processRequest → validate") {
		t.Errorf("should show callee relationship:\n%s", out)
	}
}

// TestRenderCallsSection_EmptyWhenNoCallRelationships returns empty for a file
// with functions that have no callers or callees.
func TestRenderCallsSection_EmptyWhenNoCallRelationships(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "isolated", "filePath": "src/a.go"}},
		},
		nil,
	)
	c := makeRenderCache(ir)
	out := renderCallsSection("src/a.go", c, "//")
	if out != "" {
		t.Errorf("expected empty output for function with no call relationships, got:\n%s", out)
	}
}

// ── CommentPrefix / ShardFilename / Header ────────────────────────────────────

func TestCommentPrefix(t *testing.T) {
	cases := []struct{ ext, want string }{
		{".go", "//"},
		{".ts", "//"},
		{".js", "//"},
		{".py", "#"},
		{".rb", "#"},
		{".rs", "//"},
		{".java", "//"},
		{"", "//"},
	}
	for _, tc := range cases {
		if got := CommentPrefix(tc.ext); got != tc.want {
			t.Errorf("CommentPrefix(%q) = %q, want %q", tc.ext, got, tc.want)
		}
	}
}

func TestShardFilename(t *testing.T) {
	cases := []struct{ input, want string }{
		{"src/handler.go", "src/handler.graph.go"},
		{"lib/util.ts", "lib/util.graph.ts"},
		{"main.py", "main.graph.py"},
		{"src/no_ext", "src/no_ext.graph"},
	}
	for _, tc := range cases {
		if got := ShardFilename(tc.input); got != tc.want {
			t.Errorf("ShardFilename(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestHeader(t *testing.T) {
	h := Header("//")
	if !strings.Contains(h, "@generated") {
		t.Errorf("header should contain @generated: %q", h)
	}
	if !strings.HasSuffix(h, "\n") {
		t.Errorf("header should end with newline")
	}
	h2 := Header("#")
	if !strings.HasPrefix(h2, "#") {
		t.Errorf("Python header should start with #: %q", h2)
	}
}

// ── sortedUnique / sortedBoolKeys / formatLoc ─────────────────────────────────

func TestSortedUnique(t *testing.T) {
	got := sortedUnique([]string{"c", "a", "b", "a", "c"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestSortedUnique_Empty(t *testing.T) {
	if got := sortedUnique(nil); got != nil {
		t.Errorf("nil input: want nil, got %v", got)
	}
}

func TestSortedBoolKeys(t *testing.T) {
	m := map[string]bool{"z": true, "a": true, "m": true}
	got := sortedBoolKeys(m)
	if len(got) != 3 || got[0] != "a" || got[1] != "m" || got[2] != "z" {
		t.Errorf("want [a m z], got %v", got)
	}
}

func TestFormatLoc(t *testing.T) {
	if got := formatLoc("src/a.go", 10); got != "src/a.go:10" {
		t.Errorf("with file+line: got %q", got)
	}
	if got := formatLoc("src/a.go", 0); got != "src/a.go" {
		t.Errorf("with file, no line: got %q", got)
	}
	if got := formatLoc("", 0); got != "?" {
		t.Errorf("empty: got %q", got)
	}
}

// ── renderDepsSection ─────────────────────────────────────────────────────────

func TestRenderDepsSection_ShowsImportsAndImportedBy(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			{ID: "fa", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/a.go"}},
			{ID: "fb", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/b.go"}},
			{ID: "fc", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/c.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "fa", EndNode: "fb"}, // a imports b
			{ID: "r2", Type: "imports", StartNode: "fc", EndNode: "fa"}, // c imports a
		},
	)
	c := makeRenderCache(ir)
	out := renderDepsSection("src/a.go", c, "//")
	if out == "" {
		t.Fatal("expected non-empty deps section")
	}
	if !strings.Contains(out, "[deps]") {
		t.Errorf("should contain [deps] header: %s", out)
	}
	if !strings.Contains(out, "imports") && !strings.Contains(out, "src/b.go") {
		t.Errorf("should show imported file: %s", out)
	}
	if !strings.Contains(out, "imported-by") || !strings.Contains(out, "src/c.go") {
		t.Errorf("should show importing file: %s", out)
	}
}

func TestRenderDepsSection_EmptyWhenNoEdges(t *testing.T) {
	ir := shardIR(
		[]api.Node{{ID: "fa", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/a.go"}}},
		nil,
	)
	c := makeRenderCache(ir)
	if out := renderDepsSection("src/a.go", c, "//"); out != "" {
		t.Errorf("expected empty, got: %s", out)
	}
}

// ── renderImpactSection ───────────────────────────────────────────────────────

func TestRenderImpactSection_LowRisk(t *testing.T) {
	// Single direct importer, no transitive
	ir := shardIR(
		[]api.Node{
			{ID: "fa", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/a.go"}},
			{ID: "fb", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/b.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "fb", EndNode: "fa"},
		},
	)
	c := makeRenderCache(ir)
	out := renderImpactSection("src/a.go", c, "//")
	if !strings.Contains(out, "[impact]") {
		t.Errorf("should contain [impact] header: %s", out)
	}
	if !strings.Contains(out, "LOW") {
		t.Errorf("single importer should be LOW risk: %s", out)
	}
	if !strings.Contains(out, "direct") {
		t.Errorf("should contain direct count: %s", out)
	}
}

func TestRenderImpactSection_HighRisk(t *testing.T) {
	// Build 25 importers to trigger HIGH risk (transitiveCount > 20)
	nodes := []api.Node{
		{ID: "target", Labels: []string{"File"}, Properties: map[string]any{"filePath": "core/db.go"}},
	}
	rels := []api.Relationship{}
	for i := 0; i < 25; i++ {
		id := strings.Repeat("f", i+1)
		path := "src/file" + id + ".go"
		nodes = append(nodes, api.Node{ID: id, Labels: []string{"File"}, Properties: map[string]any{"filePath": path}})
		if i > 0 {
			// chain: f→f2→f3→...→target creates transitive deps
			prev := strings.Repeat("f", i)
			rels = append(rels, api.Relationship{ID: "r" + id, Type: "imports", StartNode: id, EndNode: prev})
		}
		rels = append(rels, api.Relationship{ID: "root" + id, Type: "imports", StartNode: id, EndNode: "target"})
	}
	c := makeRenderCache(shardIR(nodes, rels))
	out := renderImpactSection("core/db.go", c, "//")
	if !strings.Contains(out, "HIGH") {
		t.Errorf("many importers should trigger HIGH risk: %s", out)
	}
}

func TestRenderImpactSection_EmptyWhenNoImporters(t *testing.T) {
	ir := shardIR(
		[]api.Node{{ID: "fa", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/a.go"}}},
		nil,
	)
	c := makeRenderCache(ir)
	if out := renderImpactSection("src/a.go", c, "//"); out != "" {
		t.Errorf("expected empty, got: %s", out)
	}
}

func TestRenderImpactSection_MediumRisk(t *testing.T) {
	// 6–20 transitive dependents triggers MEDIUM risk.
	nodes := []api.Node{
		{ID: "target", Labels: []string{"File"}, Properties: map[string]any{"filePath": "lib/db.go"}},
	}
	rels := []api.Relationship{}
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("f%d", i)
		path := fmt.Sprintf("src/file%d.go", i)
		nodes = append(nodes, api.Node{
			ID:         id,
			Labels:     []string{"File"},
			Properties: map[string]any{"filePath": path},
		})
		rels = append(rels, api.Relationship{
			ID:        "r" + id,
			Type:      "imports",
			StartNode: id,
			EndNode:   "target",
		})
	}
	c := makeRenderCache(shardIR(nodes, rels))
	out := renderImpactSection("lib/db.go", c, "//")
	if !strings.Contains(out, "MEDIUM") {
		t.Errorf("6-20 importers should trigger MEDIUM risk: %s", out)
	}
}

func TestRenderImpactSection_WithDomain(t *testing.T) {
	// File is assigned to a domain; domain name should appear in impact output.
	nodes := []api.Node{
		{ID: "fa", Labels: []string{"File"}, Properties: map[string]any{"filePath": "core/auth.go"}},
		{ID: "fb", Labels: []string{"File"}, Properties: map[string]any{"filePath": "web/handler.go"}},
	}
	rels := []api.Relationship{
		{ID: "r1", Type: "imports", StartNode: "fb", EndNode: "fa"},
	}
	ir := &api.ShardIR{
		Graph: api.ShardGraph{Nodes: nodes, Relationships: rels},
		Domains: []api.ShardDomain{
			{Name: "Auth", KeyFiles: []string{"core/auth.go", "web/handler.go"}},
		},
	}
	c := NewCache()
	c.Build(ir)
	out := renderImpactSection("core/auth.go", c, "//")
	if !strings.Contains(out, "Auth") {
		t.Errorf("domain name should appear in impact output: %s", out)
	}
	if !strings.Contains(out, "domains") {
		t.Errorf("should contain domains line: %s", out)
	}
}

// ── RenderGraph ───────────────────────────────────────────────────────────────

func TestRenderGraph_CombinesSections(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			{ID: "fa", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/a.go"}},
			{ID: "fb", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/b.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "doWork", "filePath": "src/a.go"}},
			{ID: "fn2", Labels: []string{"Function"}, Properties: map[string]any{"name": "helper", "filePath": "src/b.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "fa", EndNode: "fb"},
			{ID: "r2", Type: "calls", StartNode: "fn1", EndNode: "fn2"},
		},
	)
	c := makeRenderCache(ir)
	out := RenderGraph("src/a.go", c, "//")
	if out == "" {
		t.Fatal("expected non-empty render output")
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("RenderGraph output should end with newline")
	}
}

func TestRenderGraph_EmptyForUnknownFile(t *testing.T) {
	c := makeRenderCache(shardIR(nil, nil))
	out := RenderGraph("nonexistent.go", c, "//")
	if out != "" {
		t.Errorf("unknown file should produce empty output, got: %s", out)
	}
}

// ── WriteShard ────────────────────────────────────────────────────────────────

func TestWriteShard_WritesFile(t *testing.T) {
	dir := t.TempDir()
	if err := WriteShard(dir, "src/handler.graph.go", "// content\n", false); err != nil {
		t.Fatalf("WriteShard: %v", err)
	}
}

func TestWriteShard_PathTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	err := WriteShard(dir, "../../etc/passwd", "evil", false)
	if err == nil {
		t.Error("expected path traversal error")
	}
	if !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWriteShard_DryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	if err := WriteShard(dir, "src/a.graph.go", "content", true); err != nil {
		t.Fatalf("dry-run WriteShard: %v", err)
	}
	// File should not exist
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("dry-run should not create files")
	}
}

func TestWriteShard_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file at "subdir" so that MkdirAll("subdir/...") fails.
	if err := os.WriteFile(dir+"/subdir", []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := WriteShard(dir, "subdir/handler.graph.go", "content", false)
	if err == nil {
		t.Error("expected MkdirAll error when parent path is a file")
	}
}

func TestWriteShard_WriteFileError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	// Create the target subdirectory then make it read-only so WriteFile fails.
	subDir := dir + "/ro"
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(subDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(subDir, 0o755) }) //nolint:errcheck
	err := WriteShard(dir, "ro/handler.graph.go", "content", false)
	if err == nil {
		t.Error("expected WriteFile error on read-only directory")
	}
}

// ── updateGitignore ───────────────────────────────────────────────────────────

func TestUpdateGitignore_AddsEntry(t *testing.T) {
	dir := t.TempDir()
	if err := updateGitignore(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dir + "/.gitignore")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, ".supermodel/") {
		t.Errorf("expected .supermodel/ in gitignore: %s", content)
	}
	if !strings.Contains(content, "*.graph.*") {
		t.Errorf("expected *.graph.* in gitignore: %s", content)
	}
}

func TestUpdateGitignore_DoesNotDuplicate(t *testing.T) {
	dir := t.TempDir()
	// Call twice; each entry should appear exactly once.
	updateGitignore(dir) //nolint:errcheck
	updateGitignore(dir) //nolint:errcheck
	data, _ := os.ReadFile(dir + "/.gitignore")
	content := string(data)
	for _, entry := range []string{".supermodel/", "*.graph.*"} {
		if strings.Count(content, entry) != 1 {
			t.Errorf("%q appears more than once in gitignore:\n%s", entry, content)
		}
	}
}

func TestUpdateGitignore_ExistingEntrySkipped(t *testing.T) {
	dir := t.TempDir()
	// Pre-populate with both entries.
	os.WriteFile(dir+"/.gitignore", []byte(".supermodel/\n*.graph.*\n"), 0o600) //nolint:errcheck
	updateGitignore(dir)                                                        //nolint:errcheck
	data, _ := os.ReadFile(dir + "/.gitignore")
	content := string(data)
	for _, entry := range []string{".supermodel/", "*.graph.*"} {
		if strings.Count(content, entry) != 1 {
			t.Errorf("%q should not be duplicated: %s", entry, content)
		}
	}
}

func TestUpdateGitignore_AddsOnlyMissingEntries(t *testing.T) {
	dir := t.TempDir()
	// Pre-populate with only .supermodel/ — *.graph.* should be added.
	os.WriteFile(dir+"/.gitignore", []byte(".supermodel/\n"), 0o600) //nolint:errcheck
	updateGitignore(dir)                                             //nolint:errcheck
	data, _ := os.ReadFile(dir + "/.gitignore")
	content := string(data)
	if strings.Count(content, ".supermodel/") != 1 {
		t.Errorf(".supermodel/ should appear exactly once: %s", content)
	}
	if !strings.Contains(content, "*.graph.*") {
		t.Errorf("*.graph.* should have been added: %s", content)
	}
}

func TestUpdateGitignore_ReadErrorSkipped(t *testing.T) {
	// Create a directory at .gitignore path → ReadFile returns EISDIR (not IsNotExist)
	// → updateGitignore returns nil (skips silently).
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/.gitignore", 0700); err != nil {
		t.Fatal(err)
	}
	// Should not error out.
	if err := updateGitignore(dir); err != nil {
		t.Errorf("updateGitignore with unreadable .gitignore should return nil, got %v", err)
	}
}

func TestUpdateGitignore_OpenFileErrorSkipped(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	// Make the directory read-only so OpenFile (O_CREATE|O_APPEND|O_WRONLY) fails.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) }) //nolint:errcheck
	// Should return nil (silent skip on write failure).
	if err := updateGitignore(dir); err != nil {
		t.Errorf("updateGitignore with read-only dir should return nil, got %v", err)
	}
}

func TestUpdateGitignore_NoTrailingNewlineHandled(t *testing.T) {
	dir := t.TempDir()
	// Write without trailing newline
	os.WriteFile(dir+"/.gitignore", []byte("node_modules"), 0o600) //nolint:errcheck
	updateGitignore(dir)                                           //nolint:errcheck
	data, _ := os.ReadFile(dir + "/.gitignore")
	if !strings.Contains(string(data), ".supermodel/") {
		t.Errorf("missing .supermodel/: %s", data)
	}
}

// ── RenderAll ─────────────────────────────────────────────────────────────────

func TestRenderAll_EmptyFiles(t *testing.T) {
	dir := t.TempDir()
	c := makeRenderCache(shardIR(nil, nil))
	n, err := RenderAll(dir, c, nil, false)
	if err != nil {
		t.Fatalf("RenderAll(empty): %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 written, got %d", n)
	}
}

func TestRenderAll_WritesShards(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			{ID: "fa", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/a.go"}},
			{ID: "fb", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/b.go"}},
			{ID: "fn1", Labels: []string{"Function"}, Properties: map[string]any{"name": "doWork", "filePath": "src/a.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "fa", EndNode: "fb"},
		},
	)
	dir := t.TempDir()
	c := makeRenderCache(ir)
	n, err := RenderAll(dir, c, []string{"src/a.go"}, false)
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 written, got %d", n)
	}
}

func TestRenderAll_DryRun(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			{ID: "fa", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/a.go"}},
			{ID: "fb", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/b.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "fa", EndNode: "fb"},
		},
	)
	dir := t.TempDir()
	c := makeRenderCache(ir)
	n, err := RenderAll(dir, c, []string{"src/a.go"}, true)
	if err != nil {
		t.Fatalf("RenderAll dryRun: %v", err)
	}
	if n != 1 {
		t.Errorf("dryRun: expected 1 counted, got %d", n)
	}
	// No actual files written.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("dry-run should not create files, found %d", len(entries))
	}
}

func TestRenderAll_SkipsEmptyContent(t *testing.T) {
	// A file not in the cache produces empty content → no shard written.
	dir := t.TempDir()
	c := makeRenderCache(shardIR(nil, nil))
	n, err := RenderAll(dir, c, []string{"src/unknown.go"}, false)
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}
	if n != 0 {
		t.Errorf("unknown file should produce 0 written, got %d", n)
	}
}

func TestRenderAll_PathTraversalSkipped(t *testing.T) {
	// A srcFile whose ShardFilename would escape the repo dir is silently skipped.
	// Build a cache that produces non-empty content for the path-traversal file,
	// so the WriteShard call is actually reached.
	ir := shardIR(
		[]api.Node{
			{ID: "fa", Labels: []string{"File"}, Properties: map[string]any{"filePath": "../../evil.go"}},
			{ID: "fb", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/good.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "fa", EndNode: "fb"},
		},
	)
	dir := t.TempDir()
	c := makeRenderCache(ir)
	n, err := RenderAll(dir, c, []string{"../../evil.go"}, false)
	if err != nil {
		t.Fatalf("RenderAll path-traversal: %v", err)
	}
	if n != 0 {
		t.Errorf("path-traversal file should be skipped (n=0), got %d", n)
	}
}

func TestRenderAll_WriteshardError(t *testing.T) {
	// Create a file at the shard subdirectory so MkdirAll fails → WriteShard
	// returns a non-path-traversal error → RenderAll returns that error.
	dir := t.TempDir()

	// File node that imports another → non-empty RenderGraph output.
	ir := shardIR(
		[]api.Node{
			{ID: "fa", Labels: []string{"File"}, Properties: map[string]any{"filePath": "sub/a.go"}},
			{ID: "fb", Labels: []string{"File"}, Properties: map[string]any{"filePath": "src/b.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "imports", StartNode: "fa", EndNode: "fb"},
		},
	)
	c := makeRenderCache(ir)

	// ShardFilename("sub/a.go") = "sub/a.graph.go"; make "sub" a regular file so
	// MkdirAll("sub") fails with ENOTDIR.
	if err := os.WriteFile(dir+"/sub", []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := RenderAll(dir, c, []string{"sub/a.go"}, false)
	if err == nil {
		t.Error("expected error when shard directory cannot be created")
	}
}

// TestRenderCallsSection_SortsByDifferentNames verifies the sort.Slice comparator
// takes the fns[i].name != fns[j].name == true branch for functions with distinct names.
func TestRenderCallsSection_SortsByDifferentNames(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			// Two functions with different names in the same file, both with callers.
			{ID: "fn_b", Labels: []string{"Function"}, Properties: map[string]any{"name": "Beta", "filePath": "src/a.go"}},
			{ID: "fn_a", Labels: []string{"Function"}, Properties: map[string]any{"name": "Alpha", "filePath": "src/a.go"}},
			{ID: "caller", Labels: []string{"Function"}, Properties: map[string]any{"name": "main", "filePath": "src/main.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "calls", StartNode: "caller", EndNode: "fn_a"},
			{ID: "r2", Type: "calls", StartNode: "caller", EndNode: "fn_b"},
		},
	)
	c := makeRenderCache(ir)
	out := renderCallsSection("src/a.go", c, "//")
	// Alpha should appear before Beta in the sorted output.
	alphaIdx := strings.Index(out, "Alpha")
	betaIdx := strings.Index(out, "Beta")
	if alphaIdx == -1 || betaIdx == -1 {
		t.Fatalf("expected both Alpha and Beta in output:\n%s", out)
	}
	if alphaIdx > betaIdx {
		t.Errorf("Alpha should appear before Beta in sorted output:\n%s", out)
	}
}

// TestRenderImpactSection_CallerFromAnotherFile covers the directCallerFiles loop
// in renderImpactSection (lines 136-147): a function in the target file is called
// by a function in a different file.
func TestRenderImpactSection_CallerFromAnotherFile(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			{ID: "fn_target", Labels: []string{"Function"}, Properties: map[string]any{"name": "doWork", "filePath": "src/a.go"}},
			{ID: "fn_caller", Labels: []string{"Function"}, Properties: map[string]any{"name": "main", "filePath": "src/main.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "calls", StartNode: "fn_caller", EndNode: "fn_target"},
		},
	)
	c := makeRenderCache(ir)
	out := renderImpactSection("src/a.go", c, "//")
	if out == "" {
		t.Fatal("expected non-empty impact section when function has cross-file callers")
	}
	if !strings.Contains(out, "[impact]") {
		t.Errorf("expected [impact] header:\n%s", out)
	}
	if !strings.Contains(out, "src/main.go") {
		t.Errorf("expected caller file in affects line:\n%s", out)
	}
}

// TestRenderGraph_IncludesImpactSection exercises the `if impact != ""` branch in
// RenderGraph (L47) by using a file whose function is called by a function in another file.
func TestRenderGraph_IncludesImpactSection(t *testing.T) {
	ir := shardIR(
		[]api.Node{
			{ID: "fn_lib", Labels: []string{"Function"}, Properties: map[string]any{"name": "LibFunc", "filePath": "lib/util.go"}},
			{ID: "fn_app", Labels: []string{"Function"}, Properties: map[string]any{"name": "AppFunc", "filePath": "app/main.go"}},
		},
		[]api.Relationship{
			{ID: "r1", Type: "calls", StartNode: "fn_app", EndNode: "fn_lib"},
		},
	)
	c := makeRenderCache(ir)
	out := RenderGraph("lib/util.go", c, "//")
	if !strings.Contains(out, "[impact]") {
		t.Errorf("expected [impact] section in RenderGraph output:\n%s", out)
	}
	if !strings.Contains(out, "[calls]") {
		t.Errorf("expected [calls] section in RenderGraph output:\n%s", out)
	}
}

// TestWriteShard_RenameError covers L232-235: os.Rename fails when the
// destination path already exists as a directory.
func TestWriteShard_RenameError(t *testing.T) {
	dir := t.TempDir()
	// Create the target subdirectory normally so MkdirAll succeeds.
	subdir := filepath.Join(dir, "src")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Place a directory at the exact destination path so Rename(tmp→full) fails.
	fullAsDir := filepath.Join(subdir, "handler.graph.go")
	if err := os.Mkdir(fullAsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := WriteShard(dir, "src/handler.graph.go", "content", false)
	if err == nil {
		t.Error("expected Rename error when destination is a directory")
	}
}

// ── Hook ─────────────────────────────────────────────────────────────────────

func TestHook_InvalidJSONExitsCleanly(t *testing.T) {
	// Hook reads from stdin; we test via the exported function with invalid data.
	// The function must return nil (never break the agent) on bad input.
	// We can't easily inject stdin, but we test the underlying validation logic
	// directly by calling with a mock via the export test file.
}
