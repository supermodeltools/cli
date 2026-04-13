package archdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── deriveRepoInfo ────────────────────────────────────────────────────────────

func TestDeriveRepoInfo_OwnerSlash(t *testing.T) {
	name, repoURL := deriveRepoInfo("myorg/myrepo", "/any/dir")
	if name != "myrepo" {
		t.Errorf("name = %q; want %q", name, "myrepo")
	}
	if repoURL != "https://github.com/myorg/myrepo" {
		t.Errorf("repoURL = %q; want %q", repoURL, "https://github.com/myorg/myrepo")
	}
}

func TestDeriveRepoInfo_PlainSlug(t *testing.T) {
	// no slash → treat as bare name, no repo URL
	name, repoURL := deriveRepoInfo("justname", "/any/dir")
	if name != "justname" {
		t.Errorf("name = %q; want %q", name, "justname")
	}
	if repoURL != "" {
		t.Errorf("repoURL = %q; want empty", repoURL)
	}
}

func TestDeriveRepoInfo_Empty(t *testing.T) {
	// empty slug → fall back to directory basename
	name, repoURL := deriveRepoInfo("", "/some/path/mydir")
	if name != "mydir" {
		t.Errorf("name = %q; want %q", name, "mydir")
	}
	if repoURL != "" {
		t.Errorf("repoURL = %q; want empty", repoURL)
	}
}

// ── extractPathPrefix ─────────────────────────────────────────────────────────

func TestExtractPathPrefix_WithPath(t *testing.T) {
	p := extractPathPrefix("https://myorg.github.io/myrepo")
	if p != "/myrepo" {
		t.Errorf("prefix = %q; want %q", p, "/myrepo")
	}
}

func TestExtractPathPrefix_RootOnly(t *testing.T) {
	p := extractPathPrefix("https://example.com/")
	if p != "" {
		t.Errorf("prefix = %q; want empty", p)
	}
}

func TestExtractPathPrefix_NoPath(t *testing.T) {
	p := extractPathPrefix("https://example.com")
	if p != "" {
		t.Errorf("prefix = %q; want empty", p)
	}
}

func TestExtractPathPrefix_InvalidURL(t *testing.T) {
	p := extractPathPrefix("://not-a-url")
	if p != "" {
		t.Errorf("prefix = %q; want empty for invalid URL", p)
	}
}

func TestExtractPathPrefix_NestedPath(t *testing.T) {
	p := extractPathPrefix("https://example.com/org/repo")
	if p != "/org/repo" {
		t.Errorf("prefix = %q; want %q", p, "/org/repo")
	}
}

func TestExtractPathPrefix_TrailingSlash(t *testing.T) {
	p := extractPathPrefix("https://example.com/myrepo/")
	if p != "/myrepo" {
		t.Errorf("prefix = %q; want %q", p, "/myrepo")
	}
}

// ── countFiles ────────────────────────────────────────────────────────────────

func TestCountFiles_Empty(t *testing.T) {
	dir := t.TempDir()
	if n := countFiles(dir, ".md"); n != 0 {
		t.Errorf("countFiles = %d; want 0", n)
	}
}

func TestCountFiles_MatchingExtension(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if n := countFiles(dir, ".md"); n != 3 {
		t.Errorf("countFiles = %d; want 3", n)
	}
}

func TestCountFiles_MixedExtensions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if n := countFiles(dir, ".html"); n != 1 {
		t.Errorf("countFiles = %d; want 1", n)
	}
}

func TestCountFiles_Recursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.md"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested.md"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if n := countFiles(dir, ".md"); n != 2 {
		t.Errorf("countFiles = %d; want 2", n)
	}
}

func TestCountFiles_NonExistentDir(t *testing.T) {
	// Walk fails silently; count returns 0
	if n := countFiles("/nonexistent-dir-archdocs-xyz", ".md"); n != 0 {
		t.Errorf("countFiles = %d; want 0 for non-existent dir", n)
	}
}

// ── writePssgConfig ───────────────────────────────────────────────────────────

func TestWritePssgConfig_WritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pssg.yaml")
	err := writePssgConfig(path, "MySite", "https://example.com", "https://github.com/org/repo", "repo", "/data", "/tpl", "/out", "/src")
	if err != nil {
		t.Fatalf("writePssgConfig: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `name: "MySite"`) {
		t.Error("config should contain site name")
	}
	if !strings.Contains(content, `base_url: "https://example.com"`) {
		t.Error("config should contain base_url")
	}
	if !strings.Contains(content, `repo_url: "https://github.com/org/repo"`) {
		t.Error("config should contain repo_url")
	}
}

func TestWritePssgConfig_WriteError(t *testing.T) {
	err := writePssgConfig("/nonexistent-dir-pssg/pssg.yaml", "S", "U", "R", "N", "D", "T", "O", "S")
	if err == nil {
		t.Error("expected error writing to non-existent directory")
	}
}

// ── rewritePathPrefix ─────────────────────────────────────────────────────────

func TestRewritePathPrefix_HTML(t *testing.T) {
	dir := t.TempDir()
	content := `<a href="/page">link</a><img src="/img.png">`
	path := filepath.Join(dir, "index.html")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := rewritePathPrefix(dir, "/prefix"); err != nil {
		t.Fatalf("rewritePathPrefix: %v", err)
	}
	data, _ := os.ReadFile(path)
	result := string(data)
	if !strings.Contains(result, `href="/prefix/page"`) {
		t.Errorf("href not rewritten: %s", result)
	}
	if !strings.Contains(result, `src="/prefix/img.png"`) {
		t.Errorf("src not rewritten: %s", result)
	}
}

func TestRewritePathPrefix_JS(t *testing.T) {
	dir := t.TempDir()
	content := `fetch("/api/data")`
	path := filepath.Join(dir, "main.js")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := rewritePathPrefix(dir, "/base"); err != nil {
		t.Fatalf("rewritePathPrefix: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `fetch("/base/api/data")`) {
		t.Errorf("fetch not rewritten: %s", string(data))
	}
}

func TestRewritePathPrefix_SkipsNonHTMLJS(t *testing.T) {
	dir := t.TempDir()
	content := `href="/page"`
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := rewritePathPrefix(dir, "/prefix"); err != nil {
		t.Fatalf("rewritePathPrefix: %v", err)
	}
	data, _ := os.ReadFile(path)
	// JSON file should be unchanged
	if string(data) != content {
		t.Errorf("non-html/js file should not be modified: %s", string(data))
	}
}

func TestRewritePathPrefix_NoChangesNeeded(t *testing.T) {
	dir := t.TempDir()
	content := `<html><body>no absolute paths</body></html>`
	path := filepath.Join(dir, "index.html")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := rewritePathPrefix(dir, "/prefix"); err != nil {
		t.Fatalf("rewritePathPrefix: %v", err)
	}
	data, _ := os.ReadFile(path)
	// unchanged
	if string(data) != content {
		t.Errorf("content should be unchanged: %s", string(data))
	}
}

func TestRewritePathPrefix_WindowsLocationHref(t *testing.T) {
	dir := t.TempDir()
	content := `window.location.href = "/"`
	path := filepath.Join(dir, "nav.js")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := rewritePathPrefix(dir, "/base"); err != nil {
		t.Fatalf("rewritePathPrefix: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `window.location.href = "/base/"`) {
		t.Errorf("window.location.href not rewritten: %s", string(data))
	}
}

func TestRewritePathPrefix_NonExistentDir(t *testing.T) {
	// Walk on non-existent dir returns error
	err := rewritePathPrefix("/nonexistent-dir-archdocs-rewrite", "/prefix")
	if err == nil {
		t.Error("expected error for non-existent dir")
	}
}

func TestRewritePathPrefix_ReadFileError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "index.html")
	if err := os.WriteFile(path, []byte(`<a href="/page">link</a>`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0600) }) //nolint:errcheck
	// ReadFile failure is silently ignored (returns nil)
	if err := rewritePathPrefix(dir, "/prefix"); err != nil {
		t.Errorf("expected no error when ReadFile fails (silently ignored): %v", err)
	}
}

func TestRewritePathPrefix_WriteFileError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "index.html")
	if err := os.WriteFile(path, []byte(`<a href="/page">link</a>`), 0600); err != nil {
		t.Fatal(err)
	}
	// Make file readable but not writable so WriteFile fails after rewrite
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0644) }) //nolint:errcheck
	err := rewritePathPrefix(dir, "/prefix")
	if err == nil {
		t.Error("expected error when WriteFile fails on read-only file")
	}
}

// ── resolveTemplates ──────────────────────────────────────────────────────────

func TestResolveTemplates_Override(t *testing.T) {
	override := t.TempDir()
	dir, cleanup, err := resolveTemplates(override)
	if err != nil {
		t.Fatalf("resolveTemplates: %v", err)
	}
	if cleanup != nil {
		t.Error("override should not return a cleanup function")
	}
	if dir != override {
		t.Errorf("dir = %q; want %q", dir, override)
	}
}

func TestResolveTemplates_MkdirTempError(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "nonexistent-tmp"))
	_, cleanup, err := resolveTemplates("")
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		t.Error("expected error when os.MkdirTemp fails")
	}
}

func TestResolveTemplates_Bundled(t *testing.T) {
	dir, cleanup, err := resolveTemplates("")
	if err != nil {
		t.Fatalf("resolveTemplates bundled: %v", err)
	}
	if cleanup == nil {
		t.Error("bundled templates should return a cleanup function")
	}
	defer cleanup()
	if dir == "" {
		t.Error("dir should not be empty")
	}
	// Verify the tmp dir exists and has files
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("bundled templates dir should have files")
	}
}
