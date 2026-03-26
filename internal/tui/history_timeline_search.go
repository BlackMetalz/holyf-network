package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
	tuipanels "github.com/BlackMetalz/holyf-network/internal/tui/panels"
	tuireplay "github.com/BlackMetalz/holyf-network/internal/tui/replay"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (h *HistoryApp) promptTimelineSearch() {
	if len(h.refs) == 0 {
		h.setStatusNote("No snapshots available", 4*time.Second)
		return
	}
	if h.timelineSearchRunning {
		h.setStatusNote("Timeline search is already running", 4*time.Second)
		return
	}

	input := tview.NewInputField()
	input.SetLabel("Search timeline (contains): ")
	input.SetFieldWidth(44)
	input.SetText(h.timelineSearchQuery)
	input.SetBorder(true)
	input.SetTitle(" Timeline Search ")

	closeModal := func() {
		h.pages.RemovePage("history-timeline-search")
		h.app.SetFocus(h.panel)
		h.updateStatusBar()
	}

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			query := strings.TrimSpace(input.GetText())
			closeModal()
			if query == "" {
				return
			}
			h.startTimelineSearch(query)
			return
		}
		closeModal()
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 68, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.AddPage("history-timeline-search", modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(input)
}

func (h *HistoryApp) startTimelineSearch(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	if h.timelineSearchRunning {
		h.setStatusNote("Timeline search is already running", 4*time.Second)
		return
	}
	if len(h.refs) == 0 {
		h.setStatusNote("No snapshots available", 4*time.Second)
		return
	}

	h.followLatest = false
	h.timelineSearchRunning = true
	h.timelineSearchQuery = query
	h.timelineSearchResults = nil
	h.timelineSearchSelected = 0
	h.setStatusNote(fmt.Sprintf("Searching timeline for '%s'...", shortStatus(query, 48)), 5*time.Second)

	refs := append([]history.SnapshotRef(nil), h.refs...)
	go func(searchQuery string, searchRefs []history.SnapshotRef) {
		results := h.scanTimelineMatches(searchQuery, searchRefs)

		h.app.QueueUpdateDraw(func() {
			h.timelineSearchRunning = false
			h.timelineSearchQuery = searchQuery
			h.timelineSearchResults = results
			h.timelineSearchSelected = 0

			if len(results) == 0 {
				h.setStatusNote(fmt.Sprintf("No snapshots matched '%s'", shortStatus(searchQuery, 48)), 6*time.Second)
				return
			}

			h.showTimelineSearchResults()
			h.setStatusNote(
				fmt.Sprintf("Found %d matching snapshots for '%s'", len(results), shortStatus(searchQuery, 48)),
				6*time.Second,
			)
		})
	}(query, refs)
}

func (h *HistoryApp) scanTimelineMatches(query string, refs []history.SnapshotRef) []tuireplay.SearchResult {
	query = strings.TrimSpace(query)
	if query == "" || len(refs) == 0 {
		return nil
	}

	return tuireplay.ScanTimelineMatches(refs, func(ref history.SnapshotRef) (int, error) {
		record, err := history.ReadSnapshot(ref)
		if err != nil {
			return 0, err
		}
		return len(tuipanels.FilterHistoryGroupsByText(h.rowsForDirection(record, h.topDirection), query)), nil
	})
}

func (h *HistoryApp) showTimelineSearchResults() {
	if len(h.timelineSearchResults) == 0 {
		return
	}

	closeModal := func() {
		h.pages.RemovePage("history-timeline-results")
		h.app.SetFocus(h.panel)
		h.updateStatusBar()
	}

	list := tview.NewList().ShowSecondaryText(false)
	list.SetBorder(true)
	list.SetTitle(" Timeline Search Results ")
	list.SetTitleAlign(tview.AlignCenter)

	totalSnapshots := len(h.refs)
	for _, result := range h.timelineSearchResults {
		label := fmt.Sprintf(
			"%s | snapshot %d/%d | matches=%d",
			result.CapturedAt.Local().Format("2006-01-02 15:04:05"),
			result.SnapshotIndex+1,
			totalSnapshots,
			result.MatchCount,
		)
		list.AddItem(label, "", 0, nil)
	}

	if h.timelineSearchSelected < 0 {
		h.timelineSearchSelected = 0
	}
	if h.timelineSearchSelected >= len(h.timelineSearchResults) {
		h.timelineSearchSelected = len(h.timelineSearchResults) - 1
	}
	list.SetCurrentItem(h.timelineSearchSelected)

	list.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		h.jumpToTimelineSearchResult(index)
		closeModal()
	})
	list.SetDoneFunc(func() {
		closeModal()
	})
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeModal()
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			closeModal()
			return nil
		}
		return event
	})

	resultHeight := len(h.timelineSearchResults) + 4
	if resultHeight < 8 {
		resultHeight = 8
	}
	if resultHeight > 24 {
		resultHeight = 24
	}

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(list, 104, 0, true).
			AddItem(nil, 0, 1, false),
			resultHeight, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.RemovePage("history-timeline-results")
	h.pages.AddPage("history-timeline-results", modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(list)
}

func (h *HistoryApp) jumpToTimelineSearchResult(resultIndex int) {
	if resultIndex < 0 || resultIndex >= len(h.timelineSearchResults) {
		return
	}
	result := h.timelineSearchResults[resultIndex]
	if result.SnapshotIndex < 0 || result.SnapshotIndex >= len(h.refs) {
		return
	}

	h.followLatest = false
	h.timelineSearchSelected = resultIndex
	h.loadSnapshotAt(result.SnapshotIndex)

	msg := fmt.Sprintf(
		"Timeline match '%s': snapshot %d/%d (%d rows)",
		shortStatus(h.timelineSearchQuery, 48),
		result.SnapshotIndex+1,
		len(h.refs),
		result.MatchCount,
	)
	h.setSnapshotMessage(msg)
	h.setStatusNote(msg, 8*time.Second)
	h.renderPanel()
	h.updateStatusBar()
}
