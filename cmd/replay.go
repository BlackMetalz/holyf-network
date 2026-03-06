package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
	"github.com/BlackMetalz/holyf-network/internal/tui"
	"github.com/spf13/cobra"
)

type replayOptions struct {
	dataDir     string
	startAt     string
	segmentFile string
	sensitiveIP bool
	begin       string
	end         string
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

			beginAt, err := resolveReplayBoundTime(opts.begin, time.Now(), opts.segmentFile)
			if err != nil {
				return fmt.Errorf("invalid --begin: %w (use YYYY-MM-DD HH:MM[:SS], HH:MM[:SS], yesterday HH:MM[:SS], or RFC3339)", err)
			}
			endAt, err := resolveReplayBoundTime(opts.end, time.Now(), opts.segmentFile)
			if err != nil {
				return fmt.Errorf("invalid --end: %w (use YYYY-MM-DD HH:MM[:SS], HH:MM[:SS], yesterday HH:MM[:SS], or RFC3339)", err)
			}
			beginAt, endAt = completeReplayDayWindow(beginAt, endAt)
			if beginAt != nil && endAt != nil && beginAt.After(*endAt) {
				return fmt.Errorf("invalid time window: --begin must be <= --end")
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
				beginAt,
				endAt,
			)
			return app.Run()
		},
	}

	replayCmd.Flags().StringVar(&opts.dataDir, "data-dir", history.DefaultDataDir(), "Snapshot data directory")
	replayCmd.Flags().StringVar(&opts.startAt, "start-at", tui.HistoryStartLatest, "Starting snapshot position: latest|oldest")
	replayCmd.Flags().StringVarP(&opts.segmentFile, "file", "f", "", "Read snapshots from one segment file (e.g. connections-20260304.jsonl)")
	replayCmd.Flags().BoolVar(&opts.sensitiveIP, "sensitive-ip", false, "Hide the first 2 IP octets/groups in replay view")
	replayCmd.Flags().StringVarP(&opts.begin, "begin", "b", "", "Replay begin time (inclusive): YYYY-MM-DD HH:MM[:SS], HH:MM[:SS], yesterday HH:MM[:SS], or RFC3339")
	replayCmd.Flags().StringVarP(&opts.end, "end", "e", "", "Replay end time (inclusive): YYYY-MM-DD HH:MM[:SS], HH:MM[:SS], yesterday HH:MM[:SS], or RFC3339")

	return replayCmd
}

func resolveReplayBoundTime(raw string, now time.Time, segmentFile string) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	parseNow := now
	if isClockOnlyReplayInput(trimmed) {
		if segmentDate, ok := history.ParseSegmentDate(filepath.Base(segmentFile)); ok {
			parseNow = time.Date(
				segmentDate.Year(),
				segmentDate.Month(),
				segmentDate.Day(),
				now.Hour(),
				now.Minute(),
				now.Second(),
				now.Nanosecond(),
				segmentDate.Location(),
			)
		}
	}

	parsed, err := history.ParseReplayTime(trimmed, parseNow)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func isClockOnlyReplayInput(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	for _, layout := range []string{"15:04:05", "15:04"} {
		if _, err := time.Parse(layout, trimmed); err == nil {
			return true
		}
	}
	return false
}

func completeReplayDayWindow(beginAt, endAt *time.Time) (*time.Time, *time.Time) {
	if beginAt != nil && endAt == nil {
		_, dayEnd := replayDayBounds(*beginAt)
		return beginAt, &dayEnd
	}
	if beginAt == nil && endAt != nil {
		dayStart, _ := replayDayBounds(*endAt)
		return &dayStart, endAt
	}
	return beginAt, endAt
}

func replayDayBounds(ts time.Time) (time.Time, time.Time) {
	y, m, d := ts.Date()
	loc := ts.Location()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	end := start.Add(24*time.Hour - time.Nanosecond)
	return start, end
}
