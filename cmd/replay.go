package cmd

import (
	"fmt"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/history"
	"github.com/BlackMetalz/holyf-network/internal/tui"
	"github.com/spf13/cobra"
)

type replayOptions struct {
	dataDir     string
	startAt     string
	segmentFile string
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
			if strings.TrimSpace(opts.segmentFile) != "" {
				if _, _, err := history.LoadIndexFromFile(dataDir, opts.segmentFile); err != nil {
					return err
				}
			}

			app := tui.NewHistoryApp(
				dataDir,
				opts.startAt,
				opts.segmentFile,
				opts.sensitiveIP,
				resolveBuildVersion(Version),
			)
			return app.Run()
		},
	}

	replayCmd.Flags().StringVar(&opts.dataDir, "data-dir", history.DefaultDataDir(), "Snapshot data directory")
	replayCmd.Flags().StringVar(&opts.startAt, "start-at", tui.HistoryStartLatest, "Starting snapshot position: latest|oldest")
	replayCmd.Flags().StringVar(&opts.segmentFile, "file", "", "Read snapshots from one segment file (e.g. connections-20260304.jsonl)")
	replayCmd.Flags().BoolVar(&opts.sensitiveIP, "sensitive-ip", false, "Hide the first 2 IP octets/groups in replay view")

	return replayCmd
}
