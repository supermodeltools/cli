package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/supermodeltools/cli/internal/config"
)

func TestLoginWithToken(t *testing.T) {
	// Point config to a temp dir so we don't touch real config.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("SUPERMODEL_API_KEY", "")

	if err := LoginWithToken("smsk_live_test123"); err != nil {
		t.Fatalf("LoginWithToken: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "smsk_live_test123" {
		t.Errorf("expected key smsk_live_test123, got %q", cfg.APIKey)
	}
}

func TestLoginWithToken_Empty(t *testing.T) {
	if err := LoginWithToken(""); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestLoginWithToken_Whitespace(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("SUPERMODEL_API_KEY", "")

	if err := LoginWithToken("  smsk_live_padded  "); err != nil {
		t.Fatalf("LoginWithToken: %v", err)
	}

	cfg, _ := config.Load()
	if cfg.APIKey != "smsk_live_padded" {
		t.Errorf("expected trimmed key, got %q", cfg.APIKey)
	}
}

func TestCallbackServer(t *testing.T) {
	// Simulate the browser callback flow by starting the localhost server
	// and hitting the callback endpoint directly.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	state := "test-state-123"

	keyCh := make(chan string, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "bad state", http.StatusBadRequest)
			return
		}
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "missing key", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		keyCh <- key
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(listener)
	defer srv.Close()

	// Simulate the dashboard redirect.
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?key=smsk_live_from_browser&state=%s", port, state))
	if err != nil {
		t.Fatalf("callback request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	select {
	case key := <-keyCh:
		if key != "smsk_live_from_browser" {
			t.Errorf("expected smsk_live_from_browser, got %q", key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for key")
	}
}

func TestCallbackServer_BadState(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "correct-state" {
			http.Error(w, "bad state", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/callback?key=smsk_live_x&state=wrong-state")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for bad state, got %d", resp.StatusCode)
	}
}

func TestCallbackServer_MissingKey(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "s" {
			http.Error(w, "bad state", http.StatusBadRequest)
			return
		}
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "missing key", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/callback?state=s")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing key, got %d", resp.StatusCode)
	}
}

func TestRandomState(t *testing.T) {
	s1 := randomState()
	s2 := randomState()
	if s1 == s2 {
		t.Error("randomState should produce unique values")
	}
	if len(s1) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("expected 32 char hex string, got %d chars", len(s1))
	}
}

func TestLogout(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("SUPERMODEL_API_KEY", "")

	// Set up a config with a key.
	cfg := &config.Config{APIKey: "smsk_live_toremove", APIBase: config.DefaultAPIBase, Output: "human"}
	os.MkdirAll(filepath.Join(tmp, ".supermodel"), 0o700)
	cfg.Save()

	if err := Logout(context.Background()); err != nil {
		t.Fatal(err)
	}

	cfg, _ = config.Load()
	if cfg.APIKey != "" {
		t.Errorf("expected empty key after logout, got %q", cfg.APIKey)
	}
}

func TestLogout_AlreadyLoggedOut(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("SUPERMODEL_API_KEY", "")

	// No API key set.
	if err := Logout(context.Background()); err != nil {
		t.Fatalf("Logout when already logged out: %v", err)
	}
}

func TestLoginWithToken_ConfigLoadError(t *testing.T) {
	// Make config.yaml a directory → os.ReadFile returns a non-NotExist error.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("SUPERMODEL_API_KEY", "")
	cfgDir := filepath.Join(tmp, ".supermodel")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Create a directory where config.yaml would be → ReadFile fails.
	if err := os.Mkdir(filepath.Join(cfgDir, "config.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := LoginWithToken("smsk_live_test"); err == nil {
		t.Error("expected error when config.Load fails")
	}
}

func TestLoginWithToken_SaveError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("SUPERMODEL_API_KEY", "")
	cfgDir := filepath.Join(tmp, ".supermodel")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(cfgDir, 0o755) }) //nolint:errcheck
	if err := LoginWithToken("smsk_live_test"); err == nil {
		t.Error("expected error when cfg.Save fails")
	}
}

func TestLogout_ConfigLoadError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("SUPERMODEL_API_KEY", "")
	cfgDir := filepath.Join(tmp, ".supermodel")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(cfgDir, "config.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Logout(context.Background()); err == nil {
		t.Error("expected error when config.Load fails")
	}
}

func TestLogout_SaveError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("SUPERMODEL_API_KEY", "")
	// Pre-create a config with a key so Logout proceeds to Save.
	cfgDir := filepath.Join(tmp, ".supermodel")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{APIKey: "smsk_live_toremove"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(cfgDir, 0o755) }) //nolint:errcheck
	if err := Logout(context.Background()); err == nil {
		t.Error("expected error when cfg.Save fails during logout")
	}
}
