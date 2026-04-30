package cmd

import (
	"os"
	"regexp"
	"testing"
)

func TestInstallScript(t *testing.T) {
	data, err := os.ReadFile("../install.sh")
	if err != nil {
		t.Fatalf("could not read install.sh: %v", err)
	}
	content := string(data)

	t.Run("does not run setup wizard at install time", func(t *testing.T) {
		// Match command execution shape only — not arbitrary text mentions.
		setupInvocation := regexp.MustCompile(`(?m)^\s*"\$INSTALL_DIR/\$BINARY"\s+setup\b`)
		if setupInvocation.MatchString(content) {
			t.Error("install.sh must not run the setup subcommand at install time — the wizard now auto-launches from bare 'supermodel' in a project directory (PR #152)")
		}
	})

	t.Run("includes getting-started message", func(t *testing.T) {
		hasRunSupermodel := regexp.MustCompile(`(?i)\brun\s+['"]?supermodel['"]?\b`).MatchString(content)
		hasProjectContext := regexp.MustCompile(`(?i)\b(in|inside)\s+your\s+project\b|\bproject\s+directory\b`).MatchString(content)
		if !hasRunSupermodel || !hasProjectContext {
			t.Error("install.sh must include a getting-started message directing users to run 'supermodel' in their project directory")
		}
	})
}
