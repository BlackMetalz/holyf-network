package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/history"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (h *HistoryApp) handleKeyEvent(event *tcell.EventKey) *tcell.EventKey {
	if h.isHelpVisible() {
		h.hideHelp()
		return nil
	}
	if h.isOverlayVisible() {
		return event
	}

	switch event.Key() {
	case tcell.KeyUp:
		if h.moveSelection(-1) {
			return nil
		}
		return event
	case tcell.KeyDown:
		if h.moveSelection(1) {
			return nil
		}
		return event
	case tcell.KeyEnter:
		h.setStatusNote("Read-only replay mode", 4*time.Second)
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case 'q':
			select {
			case <-h.stopChan:
			default:
				close(h.stopChan)
			}
			h.app.Stop()
			return nil
		case '?':
			h.showHelp()
			return nil
		case '[':
			h.navigatePrev()
			return nil
		case ']':
			h.navigateNext()
			return nil
		case 'a', 'A':
			h.navigateOldest()
			return nil
		case 'e', 'E':
			h.navigateLatest()
			return nil
		case 'f':
			h.promptPortFilter()
			return nil
		case '/':
			h.promptTextFilter()
			return nil
		case 't', 'T':
			h.promptJumpToTime()
			return nil
		case 'S':
			h.promptTimelineSearch()
			return nil
		case 'B', 'C', 'P':
			mode, _ := directSortModeForRune(event.Rune())
			h.applySortInput(mode)
			return nil
		case 's':
			h.sensitiveIP = !h.sensitiveIP
			h.renderPanel()
			h.updateStatusBar()
			return nil
		case 'i', 'I':
			h.promptSocketQueueExplain()
			return nil
		case 'x', 'X':
			h.skipEmpty = !h.skipEmpty
			if h.skipEmpty {
				h.setStatusNote("Skip-empty enabled", 4*time.Second)
				h.currentIndex = h.adjustGenericIndexForSkipEmpty(h.currentIndex)
			} else {
				h.setStatusNote("Skip-empty disabled", 4*time.Second)
			}
			h.renderPanel()
			h.updateStatusBar()
			return nil
		case 'L':
			h.followLatest = !h.followLatest
			if h.followLatest {
				h.reloadIndex(false)
				h.navigateLatest()
				h.setStatusNote("Follow latest enabled", 4*time.Second)
			} else {
				h.setStatusNote("Follow latest disabled", 4*time.Second)
			}
			h.renderPanel()
			h.updateStatusBar()
			return nil
		case 'z':
			h.setStatusNote("Single-panel mode: zoom not needed", 4*time.Second)
			return nil
		case 'k', 'b':
			h.setStatusNote("Read-only replay mode", 4*time.Second)
			return nil
		}
	}

	return event
}

func (h *HistoryApp) navigatePrev() {
	if len(h.refs) == 0 {
		return
	}
	h.followLatest = false
	target := h.currentIndex - 1
	if h.skipEmpty {
		if idx, skipped, ok := h.findPrevNonEmptyIndex(target); ok {
			target = idx
			if skipped > 0 {
				h.setStatusNote(fmt.Sprintf("Skipped %d empty snapshots", skipped), 4*time.Second)
			}
		} else {
			h.setStatusNote("No previous active snapshot", 4*time.Second)
			h.renderPanel()
			h.updateStatusBar()
			return
		}
	}
	h.loadSnapshotAt(target)
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) navigateNext() {
	if len(h.refs) == 0 {
		return
	}
	h.followLatest = false
	target := h.currentIndex + 1
	if h.skipEmpty {
		if idx, skipped, ok := h.findNextNonEmptyIndex(target); ok {
			target = idx
			if skipped > 0 {
				h.setStatusNote(fmt.Sprintf("Skipped %d empty snapshots", skipped), 4*time.Second)
			}
		} else {
			h.setStatusNote("No next active snapshot", 4*time.Second)
			h.renderPanel()
			h.updateStatusBar()
			return
		}
	}
	h.loadSnapshotAt(target)
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) navigateOldest() {
	if len(h.refs) == 0 {
		return
	}
	h.followLatest = false
	target := 0
	if h.skipEmpty {
		if idx, skipped, ok := h.findNextNonEmptyIndex(target); ok {
			target = idx
			if skipped > 0 {
				h.setStatusNote(fmt.Sprintf("Oldest is empty, jumped forward %d snapshots", skipped), 4*time.Second)
			}
		}
	}
	h.loadSnapshotAt(target)
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) navigateLatest() {
	if len(h.refs) == 0 {
		return
	}
	target := len(h.refs) - 1
	if h.skipEmpty {
		if idx, skipped, ok := h.findPrevNonEmptyIndex(target); ok {
			target = idx
			if skipped > 0 {
				h.setStatusNote(fmt.Sprintf("Latest is empty, jumped back %d snapshots", skipped), 4*time.Second)
			}
		}
	}
	h.loadSnapshotAt(target)
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) promptPortFilter() {
	if h.portFilter != "" || h.textFilter != "" {
		h.portFilter = ""
		h.textFilter = ""
		h.selectedIndex = 0
		h.renderPanel()
		h.updateStatusBar()
		return
	}

	input := tview.NewInputField()
	input.SetLabel("Filter by port: ")
	input.SetFieldWidth(10)
	input.SetBorder(true)
	input.SetTitle(" Port Filter ")
	input.SetAcceptanceFunc(tview.InputFieldInteger)

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			parsed, err := parseHistoryPortFilter(input.GetText())
			if err != nil {
				h.setStatusNote("Invalid port filter", 4*time.Second)
				return
			}
			h.portFilter = parsed
			h.selectedIndex = 0
			h.renderPanel()
		}
		h.pages.RemovePage("history-filter")
		h.app.SetFocus(h.panel)
		h.updateStatusBar()
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 30, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.AddPage("history-filter", modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(input)
}

func (h *HistoryApp) promptTextFilter() {
	input := tview.NewInputField()
	input.SetLabel("Search (contains): ")
	input.SetFieldWidth(36)
	input.SetText(h.textFilter)
	input.SetBorder(true)
	input.SetTitle(" Search Filter ")

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			entered := strings.TrimSpace(input.GetText())
			if entered == "" {
				h.portFilter = ""
				h.textFilter = ""
			} else {
				h.textFilter = entered
			}
			h.selectedIndex = 0
			h.renderPanel()
		}
		h.pages.RemovePage("history-search")
		h.app.SetFocus(h.panel)
		h.updateStatusBar()
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 54, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.AddPage("history-search", modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(input)
}

func (h *HistoryApp) promptJumpToTime() {
	if len(h.refs) == 0 {
		h.setStatusNote("No snapshots available", 4*time.Second)
		return
	}

	input := tview.NewInputField()
	input.SetLabel("Jump to time: ")
	input.SetFieldWidth(36)
	if h.currentIndex >= 0 && h.currentIndex < len(h.refs) {
		input.SetText(h.refs[h.currentIndex].CapturedAt.Local().Format("2006-01-02 15:04:05"))
	}
	input.SetBorder(true)
	input.SetTitle(" Jump To Snapshot Time ")

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			target, err := parseHistoryJumpTime(input.GetText(), time.Now())
			if err != nil {
				h.setStatusNote("Invalid time. Use YYYY-MM-DD HH:MM[:SS], HH:MM[:SS], or yesterday HH:MM", 6*time.Second)
				return
			}

			index := h.closestSnapshotIndex(target)
			if index >= 0 {
				if h.skipEmpty {
					if idx, ok := h.findNearestNonEmptyIndex(index); ok {
						index = idx
					}
				}
				h.followLatest = false
				h.loadSnapshotAt(index)
				summary := h.buildJumpSummary(target, index)
				h.setSnapshotMessage(summary)
				h.setStatusNote(summary, 10*time.Second)
			}
			h.renderPanel()
		}
		h.pages.RemovePage("history-jump-time")
		h.app.SetFocus(h.panel)
		h.updateStatusBar()
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(input, 60, 0, true).
			AddItem(nil, 0, 1, false),
			3, 0, true).
		AddItem(nil, 0, 1, false)

	h.pages.AddPage("history-jump-time", modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(input)
}

func (h *HistoryApp) closestSnapshotIndex(target time.Time) int {
	if len(h.refs) == 0 {
		return -1
	}

	targetUTC := target.UTC()
	idx := sort.Search(len(h.refs), func(i int) bool {
		return !h.refs[i].CapturedAt.Before(targetUTC)
	})

	if idx <= 0 {
		return 0
	}
	if idx >= len(h.refs) {
		return len(h.refs) - 1
	}

	before := h.refs[idx-1].CapturedAt
	after := h.refs[idx].CapturedAt
	if targetUTC.Sub(before) <= after.Sub(targetUTC) {
		return idx - 1
	}
	return idx
}

func (h *HistoryApp) showHelp() {
	h.pages.SendToFront("history-help")
	h.pages.ShowPage("history-help")
	h.updateStatusBar()
}

func (h *HistoryApp) hideHelp() {
	h.pages.HidePage("history-help")
	h.updateStatusBar()
}

func (h *HistoryApp) isHelpVisible() bool {
	name, _ := h.pages.GetFrontPage()
	return name == "history-help"
}

func (h *HistoryApp) isOverlayVisible() bool {
	name, _ := h.pages.GetFrontPage()
	return name != "main" && name != "history-help"
}

func historyStatusHotkeysForPage(page string) (styled string, plain string) {
	switch page {
	case "history-help":
		return "[dim]any key[white]=close", "any key=close"
	case "history-filter", "history-search", "history-jump-time":
		return "[dim]Enter[white]=apply [dim]Esc[white]=cancel", "Enter=apply Esc=cancel"
	case "history-timeline-search":
		return "[dim]Enter[white]=search [dim]Esc[white]=cancel", "Enter=search Esc=cancel"
	case "history-timeline-results":
		return "[dim]Up/Down[white]=select [dim]Enter[white]=jump [dim]Esc[white]=close", "Up/Down=select Enter=jump Esc=close"
	case "history-socket-queue-explain":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	default:
		return "[dim][=prev ]=next a e t f / Shift+S Shift+B/C/P s i Shift+I x z L ? q[white]", "[=prev ]=next a e t f / Shift+S Shift+B/C/P s i Shift+I x z L ? q"
	}
}

func (h *HistoryApp) applySortInput(mode SortMode) {
	if h.sortMode == mode {
		h.sortDesc = !h.sortDesc
	} else {
		h.sortMode = mode
		h.sortDesc = true // first hit on mode starts DESC
	}
	h.selectedIndex = 0
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) snapshotSummary() string {
	if len(h.refs) == 0 || h.currentIndex < 0 {
		return "Snapshot: 0/0"
	}
	rec := h.currentRecord
	when := rec.CapturedAt.Local().Format("2006-01-02 15:04:05")
	return fmt.Sprintf("Snapshot: %d/%d (%s)", h.currentIndex+1, len(h.refs), when)
}

func parseHistoryPortFilter(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	port, err := strconv.Atoi(trimmed)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid port filter")
	}
	return strconv.Itoa(port), nil
}

func parseHistoryJumpTime(raw string, now time.Time) (time.Time, error) {
	return history.ParseReplayTime(raw, now)
}
