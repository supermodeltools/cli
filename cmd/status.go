package cmd

import (
	"github.com/spf13/cobra"
	"github.com/supermodeltools/cli/internal/status"
)

func init() {
	var opts status.Options

	c := &cobra.Command{
		Use:   "status",
		Short: "Show authentication and cache status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return status.Run(cmd.Context(), opts)
		},
	}

	c.Flags().StringVarP(&opts.Output, "output", "o", "", "output format: human|json")

	rootCmd.AddCommand(c)
}
