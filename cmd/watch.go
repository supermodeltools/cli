package cmd

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/files"
)

func init() {
	var opts files.WatchOptions

	c := &cobra.Command{
		Use:   "watch [path]",
		Short: "Generate graph files on startup, then keep them updated as you code",
		Long: `Runs a full generate on startup (using cached graph if available), then
enters daemon mode. Listens for file-change notifications from the
'supermodel hook' command and incrementally re-renders affected files.

Press Ctrl+C to stop and remove graph files.`,
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
			return files.Watch(cmd.Context(), cfg, dir, opts)
		},
	}

	c.Flags().StringVar(&opts.CacheFile, "cache-file", "", "override cache file path")
	c.Flags().DurationVar(&opts.Debounce, "debounce", 2*time.Second, "debounce duration before processing changes")
	c.Flags().IntVar(&opts.NotifyPort, "notify-port", 7734, "UDP port for hook notifications")
	c.Flags().BoolVar(&opts.FSWatch, "fs-watch", false, "enable git-poll fallback")
	c.Flags().DurationVar(&opts.PollInterval, "poll-interval", 3*time.Second, "git poll interval when --fs-watch is enabled")

	rootCmd.AddCommand(c)
}
