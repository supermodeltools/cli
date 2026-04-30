package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestInstallScript(t *testing.T) {
	data, err := os.ReadFile("../install.sh")
	if err != nil {
		t.Fatalf("could not read install.sh: %v", err)
	}
	content := string(data)

	t.Run("does not run setup wizard at install time", func(t *testing.T) {
		// The script uses $BINARY variable, so match the literal subcommand argument
		if strings.Contains(content, `" setup`) || strings.Contains(content, "supermodel setup") {
			t.Error("install.sh must not run the setup subcommand at install time — the wizard now auto-launches from bare 'supermodel' in a project directory (PR #152)")
		}
	})

	t.Run("includes getting-started message", func(t *testing.T) {
		lower := strings.ToLower(content)
		hasRunSupermodel := strings.Contains(lower, "run 'supermodel'") || strings.Contains(lower, "run \"supermodel\"")
		hasGetStarted := strings.Contains(lower, "get started")
		if !hasRunSupermodel && !hasGetStarted {
			t.Error("install.sh must include a getting-started message directing users to run 'supermodel' in their project directory")
		}
	})
}
