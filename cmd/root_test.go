package cmd

import (
	"os"
	"testing"
)

// TestStdinIsTerminalUsesDevTtyFallback verifies that stdinIsTerminal falls
// back to opening /dev/tty when term.IsTerminal returns false (e.g. in CI or
// MinTTY/Git Bash on Windows). The fix is tracked in issue #154.
func TestStdinIsTerminalUsesDevTtyFallback(t *testing.T) {
	// Save and restore the real openDevTty hook.
	orig := openDevTty
	t.Cleanup(func() { openDevTty = orig })

	called := false
	openDevTty = func() (*os.File, error) {
		called = true
		// Return a real, harmless file so the caller can call f.Close().
		return os.Open(os.DevNull)
	}

	// In CI stdin is not a TTY, so term.IsTerminal returns false.
	// The fixed stdinIsTerminal must call openDevTty as a fallback and
	// return true because our mock succeeds.
	got := stdinIsTerminal()

	if !called {
		t.Error("stdinIsTerminal did not call openDevTty — /dev/tty fallback is missing (fix #154)")
	}
	// If openDevTty was called and succeeded, the result must be true.
	if called && !got {
		t.Error("openDevTty returned a file but stdinIsTerminal returned false — fix #154")
	}
}

func TestPickRootAction(t *testing.T) {
	cases := []struct {
		name        string
		hasAPIKey   bool
		interactive bool
		want        rootAction
	}{
		{"key + tty starts watch", true, true, runWatch},
		{"key + non-tty starts watch (CI happy path)", true, false, runWatch},
		{"no key + tty drops into setup wizard", false, true, runSetup},
		{"no key + non-tty errors instead of hanging", false, false, errNotAuthenticated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pickRootAction(tc.hasAPIKey, tc.interactive)
			if got != tc.want {
				t.Errorf("pickRootAction(hasAPIKey=%v, interactive=%v) = %v, want %v",
					tc.hasAPIKey, tc.interactive, got, tc.want)
			}
		})
	}
}
