package cmd_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// binaryOnce ensures the binary is built only once across all tests in this
// package, even when tests are run in parallel.
var (
	binaryOnce sync.Once
	binaryPath string
	binaryErr  error
)

// buildBinary compiles the supermodel binary into a temp dir and returns its
// path. Subsequent calls return the cached result. The binary is rebuilt from
// the repo root, so the test reflects the current state of the codebase.
func buildBinary(t *testing.T) string {
	t.Helper()
	binaryOnce.Do(func() {
		// Resolve the module root (one level up from cmd/).
		moduleRoot, err := filepath.Abs(filepath.Join("..", "."))
		if err != nil {
			binaryErr = fmt.Errorf("resolve module root: %w", err)
			return
		}
		// Write to a stable temp path (not t.TempDir()) so the cached
		// binary survives across sub-test invocations within the process.
		dir, err := os.MkdirTemp("", "supermodel-integration-*")
		if err != nil {
			binaryErr = fmt.Errorf("create temp dir: %w", err)
			return
		}
		bin := filepath.Join(dir, "supermodel")
		cmd := exec.Command("go", "build", "-o", bin, ".")
		cmd.Dir = moduleRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			binaryErr = fmt.Errorf("build failed: %v\n%s", err, out)
			return
		}
		binaryPath = bin
	})
	if binaryErr != nil {
		t.Fatalf("buildBinary: %v", binaryErr)
	}
	return binaryPath
}

// runSupermodel executes the supermodel binary with the given arguments in a
// controlled, non-TTY environment.
//
//   - HOME is set to a fresh temp dir (no config file unless the caller writes one).
//   - SUPERMODEL_API_KEY is absent from the environment.
//   - Stdin is /dev/null to guarantee non-interactive mode.
//   - Any additional environment overrides can be supplied via extraEnv
//     ("KEY=value" strings).
//
// It returns (stdout+stderr combined, exit code).
func runSupermodel(t *testing.T, args []string, homeDir string, extraEnv ...string) (string, int) {
	t.Helper()
	bin := buildBinary(t)

	cmd := exec.Command(bin, args...)

	// Build a minimal, controlled environment — start from scratch rather
	// than inheriting the test process's env so that any SUPERMODEL_API_KEY
	// set in the CI environment cannot leak into these tests.
	env := []string{
		"HOME=" + homeDir,
		"PATH=" + os.Getenv("PATH"),
	}
	// Callers can inject overrides (e.g. SUPERMODEL_API_KEY=sm_test...).
	env = append(env, extraEnv...)
	cmd.Env = env

	// Ensure non-TTY stdin.
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer devNull.Close()
	cmd.Stdin = devNull

	out, err := cmd.CombinedOutput()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("exec error (not an ExitError): %v", err)
	}
	return string(out), code
}

// freshHome creates an empty temporary directory to act as the HOME directory
// for a test invocation. It is automatically cleaned up when the test ends.
func freshHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

// writeConfig creates ~/.supermodel/config.yaml with the supplied content.
func writeConfig(t *testing.T, homeDir, content string) {
	t.Helper()
	cfgDir := filepath.Join(homeDir, ".supermodel")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestFirstRun_NoTTY_NoKey verifies that running bare `supermodel` in a
// non-interactive context without an API key exits 1 with a clear "not
// authenticated" message (issue #151 dispatch matrix, bottom-left cell).
func TestFirstRun_NoTTY_NoKey(t *testing.T) {
	home := freshHome(t)
	out, code := runSupermodel(t, nil, home)

	if code != 1 {
		t.Errorf("expected exit code 1, got %d\noutput: %s", code, out)
	}
	if !strings.Contains(out, "not authenticated") {
		t.Errorf("expected output to contain %q\nfull output: %s", "not authenticated", out)
	}
}

// TestFirstRun_NoTTY_WithKey verifies that a non-interactive caller that
// supplies a valid-format API key enters the watch path (not the auth-error
// path). The watch daemon will fail quickly in the test environment (no real
// API, no project cache), but the error must NOT be the "not authenticated"
// message — proving the dispatch selected runWatch, not errNotAuthenticated.
func TestFirstRun_NoTTY_WithKey(t *testing.T) {
	home := freshHome(t)
	// Create a project sub-directory so the binary doesn't refuse to run in
	// HOME (which the watch daemon rejects as a safety measure).
	projectDir := filepath.Join(home, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create project dir: %v", err)
	}

	// Use an env var key. The format must look plausible; any non-empty
	// value causes the dispatch to pick runWatch.
	bin := buildBinary(t)
	cmd := exec.Command(bin, "--dir", projectDir, "--notify-port", "0")
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"SUPERMODEL_API_KEY=sm_integration_test_key",
	}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer devNull.Close()
	cmd.Stdin = devNull

	out, execErr := cmd.CombinedOutput()
	combined := string(out)

	// The process exits non-zero quickly (API call fails, port conflict, etc.)
	// — that's expected. What we must NOT see is the "not authenticated" error.
	if strings.Contains(combined, "not authenticated") {
		t.Errorf("got 'not authenticated' error, but expected the watch path to be entered with a valid API key\noutput: %s", combined)
	}
	// Confirm the binary did exit (one way or another) — don't hang.
	if execErr == nil {
		// Unexpected clean exit — the daemon should not stop immediately.
		// This isn't necessarily a failure, but log it for visibility.
		t.Logf("binary exited 0 unexpectedly (may need a real project dir or API)\noutput: %s", combined)
	}
}

// TestFirstRun_VersionSubcommand_NoKey ensures `supermodel version` exits 0
// and prints a version string without requiring an API key. This exercises
// the noConfigCommands bypass in persistentPreRunE.
func TestFirstRun_VersionSubcommand_NoKey(t *testing.T) {
	home := freshHome(t)
	out, code := runSupermodel(t, []string{"version"}, home)

	if code != 0 {
		t.Errorf("expected exit code 0, got %d\noutput: %s", code, out)
	}
	if !strings.Contains(out, "supermodel") {
		t.Errorf("version output should contain the word 'supermodel'\nfull output: %s", out)
	}
}

// TestFirstRun_CompletionBash_NoKey checks that the nested `completion bash`
// subcommand (a noConfigCommand) exits 0 and emits a bash completion script
// without requiring an API key.
func TestFirstRun_CompletionBash_NoKey(t *testing.T) {
	home := freshHome(t)
	out, code := runSupermodel(t, []string{"completion", "bash"}, home)

	if code != 0 {
		t.Errorf("expected exit code 0, got %d\noutput: %s", code, out)
	}
	// Bash completion scripts always begin with a comment or a function
	// declaration. Both contain "supermodel" as the command name.
	if !strings.Contains(out, "supermodel") {
		t.Errorf("completion output should reference 'supermodel'\nfull output: %s", out)
	}
}

// TestFirstRun_Analyze_NoKey verifies that `supermodel analyze` without an
// API key exits 1 with an actionable "run 'supermodel setup'" message (via
// persistentPreRunE), NOT the generic "not authenticated" error reserved for
// the bare root command.
func TestFirstRun_Analyze_NoKey(t *testing.T) {
	home := freshHome(t)
	out, code := runSupermodel(t, []string{"analyze"}, home)

	if code != 1 {
		t.Errorf("expected exit code 1, got %d\noutput: %s", code, out)
	}
	// persistentPreRunE should fire before analyze's own RequireAPIKey(),
	// producing the "setup" guidance rather than the root-command error.
	if !strings.Contains(out, "supermodel setup") {
		t.Errorf("expected output to contain %q\nfull output: %s", "supermodel setup", out)
	}
	// Specifically must NOT surface the bare-root "not authenticated" error.
	if strings.Contains(out, "not authenticated") {
		t.Errorf("got 'not authenticated' (root-command error) but expected 'supermodel setup' guidance\nfull output: %s", out)
	}
}

// TestFirstRun_EmptyAPIKeyInConfig verifies that a config file with an
// explicit empty api_key field is treated the same as having no key at all —
// i.e. the binary exits 1 with a "not authenticated" error in non-TTY mode.
// This is an edge case that could silently break if config parsing changes.
func TestFirstRun_EmptyAPIKeyInConfig(t *testing.T) {
	home := freshHome(t)
	writeConfig(t, home, "api_key: \"\"\n")

	out, code := runSupermodel(t, nil, home)

	if code != 1 {
		t.Errorf("expected exit code 1 with empty api_key in config, got %d\noutput: %s", code, out)
	}
	if !strings.Contains(out, "not authenticated") {
		t.Errorf("expected 'not authenticated' error for empty api_key config\nfull output: %s", out)
	}
}
