package cmd

import (
	"github.com/spf13/cobra"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/focus"
)

func init() {
	var opts focus.Options

	c := &cobra.Command{
		Use:     "focus <file>",
		Aliases: []string{"ctx", "context"},
		Short:   "Extract a token-efficient graph slice for a file",
		Long: `Extracts the minimal graph context relevant to the given file:
direct imports, functions defined, callers, and type declarations.

Output is structured markdown for direct injection into LLM context windows,
keeping token usage minimal while preserving semantic relevance.

Use --output json for structured consumption by tools.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireAPIKey(); err != nil {
				return err
			}
			return focus.Run(cmd.Context(), cfg, ".", args[0], opts)
		},
	}

	c.Flags().BoolVar(&opts.Force, "force", false, "re-analyze even if cache is fresh")
	c.Flags().IntVar(&opts.Depth, "depth", 1, "import traversal depth")
	c.Flags().BoolVar(&opts.IncludeTypes, "types", false, "include type/class declarations")
	c.Flags().StringVarP(&opts.Output, "output", "o", "markdown", "output format: markdown|json")

	rootCmd.AddCommand(c)
}
