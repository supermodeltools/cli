package cmd

import (
	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/files"
)

func init() {
	var port int

	c := &cobra.Command{
		Use:   "hook",
		Short: "Forward Claude Code file-change events to the watch daemon",
		Long:  `Reads a Claude Code PostToolUse JSON payload from stdin and forwards the file path to the running watch daemon via UDP. Install as a PostToolUse hook in .claude/settings.json.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return files.Hook(port)
		},
	}

	c.Flags().IntVar(&port, "port", 7734, "UDP port of the watch daemon")
	rootCmd.AddCommand(c)
}
