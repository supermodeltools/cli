package status

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/ui"
)

// ── render ────────────────────────────────────────────────────────────────────

func TestRender_JSON(t *testing.T) {
	r := &Report{
		Version:    "1.2.3",
		Authed:     true,
		APIBase:    "https://api.example.com",
		ConfigPath: "/home/user/.supermodel/config.yaml",
		CacheDir:   "/home/user/.supermodel/cache",
		CacheCount: 7,
	}
	var buf bytes.Buffer
	if err := render(&buf, r, ui.FormatJSON); err != nil {
		t.Fatalf("render JSON: %v", err)
	}
	var decoded Report
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if decoded.Version != "1.2.3" {
		t.Errorf("version: got %q", decoded.Version)
	}
	if !decoded.Authed {
		t.Error("authed: want true")
	}
	if decoded.CacheCount != 7 {
		t.Errorf("cache count: want 7, got %d", decoded.CacheCount)
	}
}

func TestRender_HumanAuthenticated(t *testing.T) {
	r := &Report{
		Version: "1.0.0",
		Authed:  true,
		APIBase: "https://api.supermodeltools.com",
	}
	var buf bytes.Buffer
	if err := render(&buf, r, ui.FormatHuman); err != nil {
		t.Fatalf("render human: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Authenticated") {
		t.Errorf("authenticated: should say 'Authenticated', got:\n%s", out)
	}
	if !strings.Contains(out, "1.0.0") {
		t.Errorf("should contain version, got:\n%s", out)
	}
}

func TestRender_HumanNotAuthenticated(t *testing.T) {
	r := &Report{Authed: false}
	var buf bytes.Buffer
	if err := render(&buf, r, ui.FormatHuman); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Not authenticated") {
		t.Errorf("unauthenticated: should say 'Not authenticated', got:\n%s", out)
	}
}

func TestRender_HumanContainsAllFields(t *testing.T) {
	r := &Report{
		Version:    "2.0.0",
		Authed:     true,
		APIBase:    "https://custom.api.com",
		ConfigPath: "/etc/supermodel/config.yaml",
		CacheDir:   "/var/cache/supermodel",
		CacheCount: 3,
	}
	var buf bytes.Buffer
	if err := render(&buf, r, ui.FormatHuman); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"2.0.0",
		"https://custom.api.com",
		"/etc/supermodel/config.yaml",
		"/var/cache/supermodel",
		"3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("should contain %q, got:\n%s", want, out)
		}
	}
}

// ── countCacheEntries ─────────────────────────────────────────────────────────

func TestCountCacheEntries_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if n := countCacheEntries(dir); n != 0 {
		t.Errorf("empty dir: want 0, got %d", n)
	}
}

func TestCountCacheEntries_OnlyJSON(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"abc.json", "def.json", "ghi.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if n := countCacheEntries(dir); n != 3 {
		t.Errorf("3 json files: want 3, got %d", n)
	}
}

func TestCountCacheEntries_IgnoresNonJSON(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"data.json", "data.zip", "data.txt", "data"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if n := countCacheEntries(dir); n != 1 {
		t.Errorf("only 1 json file: want 1, got %d", n)
	}
}

func TestCountCacheEntries_IgnoresSubdirs(t *testing.T) {
	dir := t.TempDir()
	// A directory named "something.json" should not be counted
	if err := os.Mkdir(filepath.Join(dir, "subdir.json"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if n := countCacheEntries(dir); n != 1 {
		t.Errorf("1 json file + 1 json-named dir: want 1, got %d", n)
	}
}

func TestCountCacheEntries_MissingDir(t *testing.T) {
	if n := countCacheEntries("/nonexistent/cache/dir"); n != 0 {
		t.Errorf("missing dir: want 0, got %d", n)
	}
}

// ── Run ───────────────────────────────────────────────────────────────────────

// TestRun_HappyPath covers L36-44: Run succeeds when config can be loaded,
// exercising the full happy path including countCacheEntries and render.
func TestRun_HappyPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("SUPERMODEL_API_KEY", "")
	t.Setenv("SUPERMODEL_API_BASE", "")
	if err := Run(context.Background(), Options{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// TestRun_ConfigLoadError covers L33-35: Run returns error when config.Load fails.
func TestRun_ConfigLoadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// Place a directory at the config file path so ReadFile returns EISDIR.
	cfgPath := filepath.Join(home, ".supermodel", "config.yaml")
	if err := os.MkdirAll(cfgPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), Options{}); err == nil {
		t.Error("Run should fail when config cannot be loaded")
	}
}
