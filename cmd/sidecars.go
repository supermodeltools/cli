package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/sidecars"
)

func init() {
	sidecarsCmd := &cobra.Command{
		Use:   "sidecars",
		Short: "Generate and manage AI-readable code graph sidecars",
		Long: `Sidecars are lightweight .graph.* files placed next to each source file,
containing dependency, call graph, and blast-radius data extracted by the
Supermodel API. AI coding agents (Claude Code, Cursor, etc.) read these files
automatically to understand cross-file relationships.

Run 'supermodel sidecars generate' to create sidecars for your repo.
Run 'supermodel sidecars watch' to keep them updated as you code.`,
	}

	// --- generate ---
	{
		var opts sidecars.GenerateOptions

		c := &cobra.Command{
			Use:   "generate [path]",
			Short: "Zip, upload, and render sidecars for the repository",
			Long: `Archives the repository, uploads it to the Supermodel API, builds a local
graph cache, and writes .graph.* sidecar files next to every source file.

Subsequent runs reuse the local cache automatically unless --force is given.`,
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
				return sidecars.Generate(cmd.Context(), cfg, dir, opts)
			},
		}

		c.Flags().BoolVar(&opts.Force, "force", false, "re-fetch from API even if a cached graph exists")
		c.Flags().BoolVar(&opts.DryRun, "dry-run", false, "show what would be written without writing")
		c.Flags().StringVar(&opts.CacheFile, "cache-file", "", "override cache file location (default: <repo>/.supermodel/graph.json)")

		sidecarsCmd.AddCommand(c)
	}

	// --- watch ---
	{
		var opts sidecars.WatchOptions

		c := &cobra.Command{
			Use:   "watch [path]",
			Short: "Generate sidecars on startup, then watch for file changes",
			Long: `Runs a full generate on startup (using cached graph if available), then
enters daemon mode. The daemon listens for UDP notifications from the
'supermodel sidecars hook' command (or git-poll when --fs-watch is set)
and incrementally re-renders affected sidecars.

Press Ctrl+C to stop.`,
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
				return sidecars.Watch(cmd.Context(), cfg, dir, opts)
			},
		}

		c.Flags().StringVar(&opts.CacheFile, "cache-file", "", "override cache file location")
		c.Flags().DurationVar(&opts.Debounce, "debounce", 2*time.Second, "debounce duration before processing changes")
		c.Flags().IntVar(&opts.NotifyPort, "notify-port", 7734, "UDP port to listen for hook notifications")
		c.Flags().BoolVar(&opts.FSWatch, "fs-watch", false, "enable git-poll fallback for environments without hooks")
		c.Flags().DurationVar(&opts.PollInterval, "poll-interval", 3*time.Second, "git poll interval when --fs-watch is enabled")

		sidecarsCmd.AddCommand(c)
	}

	// --- clean ---
	{
		var opts sidecars.CleanOptions

		c := &cobra.Command{
			Use:   "clean [path]",
			Short: "Remove all .graph.* sidecar files",
			Long:  `Walks the directory tree and removes every generated .graph.* sidecar file.`,
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				dir := "."
				if len(args) > 0 {
					dir = args[0]
				}
				return sidecars.Clean(dir, opts)
			},
		}

		c.Flags().BoolVar(&opts.DryRun, "dry-run", false, "show what would be removed without removing")

		sidecarsCmd.AddCommand(c)
	}

	// --- hook ---
	{
		var opts sidecars.HookOptions

		c := &cobra.Command{
			Use:   "hook",
			Short: "Forward Claude Code PostToolUse events to the watch daemon",
			Long: `Reads a Claude Code PostToolUse JSON event from stdin and sends a UDP
notification to the watch daemon when a source file is written or edited.

Configure this as a Claude Code hook in .claude/settings.json:

  {
    "hooks": {
      "PostToolUse": [
        {
          "matcher": "Write|Edit|MultiEdit",
          "hooks": [{"type": "command", "command": "supermodel sidecars hook"}]
        }
      ]
    }
  }`,
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return sidecars.Hook(opts)
			},
		}

		c.Flags().IntVar(&opts.Port, "port", 7734, "UDP port of the watch daemon")

		sidecarsCmd.AddCommand(c)
	}

	// --- render ---
	{
		var opts sidecars.RenderOptions

		c := &cobra.Command{
			Use:   "render [path]",
			Short: "Render sidecars from the existing local cache (offline)",
			Long: `Re-renders sidecars using the locally cached graph without fetching from
the API. Useful after a git pull or branch switch when the graph is still valid.`,
			Args: cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				dir := "."
				if len(args) > 0 {
					dir = args[0]
				}
				return sidecars.Render(dir, opts)
			},
		}

		c.Flags().StringVar(&opts.CacheFile, "cache-file", "", "override cache file location")
		c.Flags().BoolVar(&opts.DryRun, "dry-run", false, "show what would be written without writing")

		sidecarsCmd.AddCommand(c)
	}

	// --- setup (stub) ---
	{
		c := &cobra.Command{
			Use:   "setup",
			Short: "Show setup instructions",
			Long:  `Prints a quick-start guide for configuring Supermodel sidecars.`,
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("Supermodel Sidecars — Quick Setup")
				fmt.Println()
				fmt.Println("1. Authenticate:")
				fmt.Println("   supermodel login")
				fmt.Println()
				fmt.Println("2. Generate sidecars for your repo:")
				fmt.Println("   supermodel sidecars generate")
				fmt.Println()
				fmt.Println("3. Keep sidecars updated while coding:")
				fmt.Println("   supermodel sidecars watch")
				fmt.Println()
				fmt.Println("4. (Optional) Add the hook to .claude/settings.json so sidecars")
				fmt.Println("   update automatically when Claude Code writes files:")
				fmt.Println(`   {`)
				fmt.Println(`     "hooks": {`)
				fmt.Println(`       "PostToolUse": [{`)
				fmt.Println(`         "matcher": "Write|Edit|MultiEdit",`)
				fmt.Println(`         "hooks": [{"type": "command", "command": "supermodel sidecars hook"}]`)
				fmt.Println(`       }]`)
				fmt.Println(`     }`)
				fmt.Println(`   }`)
				return nil
			},
		}

		sidecarsCmd.AddCommand(c)
	}

	rootCmd.AddCommand(sidecarsCmd)
}
