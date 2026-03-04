package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
		case 'o':
			h.sortMode = NextSortMode(h.sortMode)
			h.selectedIndex = 0
			h.renderPanel()
			h.updateStatusBar()
			return nil
		case 'Q', 'S', 'P', 'R':
			mode, _ := directSortModeForRune(event.Rune())
			h.sortMode = mode
			h.selectedIndex = 0
			h.renderPanel()
			h.updateStatusBar()
			return nil
		case 'g':
			h.groupView = !h.groupView
			h.selectedIndex = 0
			h.renderPanel()
			h.updateStatusBar()
			return nil
		case 's':
			h.sensitiveIP = !h.sensitiveIP
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
	h.loadSnapshotAt(h.currentIndex - 1)
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) navigateNext() {
	if len(h.refs) == 0 {
		return
	}
	h.followLatest = false
	h.loadSnapshotAt(h.currentIndex + 1)
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) navigateOldest() {
	if len(h.refs) == 0 {
		return
	}
	h.followLatest = false
	h.loadSnapshotAt(0)
	h.renderPanel()
	h.updateStatusBar()
}

func (h *HistoryApp) navigateLatest() {
	if len(h.refs) == 0 {
		return
	}
	h.loadSnapshotAt(len(h.refs) - 1)
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
	case "history-filter", "history-search":
		return "[dim]Enter[white]=apply [dim]Esc[white]=cancel", "Enter=apply Esc=cancel"
	default:
		return "[dim]lb/rb a e f / o Q/S/P/R g s z L ? q[white]", "lb/rb a e f / o Q/S/P/R g s z L ? q"
	}
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
