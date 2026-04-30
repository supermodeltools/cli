package cmd

import (
	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/shards"
)

func init() {
	var port int

	c := &cobra.Command{
		Use:   "hook",
		Short: "Forward Claude Code file-change events to the Supermodel daemon",
		Long:  `Reads a Claude Code PostToolUse JSON payload from stdin and forwards the file path to the running Supermodel daemon via UDP. Install as a PostToolUse hook in .claude/settings.json.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return shards.Hook(port)
		},
	}

	c.Flags().IntVar(&port, "port", 7734, "UDP port of the Supermodel daemon")
	rootCmd.AddCommand(c)
}
