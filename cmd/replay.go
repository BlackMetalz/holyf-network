package cmd

import (
	"fmt"

	"github.com/BlackMetalz/holyf-network/internal/history"
	"github.com/BlackMetalz/holyf-network/internal/tui"
	"github.com/spf13/cobra"
)

type replayOptions struct {
	dataDir     string
	startAt     string
	sensitiveIP bool
}

func newReplayCmd() *cobra.Command {
	opts := replayOptions{}

	replayCmd := &cobra.Command{
		Use:   "replay",
		Short: "Open read-only replay UI for connection snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.startAt != tui.HistoryStartLatest && opts.startAt != tui.HistoryStartOldest {
				return fmt.Errorf("start-at must be '%s' or '%s'", tui.HistoryStartLatest, tui.HistoryStartOldest)
			}

			dataDir := history.ExpandPath(opts.dataDir)
			if dataDir == "" {
				dataDir = history.DefaultDataDir()
			}
			if dataDir == "" {
				return fmt.Errorf("cannot determine default data dir")
			}

			app := tui.NewHistoryApp(
				dataDir,
				opts.startAt,
				opts.sensitiveIP,
				resolveBuildVersion(Version),
			)
			return app.Run()
		},
	}

	replayCmd.Flags().StringVar(&opts.dataDir, "data-dir", history.DefaultDataDir(), "Snapshot data directory")
	replayCmd.Flags().StringVar(&opts.startAt, "start-at", tui.HistoryStartLatest, "Starting snapshot position: latest|oldest")
	replayCmd.Flags().BoolVar(&opts.sensitiveIP, "sensitive-ip", false, "Hide the first 2 IP octets/groups in replay view")

	return replayCmd
}
