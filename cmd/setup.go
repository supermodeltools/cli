package cmd

import (
	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/setup"
)

func init() {
	c := &cobra.Command{
		Use:   "setup",
		Short: "Interactive setup wizard",
		Long:  `Walks through authentication, repository selection, file mode, and Claude Code hook installation.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return setup.Run(cmd.Context(), cfg)
		},
	}
	rootCmd.AddCommand(c)
}
