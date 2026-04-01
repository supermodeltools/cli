package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIBase != DefaultAPIBase {
		t.Errorf("APIBase = %q, want %q", cfg.APIBase, DefaultAPIBase)
	}
	if cfg.Output != "human" {
		t.Errorf("Output = %q, want human", cfg.Output)
	}
}

func TestSaveAndLoad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &Config{APIKey: "test-key", APIBase: DefaultAPIBase, Output: "json"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	// File must be owner-only (Unix only — Windows has no chmod)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(Path())
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("config file perms = %o, want 0600", perm)
		}
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want test-key", loaded.APIKey)
	}
	if loaded.Output != "json" {
		t.Errorf("Output = %q, want json", loaded.Output)
	}
}

func TestRequireAPIKey(t *testing.T) {
	if err := (&Config{}).RequireAPIKey(); err == nil {
		t.Error("expected error when no API key")
	}
	if err := (&Config{APIKey: "x"}).RequireAPIKey(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Skip("HOME env var not used for config path on Windows")
	}
	want := filepath.Join(home, ".supermodel", "config.yaml")
	if got := Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}
