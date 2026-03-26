package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/tui/diagnosis"
	tuipanels "github.com/BlackMetalz/holyf-network/internal/tui/panels"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// --- Diagnosis History ---

func (a *App) appendDiagnosisHistory(now time.Time, diag *tuishared.Diagnosis) {
	a.diagnosisEngine.Append(now, diag)
}

func (a *App) recentDiagnosisHistory(limit int) []diagnosis.HistoryEntry {
	return a.diagnosisEngine.Recent(limit)
}

func (a *App) promptDiagnosisHistory() {
	entries := a.recentDiagnosisHistory(diagnosisHistoryLimit)

	var body strings.Builder
	if len(entries) == 0 {
		body.WriteString("  No diagnosis changes yet")
	} else {
		for i, entry := range entries {
			body.WriteString("  ")
			body.WriteString(formatDiagnosisHistoryEntry(entry))
			if i < len(entries)-1 {
				body.WriteString("\n")
			}
		}
	}
	body.WriteString("\n\n")
	body.WriteString(fmt.Sprintf(
		"  [dim]Showing latest %d diagnosis changes in this live session[white]",
		diagnosisHistoryLimit,
	))

	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetTextAlign(tview.AlignLeft).
		SetText(body.String())
	view.SetBorder(true)
	view.SetTitle(fmt.Sprintf(" Diagnosis History (latest %d) ", diagnosisHistoryLimit))

	closeModal := func() {
		a.pages.RemovePage("diagnosis-history")
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
			AddItem(view, 116, 0, true).
			AddItem(nil, 0, 1, false),
			16, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage("diagnosis-history")
	a.pages.AddPage("diagnosis-history", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(view)
}

func formatDiagnosisHistoryEntry(entry diagnosis.HistoryEntry) string {
	color := tuishared.ColorForHealthLevel(entry.Diagnosis.Severity)
	conf := shortStatus(tuipanels.DiagnosisConfidenceValue(&entry.Diagnosis), 8)
	issue := shortStatus(tuipanels.DiagnosisIssueValue(&entry.Diagnosis), 36)
	scope := shortStatus(tuipanels.DiagnosisScopeValue(&entry.Diagnosis), 36)
	signal := shortStatus(tuipanels.DiagnosisSignalValue(&entry.Diagnosis), 96)

	return fmt.Sprintf(
		"[%s]%s[white] | [%s]%s[white] | %s | %s | %s",
		color,
		formatDiagnosisHistoryRange(entry),
		color,
		issue,
		conf,
		scope,
		signal,
	)
}

func formatDiagnosisHistoryRange(entry diagnosis.HistoryEntry) string {
	first := entry.FirstSeen.Format("15:04:05")
	last := entry.LastSeen.Format("15:04:05")
	if first == last {
		return first
	}
	return first + "-" + last
}

// --- Action Log ---

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
