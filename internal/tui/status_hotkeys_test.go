package tui

import (
	"testing"

	tuireplay "github.com/BlackMetalz/holyf-network/internal/tui/replay"
)

func TestHistoryStatusHotkeysForModalPages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		page      string
		wantPlain string
	}{
		{page: "history-help", wantPlain: "any key=close"},
		{page: "history-filter", wantPlain: "Enter=apply Esc=cancel"},
		{page: "history-search", wantPlain: "Enter=apply Esc=cancel"},
		{page: "history-jump-time", wantPlain: "Enter=apply Esc=cancel"},
		{page: "history-timeline-search", wantPlain: "Enter=search Esc=cancel"},
		{page: "history-timeline-results", wantPlain: "Up/Down=select Enter=jump Esc=close"},
		{page: tuireplay.HistoryTracePage, wantPlain: "Up/Down=select Enter=detail c=compare Esc=close"},
		{page: tuireplay.HistoryTraceDetailPage, wantPlain: "Enter=close Esc=close"},
		{page: tuireplay.HistoryTraceComparePage, wantPlain: "Enter=close Esc=close"},
		{page: "main", wantPlain: "[=prev ]=next a e t f / Shift+S Shift+B/C/P o g h m i Shift+I x z L ? q"},
		{page: "history-socket-queue-explain", wantPlain: "Enter=close Esc=close"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.page, func(t *testing.T) {
			t.Parallel()
			_, plain := tuireplay.StatusHotkeysForPage(tc.page)
			if plain != tc.wantPlain {
				t.Fatalf("plain hotkeys mismatch for page=%q: got=%q want=%q", tc.page, plain, tc.wantPlain)
			}
		})
	}
}
