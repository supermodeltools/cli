package cmd

import (
	"github.com/spf13/cobra"
	"github.com/supermodeltools/cli/internal/archdocs"
	"github.com/supermodeltools/cli/internal/config"
)

func init() {
	var opts archdocs.Options

	c := &cobra.Command{
		Use:   "arch-docs [path]",
		Short: "Generate static architecture documentation for a repository",
		Long: `Generate a static HTML site documenting the architecture of a codebase.

The command uploads the repository to the Supermodel API, converts the
returned code graph to markdown, and builds a browsable static site with
search, dependency graphs, taxonomy navigation, and SEO metadata.

The output directory can be served locally or deployed to any static host
(GitHub Pages, Vercel, Netlify, Cloudflare Pages, etc.).

Examples:
  supermodel arch-docs
  supermodel arch-docs ./my-project --output ./docs-site
  supermodel arch-docs --repo owner/repo --base-url https://owner.github.io/repo
  supermodel arch-docs --site-name "My App Docs" --output /var/www/html`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireAPIKey(); err != nil {
				return err
			}
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return archdocs.Run(cmd.Context(), cfg, dir, opts)
		},
	}

	c.Flags().StringVar(&opts.SiteName, "site-name", "", "display title for the generated site (default: \"<repo> Architecture Docs\")")
	c.Flags().StringVar(&opts.BaseURL, "base-url", "", "canonical base URL where the site will be hosted (default: https://example.com)")
	c.Flags().StringVar(&opts.Repo, "repo", "", "GitHub repo slug owner/repo used to build source links")
	c.Flags().StringVarP(&opts.Output, "output", "o", "", "output directory for the generated site (default: ./arch-docs-output)")
	c.Flags().StringVar(&opts.TemplatesDir, "templates-dir", "", "override bundled HTML/CSS/JS templates with a custom directory")
	c.Flags().IntVar(&opts.MaxSourceFiles, "max-source-files", 3000, "maximum source files to include in analysis (0 = unlimited)")
	c.Flags().IntVar(&opts.MaxEntities, "max-entities", 12000, "maximum entity pages to generate (0 = unlimited)")
	c.Flags().BoolVar(&opts.Force, "force", false, "bypass cache and re-upload even if a cached result exists")

	rootCmd.AddCommand(c)
}
