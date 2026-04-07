package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/config"
)

// noConfigCommands are subcommands that work without a config file.
// Includes Cobra's internal shell-completion helpers to avoid crashing them.
var noConfigCommands = map[string]bool{
	"setup":              true,
	"login":              true,
	"logout":             true,
	"version":            true,
	"help":               true,
	"completion":         true,
	"__complete":         true,
	"__completeNoDesc":   true,
}

var rootCmd = &cobra.Command{
	Use:   "supermodel",
	Short: "Give your AI coding agent a map of your codebase",
	Long: `Supermodel connects AI coding agents to the Supermodel API,
providing call graphs, dead code detection, and blast radius analysis.

See https://supermodeltools.com for documentation.`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Walk up to the root command name to get the subcommand.
		name := cmd.Name()
		if noConfigCommands[name] {
			return nil
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
		if cfg.APIKey == "" {
			fmt.Fprintln(os.Stderr, "Run 'supermodel setup' to get started.")
			os.Exit(1)
		}
		return nil
	},
}

// Execute is the entry point called by main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
