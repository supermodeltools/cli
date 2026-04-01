package cmd

import (
	"github.com/spf13/cobra"
	"github.com/supermodeltools/cli/internal/auth"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "login",
		Short: "Authenticate with your Supermodel account",
		Long: `Prompts for an API key and saves it to ~/.supermodel/config.yaml.

Get a key at https://supermodeltools.com/dashboard`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return auth.Login(cmd.Context())
		},
	})
}
