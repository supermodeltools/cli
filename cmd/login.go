package cmd

import (
	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/auth"
)

func init() {
	var token string

	c := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with your Supermodel account",
		Long: `Opens your browser to create an API key and automatically saves it.

For CI or headless environments, pass the key directly:
  supermodel login --token smsk_live_...`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("token") {
				return auth.LoginWithToken(token)
			}
			return auth.Login(cmd.Context())
		},
	}

	c.Flags().StringVar(&token, "token", "", "API key for non-interactive login (CI)")
	rootCmd.AddCommand(c)
}
