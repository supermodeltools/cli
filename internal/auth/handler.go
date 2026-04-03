package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

const dashboardBase = "https://dashboard.supermodeltools.com"

// Login runs the browser-based login flow. Opens the dashboard to create an
// API key, receives it via localhost callback, validates, and saves it.
// Falls back to manual paste if the browser flow fails.
func Login(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Start localhost server on a random port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not start local server — falling back to manual login.")
		return loginManual(cfg)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	state := randomState()

	// Channel to receive the API key from the callback.
	keyCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)
			return
		}
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "Missing key", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body style="font-family:system-ui;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#0a0a0a;color:#fff"><div style="text-align:center"><h2>&#10003; Authenticated</h2><p style="color:#888">You can close this tab and return to your terminal.</p></div></body></html>`)
		keyCh <- key
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second} //nolint:gosec // localhost-only server
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer srv.Close()

	// Build the dashboard URL and open the browser.
	authURL := fmt.Sprintf("%s/cli-auth?port=%d&state=%s", dashboardBase, port, state)
	fmt.Println("Opening browser to log in...")
	fmt.Printf("If the browser doesn't open, visit:\n  %s\n\n", authURL)

	if err := openBrowser(authURL); err != nil {
		fmt.Fprintln(os.Stderr, "Could not open browser — falling back to manual login.")
		srv.Close()
		return loginManual(cfg)
	}

	// Wait for callback or timeout.
	fmt.Print("Waiting for authentication...")
	select {
	case key := <-keyCh:
		fmt.Println()
		cfg.APIKey = strings.TrimSpace(key)
		if err := cfg.Save(); err != nil {
			return err
		}
		ui.Success("Authenticated — key saved to %s", config.Path())
		return nil
	case err := <-errCh:
		fmt.Println()
		return fmt.Errorf("local server error: %w", err)
	case <-time.After(5 * time.Minute):
		fmt.Println()
		fmt.Fprintln(os.Stderr, "Timed out waiting for browser login — falling back to manual login.")
		srv.Close()
		return loginManual(cfg)
	case <-ctx.Done():
		fmt.Println()
		return ctx.Err()
	}
}

// LoginWithToken saves an API key directly (for CI/headless use).
func LoginWithToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("API key cannot be empty")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.APIKey = token
	if err := cfg.Save(); err != nil {
		return err
	}
	ui.Success("Authenticated — key saved to %s", config.Path())
	return nil
}

// Logout removes the API key from the config file.
func Logout(_ context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.APIKey == "" {
		fmt.Println("Already logged out.")
		return nil
	}
	cfg.APIKey = ""
	if err := cfg.Save(); err != nil {
		return err
	}
	ui.Success("Logged out — API key removed from %s", config.Path())
	return nil
}

// loginManual is the fallback paste-based login.
func loginManual(cfg *config.Config) error {
	fmt.Println("Get your API key at https://dashboard.supermodeltools.com/api-keys")
	fmt.Print("Paste your API key: ")

	key, err := readSecret()
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	cfg.APIKey = key
	if err := cfg.Save(); err != nil {
		return err
	}
	ui.Success("Authenticated — key saved to %s", config.Path())
	return nil
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func randomState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// readSecret reads a line from stdin, suppressing echo when a TTY is attached.
func readSecret() (string, error) {
	fd := int(syscall.Stdin) //nolint:unconvert // syscall.Stdin is uintptr on Windows
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Println()
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no input received")
}
