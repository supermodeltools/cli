package cmd

import (
	"os"
	"strings"
	"testing"
)

// TestNpmPackageHasPostinstallMessage verifies that npm/install.js prints a
// getting-started message after the binary is installed, directing users to
// run 'supermodel' inside their project directory.
//
// Resolves: https://github.com/supermodeltools/cli/issues/159
func TestNpmPackageHasPostinstallMessage(t *testing.T) {
	data, err := os.ReadFile("../npm/install.js")
	if err != nil {
		t.Fatalf("could not read npm/install.js: %v", err)
	}
	content := string(data)

	t.Run("does not run setup wizard at install time", func(t *testing.T) {
		if strings.Contains(content, `" setup`) || strings.Contains(content, "supermodel setup") {
			t.Error("npm/install.js must not run the setup subcommand at install time")
		}
	})

	t.Run("includes getting-started message", func(t *testing.T) {
		lower := strings.ToLower(content)
		hasRunSupermodel := strings.Contains(lower, "run 'supermodel'") || strings.Contains(lower, `run "supermodel"`)
		hasGetStarted := strings.Contains(lower, "get started")
		if !hasRunSupermodel && !hasGetStarted {
			t.Error("npm/install.js must print a getting-started message directing users to run 'supermodel' in their project directory")
		}
	})
}
