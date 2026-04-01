package cmd

import (
	"github.com/spf13/cobra"
	"github.com/supermodeltools/cli/internal/auth"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return auth.Logout(cmd.Context())
		},
	})
}
