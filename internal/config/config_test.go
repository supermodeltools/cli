package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUPERMODEL_API_KEY", "")
	t.Setenv("SUPERMODEL_API_BASE", "")
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
	t.Setenv("SUPERMODEL_API_KEY", "")
	t.Setenv("SUPERMODEL_API_BASE", "")
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

// ── ShardsEnabled ─────────────────────────────────────────────────────────────

func TestShardsEnabled_DefaultTrue(t *testing.T) {
	cfg := &Config{}
	if !cfg.ShardsEnabled() {
		t.Error("ShardsEnabled() with nil Shards should default to true")
	}
}

func TestShardsEnabled_ExplicitFalse(t *testing.T) {
	f := false
	cfg := &Config{Shards: &f}
	if cfg.ShardsEnabled() {
		t.Error("ShardsEnabled() with Shards=false should return false")
	}
}

func TestShardsEnabled_ExplicitTrue(t *testing.T) {
	tr := true
	cfg := &Config{Shards: &tr}
	if !cfg.ShardsEnabled() {
		t.Error("ShardsEnabled() with Shards=true should return true")
	}
}

// ── applyEnv ──────────────────────────────────────────────────────────────────

func TestApplyEnv_APIKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUPERMODEL_API_KEY", "env-key-123")
	t.Setenv("SUPERMODEL_API_BASE", "")
	t.Setenv("SUPERMODEL_SHARDS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "env-key-123" {
		t.Errorf("SUPERMODEL_API_KEY env override: got %q", cfg.APIKey)
	}
}

func TestApplyEnv_APIBase(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUPERMODEL_API_KEY", "")
	t.Setenv("SUPERMODEL_API_BASE", "https://custom.api.com")
	t.Setenv("SUPERMODEL_SHARDS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIBase != "https://custom.api.com" {
		t.Errorf("SUPERMODEL_API_BASE env override: got %q", cfg.APIBase)
	}
}

func TestApplyEnv_ShardsDisabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUPERMODEL_API_KEY", "")
	t.Setenv("SUPERMODEL_API_BASE", "")
	t.Setenv("SUPERMODEL_SHARDS", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ShardsEnabled() {
		t.Error("SUPERMODEL_SHARDS=false should disable shards")
	}
}

// ── applyDefaults ─────────────────────────────────────────────────────────────

func TestApplyDefaults_FilledFromFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SUPERMODEL_API_KEY", "")
	t.Setenv("SUPERMODEL_API_BASE", "")
	t.Setenv("SUPERMODEL_SHARDS", "")

	// Write a config that has api_key but no api_base or output
	cfgFile := filepath.Join(home, ".supermodel", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgFile, []byte("api_key: loaded-key\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "loaded-key" {
		t.Errorf("loaded api_key: got %q", cfg.APIKey)
	}
	if cfg.APIBase != DefaultAPIBase {
		t.Errorf("default api_base: got %q", cfg.APIBase)
	}
	if cfg.Output != "human" {
		t.Errorf("default output: got %q", cfg.Output)
	}
}
