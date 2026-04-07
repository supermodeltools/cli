package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

// Override the cache directory to use a temp dir for each test.
func withTempCacheDir(t *testing.T) func() {
	t.Helper()
	orig := os.Getenv("HOME")
	tmp := t.TempDir()
	// config.Dir() returns filepath.Join(os.UserHomeDir(), ".supermodel")
	// We redirect it by pointing HOME at a temp dir.
	t.Setenv("HOME", tmp)
	return func() { t.Setenv("HOME", orig) }
}

// ── HashFile ──────────────────────────────────────────────────────────────────

func TestHashFile_Deterministic(t *testing.T) {
	f := writeTempFile(t, []byte("hello world"))
	h1, err := HashFile(f)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	h2, err := HashFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("same file: want identical hashes, got %q vs %q", h1, h2)
	}
}

func TestHashFile_DifferentContent(t *testing.T) {
	f1 := writeTempFile(t, []byte("hello"))
	f2 := writeTempFile(t, []byte("world"))
	h1, _ := HashFile(f1)
	h2, _ := HashFile(f2)
	if h1 == h2 {
		t.Error("different content should produce different hashes")
	}
}

func TestHashFile_KnownSHA256(t *testing.T) {
	// SHA-256 of empty file is e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	f := writeTempFile(t, []byte{})
	h, err := HashFile(f)
	if err != nil {
		t.Fatal(err)
	}
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if h != want {
		t.Errorf("empty file hash: want %q, got %q", want, h)
	}
}

func TestHashFile_HexEncoded(t *testing.T) {
	f := writeTempFile(t, []byte("data"))
	h, err := HashFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 64 {
		t.Errorf("SHA-256 hex should be 64 chars, got %d: %q", len(h), h)
	}
	for _, c := range h {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("non-hex character %q in hash %q", c, h)
			break
		}
	}
}

func TestHashFile_MissingFile(t *testing.T) {
	_, err := HashFile("/nonexistent/path/file.zip")
	if err == nil {
		t.Error("missing file: want error, got nil")
	}
}

// ── Get / Put / Evict ─────────────────────────────────────────────────────────

func TestGetPut_RoundTrip(t *testing.T) {
	withTempCacheDir(t)

	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "n1", Labels: []string{"Function"}, Properties: map[string]any{"name": "main"}},
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "CALLS", StartNode: "n1", EndNode: "n2"},
		},
	}

	if err := Put("testhash123", g); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := Get("testhash123")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil after Put")
		return
	}
	if len(got.Nodes) != 1 || got.Nodes[0].ID != "n1" {
		t.Errorf("round-trip nodes: got %v", got.Nodes)
	}
	if len(got.Relationships) != 1 || got.Relationships[0].Type != "CALLS" {
		t.Errorf("round-trip rels: got %v", got.Relationships)
	}
}

func TestGet_CacheMiss(t *testing.T) {
	withTempCacheDir(t)

	got, err := Get("nonexistent-hash")
	if err != nil {
		t.Fatalf("Get miss: want nil error, got %v", err)
	}
	if got != nil {
		t.Errorf("Get miss: want nil graph, got %v", got)
	}
}

func TestPut_CreatesDirectory(t *testing.T) {
	withTempCacheDir(t)

	g := &api.Graph{}
	if err := Put("hashval", g); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Verify file exists
	cacheFile := filepath.Join(dir(), "hashval.json")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Errorf("cache file not created: %v", err)
	}
}

func TestEvict_RemovesEntry(t *testing.T) {
	withTempCacheDir(t)

	g := &api.Graph{}
	if err := Put("evict-me", g); err != nil {
		t.Fatal(err)
	}
	if err := Evict("evict-me"); err != nil {
		t.Fatalf("Evict: %v", err)
	}

	got, err := Get("evict-me")
	if err != nil {
		t.Fatalf("Get after Evict: %v", err)
	}
	if got != nil {
		t.Error("Get after Evict: want nil, got non-nil")
	}
}

func TestEvict_MissingIsNoop(t *testing.T) {
	withTempCacheDir(t)

	if err := Evict("no-such-hash"); err != nil {
		t.Errorf("Evict missing entry: want nil error, got %v", err)
	}
}

func TestPut_OverwritesExisting(t *testing.T) {
	withTempCacheDir(t)

	g1 := &api.Graph{Nodes: []api.Node{{ID: "old"}}}
	g2 := &api.Graph{Nodes: []api.Node{{ID: "new"}}}

	if err := Put("same-hash", g1); err != nil {
		t.Fatal(err)
	}
	if err := Put("same-hash", g2); err != nil {
		t.Fatal(err)
	}

	got, err := Get("same-hash")
	if err != nil || got == nil {
		t.Fatalf("Get: err=%v graph=%v", err, got)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].ID != "new" {
		t.Errorf("should return overwritten value, got %v", got.Nodes)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "hashtest-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}
