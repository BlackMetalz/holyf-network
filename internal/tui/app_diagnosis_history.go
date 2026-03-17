package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type diagnosisHistoryEntry struct {
	FirstSeen time.Time
	LastSeen  time.Time
	Diagnosis topDiagnosis
}

func (a *App) appendDiagnosisHistory(now time.Time, diagnosis *topDiagnosis) {
	if diagnosis == nil {
		return
	}

	entry := diagnosisHistoryEntry{
		FirstSeen: now,
		LastSeen:  now,
		Diagnosis: cloneTopDiagnosis(diagnosis),
	}

	if len(a.diagnosisHistory) > 0 {
		last := &a.diagnosisHistory[0]
		if diagnosisFingerprint(&last.Diagnosis) == diagnosisFingerprint(diagnosis) {
			last.LastSeen = now
			last.Diagnosis = entry.Diagnosis
			return
		}
	}

	a.diagnosisHistory = append([]diagnosisHistoryEntry{entry}, a.diagnosisHistory...)
	if len(a.diagnosisHistory) > diagnosisHistoryLimit {
		a.diagnosisHistory = append([]diagnosisHistoryEntry(nil), a.diagnosisHistory[:diagnosisHistoryLimit]...)
	}
}

func (a *App) recentDiagnosisHistory(limit int) []diagnosisHistoryEntry {
	if limit <= 0 || limit > diagnosisHistoryLimit {
		limit = diagnosisHistoryLimit
	}
	if len(a.diagnosisHistory) == 0 {
		return nil
	}
	if limit > len(a.diagnosisHistory) {
		limit = len(a.diagnosisHistory)
	}
	out := make([]diagnosisHistoryEntry, limit)
	copy(out, a.diagnosisHistory[:limit])
	return out
}

func diagnosisFingerprint(diagnosis *topDiagnosis) string {
	if diagnosis == nil {
		return ""
	}
	parts := []string{
		fmt.Sprintf("%d", diagnosis.Severity),
		strings.TrimSpace(diagnosisIssueValue(diagnosis)),
		strings.TrimSpace(diagnosisScopeValue(diagnosis)),
		strings.TrimSpace(diagnosisLikelyValue(diagnosis)),
		strings.TrimSpace(diagnosisCheckValue(diagnosis)),
	}
	return strings.Join(parts, "|")
}

func cloneTopDiagnosis(diagnosis *topDiagnosis) topDiagnosis {
	if diagnosis == nil {
		return topDiagnosis{}
	}
	clone := *diagnosis
	if len(diagnosis.Evidence) > 0 {
		clone.Evidence = append([]string(nil), diagnosis.Evidence...)
	}
	if len(diagnosis.NextChecks) > 0 {
		clone.NextChecks = append([]string(nil), diagnosis.NextChecks...)
	}
	return clone
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

func formatDiagnosisHistoryEntry(entry diagnosisHistoryEntry) string {
	color := colorForHealthLevel(entry.Diagnosis.Severity)
	issue := shortStatus(diagnosisIssueValue(&entry.Diagnosis), 36)
	scope := shortStatus(diagnosisScopeValue(&entry.Diagnosis), 36)
	signal := shortStatus(diagnosisSignalValue(&entry.Diagnosis), 96)

	return fmt.Sprintf(
		"[%s]%s[white] | [%s]%s[white] | %s | %s",
		color,
		formatDiagnosisHistoryRange(entry),
		color,
		issue,
		scope,
		signal,
	)
}

func formatDiagnosisHistoryRange(entry diagnosisHistoryEntry) string {
	first := entry.FirstSeen.Format("15:04:05")
	last := entry.LastSeen.Format("15:04:05")
	if first == last {
		return first
	}
	return first + "-" + last
}
