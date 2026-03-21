package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	historyTracePage        = "history-trace-history"
	historyTraceDetailPage  = "history-trace-history-detail"
	historyTraceComparePage = "history-trace-history-compare"
)

func (h *HistoryApp) replayTraceHistoryEntries() []traceHistoryEntry {
	entries, err := readTraceHistoryEntriesFromDir(h.dataDir)
	if err != nil || len(entries) == 0 {
		return nil
	}
	entries = h.filterTraceEntriesByReplayRange(entries)
	if len(entries) == 0 {
		return nil
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].CapturedAt.After(entries[j].CapturedAt)
	})
	return entries
}

func (h *HistoryApp) promptReplayTraceHistory() {
	entries := h.replayTraceHistoryEntries()
	if len(entries) == 0 {
		h.showReplayTraceHistoryEmptyModal()
		return
	}

	closeModal := func() {
		h.pages.RemovePage(historyTraceComparePage)
		h.pages.RemovePage(historyTraceDetailPage)
		h.pages.RemovePage(historyTracePage)
		if h.panel != nil {
			h.app.SetFocus(h.panel)
		}
		h.updateStatusBar()
	}

	list := tview.NewList().
		ShowSecondaryText(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedBackgroundColor(tcell.ColorDarkCyan)
	list.SetBorder(true)
	list.SetTitle(" Replay Trace History ")

	for _, entry := range entries {
		mainText, secondary := formatTraceHistoryListItem(entry, h.sensitiveIP)
		list.AddItem(mainText, secondary, 0, nil)
	}

	openDetail := func() {
		idx := list.GetCurrentItem()
		if idx < 0 || idx >= len(entries) {
			return
		}
		h.showReplayTraceHistoryDetail(entries[idx], list)
	}

	compareMarked := -1
	rangeLabel := h.replayRangeLabel()
	footer := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft)
	refreshFooter := func() {
		text := fmt.Sprintf("  [dim]Enter: detail | c: mark baseline/compare | Esc: close | range=%s | data-dir=%s", rangeLabel, h.dataDir)
		if compareMarked >= 0 && compareMarked < len(entries) {
			text += fmt.Sprintf(" | baseline=%s", entries[compareMarked].CapturedAt.Local().Format("15:04:05"))
		}
		footer.SetText(text + "[white]")
	}
	refreshFooter()

	handleCompare := func() {
		idx := list.GetCurrentItem()
		if idx < 0 || idx >= len(entries) {
			return
		}
		if compareMarked < 0 {
			compareMarked = idx
			h.setStatusNote("Baseline marked. Move to incident row and press c again.", 5*time.Second)
			refreshFooter()
			return
		}
		if compareMarked == idx {
			compareMarked = -1
			h.setStatusNote("Trace compare baseline cleared", 4*time.Second)
			refreshFooter()
			return
		}
		h.showReplayTraceHistoryCompare(entries[compareMarked], entries[idx], list)
		compareMarked = -1
		refreshFooter()
	}

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			closeModal()
			return nil
		case tcell.KeyEnter:
			openDetail()
			return nil
		case tcell.KeyRune:
			if event.Rune() == 'c' || event.Rune() == 'C' {
				handleCompare()
				return nil
			}
		}
		return event
	})

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(list, 0, 1, true).
		AddItem(footer, 1, 0, false)

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(content, 130, 0, true).
			AddItem(nil, 0, 1, false),
			22, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.RemovePage(historyTraceComparePage)
	h.pages.RemovePage(historyTraceDetailPage)
	h.pages.RemovePage(historyTracePage)
	h.pages.AddPage(historyTracePage, modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(list)
}

func (h *HistoryApp) showReplayTraceHistoryEmptyModal() {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft).
		SetText(
			"  No trace history in current replay scope/range\n\n" +
				fmt.Sprintf("  [dim]Data dir: %s[white]\n", strings.TrimSpace(h.dataDir)) +
				fmt.Sprintf("  [dim]Range: %s[white]\n\n", h.replayRangeLabel()) +
				"  [dim]Press Enter/Esc to close.[white]",
		)
	view.SetBorder(true)
	view.SetTitle(" Replay Trace History ")

	closeModal := func() {
		h.pages.RemovePage(historyTracePage)
		if h.panel != nil {
			h.app.SetFocus(h.panel)
		}
		h.updateStatusBar()
	}
	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc, tcell.KeyEnter:
			closeModal()
			return nil
		}
		return event
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(view, 110, 0, true).
			AddItem(nil, 0, 1, false),
			10, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.RemovePage(historyTracePage)
	h.pages.AddPage(historyTracePage, modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(view)
}

func (h *HistoryApp) showReplayTraceHistoryDetail(entry traceHistoryEntry, backFocus tview.Primitive) {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft).
		SetText(buildTraceHistoryDetailText(entry, h.sensitiveIP))
	view.SetBorder(true)
	view.SetTitle(" Replay Trace Detail ")

	closeModal := func() {
		h.pages.RemovePage(historyTraceDetailPage)
		if backFocus != nil {
			h.app.SetFocus(backFocus)
		}
		h.updateStatusBar()
	}
	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc, tcell.KeyEnter:
			closeModal()
			return nil
		}
		return event
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(view, 124, 0, true).
			AddItem(nil, 0, 1, false),
			26, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.RemovePage(historyTraceDetailPage)
	h.pages.AddPage(historyTraceDetailPage, modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(view)
}

func (h *HistoryApp) showReplayTraceHistoryCompare(baseline, incident traceHistoryEntry, backFocus tview.Primitive) {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft).
		SetText(buildTraceHistoryCompareText(baseline, incident, h.sensitiveIP))
	view.SetBorder(true)
	view.SetTitle(" Replay Trace Compare ")

	closeModal := func() {
		h.pages.RemovePage(historyTraceComparePage)
		if backFocus != nil {
			h.app.SetFocus(backFocus)
		}
		h.updateStatusBar()
	}
	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc, tcell.KeyEnter:
			closeModal()
			return nil
		}
		return event
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(view, 132, 0, true).
			AddItem(nil, 0, 1, false),
			24, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.RemovePage(historyTraceComparePage)
	h.pages.AddPage(historyTraceComparePage, modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(view)
}
