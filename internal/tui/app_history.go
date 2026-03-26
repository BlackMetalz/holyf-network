package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (a *App) promptActionLog() {
	logs := a.actionLogger.Recent(actionLogModalLimit)

	var body strings.Builder
	if len(logs) == 0 {
		body.WriteString("  No actions yet")
	} else {
		for i, entry := range logs {
			body.WriteString("  ")
			body.WriteString(entry)
			if i < len(logs)-1 {
				body.WriteString("\n")
			}
		}
	}
	body.WriteString("\n\n")
	body.WriteString(fmt.Sprintf(
		"  [dim]Showing latest %d. Full history: %s (rolling %d events)[white]",
		actionLogModalLimit,
		actionHistoryDisplayPath,
		actionLogRotateLimit,
	))

	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetTextAlign(tview.AlignLeft).
		SetText(body.String())
	view.SetBorder(true)
	view.SetTitle(fmt.Sprintf(" Action Log (latest %d) ", actionLogModalLimit))

	closeModal := func() {
		a.pages.RemovePage("action-log")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
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
			AddItem(view, 100, 0, true).
			AddItem(nil, 0, 1, false),
			16, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage("action-log")
	a.pages.AddPage("action-log", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(view)
}

func (a *App) addActionLog(message string) {
	a.actionLogger.Add(message)
}

func (a *App) recentActionLogs(limit int) []string {
	if limit <= 0 {
		limit = actionLogModalLimit
	}
	return a.actionLogger.Recent(limit)
}

func defaultActionHistoryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(strings.TrimSpace(home), actionHistoryDirName, actionHistoryFileName)
}
