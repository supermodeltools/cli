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

// ── PutJSON / GetJSON ─────────────────────────────────────────────────────────

func TestPutGetJSON_RoundTrip(t *testing.T) {
	withTempCacheDir(t)

	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	v := payload{Name: "deadcode", Count: 42}

	if err := PutJSON("jsonhash1", v); err != nil {
		t.Fatalf("PutJSON: %v", err)
	}

	var got payload
	hit, err := GetJSON("jsonhash1", &got)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if !hit {
		t.Fatal("GetJSON: expected cache hit")
	}
	if got.Name != "deadcode" || got.Count != 42 {
		t.Errorf("GetJSON: got %+v, want {deadcode 42}", got)
	}
}

func TestGetJSON_Miss(t *testing.T) {
	withTempCacheDir(t)

	var v any
	hit, err := GetJSON("nonexistent", &v)
	if err != nil {
		t.Fatalf("GetJSON miss: want nil error, got %v", err)
	}
	if hit {
		t.Error("GetJSON miss: want hit=false")
	}
}

func TestPutGetJSON_Overwrite(t *testing.T) {
	withTempCacheDir(t)

	if err := PutJSON("overwrite-key", map[string]string{"v": "1"}); err != nil {
		t.Fatal(err)
	}
	if err := PutJSON("overwrite-key", map[string]string{"v": "2"}); err != nil {
		t.Fatal(err)
	}

	var got map[string]string
	hit, err := GetJSON("overwrite-key", &got)
	if err != nil || !hit {
		t.Fatalf("GetJSON: hit=%v err=%v", hit, err)
	}
	if got["v"] != "2" {
		t.Errorf("expected overwritten value '2', got %q", got["v"])
	}
}

func TestGet_CorruptJSON(t *testing.T) {
	withTempCacheDir(t)

	// Write malformed JSON directly into the cache file.
	cacheFile := filepath.Join(dir(), "badhash.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, []byte("{not valid json}"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Get("badhash")
	if err == nil {
		t.Error("Get with corrupt JSON should return error")
	}
}

func TestGetJSON_CorruptJSON(t *testing.T) {
	withTempCacheDir(t)

	cacheFile := filepath.Join(dir(), "corruptkey.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, []byte("{not valid}"), 0o600); err != nil {
		t.Fatal(err)
	}

	var v any
	_, err := GetJSON("corruptkey", &v)
	if err == nil {
		t.Error("GetJSON with corrupt JSON should return error")
	}
}

func TestGet_NonNotExistError(t *testing.T) {
	withTempCacheDir(t)

	// Create a directory where the cache file would be, so ReadFile returns
	// a non-IsNotExist error (it's a directory, not a file).
	cacheFile := filepath.Join(dir(), "dirkey.json")
	if err := os.MkdirAll(cacheFile, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := Get("dirkey")
	if err == nil {
		t.Error("Get with directory-as-file should return error")
	}
}

func TestGetJSON_NonNotExistError(t *testing.T) {
	withTempCacheDir(t)

	cacheFile := filepath.Join(dir(), "dirkey2.json")
	if err := os.MkdirAll(cacheFile, 0o700); err != nil {
		t.Fatal(err)
	}

	var v any
	_, err := GetJSON("dirkey2", &v)
	if err == nil {
		t.Error("GetJSON with directory-as-file should return error")
	}
}

func TestPut_MkdirAllError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Create a regular file where ~/.supermodel would be → MkdirAll fails.
	smFile := home + "/.supermodel"
	if err := os.WriteFile(smFile, []byte("not a dir"), 0600); err != nil {
		t.Fatal(err)
	}
	g := &api.Graph{}
	if err := Put("any-hash", g); err == nil {
		t.Error("Put should fail when cache dir cannot be created")
	}
}

func TestPutJSON_MarshalError(t *testing.T) {
	withTempCacheDir(t)
	// Channels cannot be JSON-marshaled; json.Marshal returns an error.
	if err := PutJSON("marshal-fail", make(chan int)); err == nil {
		t.Error("PutJSON should fail when value cannot be JSON-marshaled")
	}
}

func TestPutJSON_MkdirAllError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	smFile := home + "/.supermodel"
	if err := os.WriteFile(smFile, []byte("not a dir"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := PutJSON("any-hash", map[string]string{"k": "v"}); err == nil {
		t.Error("PutJSON should fail when cache dir cannot be created")
	}
}

func TestPutJSON_WriteFileError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheDir := home + "/.supermodel/cache"
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cacheDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(cacheDir, 0755) }) //nolint:errcheck
	if err := PutJSON("any-hash", map[string]string{"k": "v"}); err == nil {
		t.Error("PutJSON should fail when temp file cannot be written")
	}
}

func TestPut_RenameError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheDir := filepath.Join(home, ".supermodel", "cache")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Create the destination path as a directory so Rename from .tmp → dest fails.
	hash := "rename-error-hash"
	destDir := filepath.Join(cacheDir, hash+".json")
	if err := os.Mkdir(destDir, 0o700); err != nil {
		t.Fatal(err)
	}
	g := &api.Graph{}
	if err := Put(hash, g); err == nil {
		t.Error("Put should fail when rename destination is a directory")
	}
}

func TestPutJSON_RenameError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheDir := filepath.Join(home, ".supermodel", "cache")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Block Rename by placing a directory at the destination path.
	hash := "rename-error-json-hash"
	destDir := filepath.Join(cacheDir, hash+".json")
	if err := os.Mkdir(destDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := PutJSON(hash, map[string]string{"k": "v"}); err == nil {
		t.Error("PutJSON should fail when rename destination is a directory")
	}
}

func TestPut_WriteFileError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Create the cache dir but make it read-only so WriteFile fails.
	cacheDir := home + "/.supermodel/cache"
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cacheDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(cacheDir, 0755) }) //nolint:errcheck
	g := &api.Graph{}
	if err := Put("any-hash", g); err == nil {
		t.Error("Put should fail when temp file cannot be written")
	}
}

func TestHashFile_ReadError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	path := dir + "/secret.dat"
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0600) }) //nolint:errcheck
	_, err := HashFile(path)
	if err == nil {
		t.Error("HashFile should fail when file is not readable")
	}
}

func TestPut_MarshalError(t *testing.T) {
	withTempCacheDir(t)
	// A graph with a channel property cannot be JSON-marshaled.
	g := &api.Graph{
		Nodes: []api.Node{
			{ID: "n1", Labels: []string{"File"}, Properties: map[string]any{"bad": make(chan int)}},
		},
	}
	if err := Put("marshal-error-put", g); err == nil {
		t.Error("Put should fail when graph has non-JSON-serializable properties")
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
