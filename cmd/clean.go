package cmd

import (
	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/files"
)

func init() {
	var dryRun bool

	c := &cobra.Command{
		Use:   "clean [path]",
		Short: "Remove all .graph.* files from the repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return files.Clean(cmd.Context(), cfg, dir, dryRun)
		},
	}

	c.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be removed without removing")
	rootCmd.AddCommand(c)
}
