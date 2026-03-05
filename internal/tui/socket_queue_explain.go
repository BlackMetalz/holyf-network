package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func buildSocketQueueExplainText(aggregate bool) string {
	lines := []string{
		"  [yellow]Send-Q[white]: bytes queued in kernel send buffer (waiting to be sent/acked).",
		"  [yellow]Recv-Q[white]: bytes received by kernel but not yet read by application.",
		"",
		"  These are [yellow]backlog snapshot[white] values at one moment in time.",
		"  They are [yellow]NOT[white] throughput counters (not B/s, not total bytes sent/recv).",
		"  [dim]0B does not mean no traffic; it means no queued backlog at snapshot time.[white]",
	}
	if aggregate {
		lines = append(lines,
			"",
			"  In replay/group rows, Send-Q/Recv-Q are [yellow]sum of matched connections[white].",
		)
	} else {
		lines = append(lines,
			"",
			"  In Top Connections row, values belong to [yellow]that single socket[white].",
		)
	}
	lines = append(lines, "", "  [dim]Press Enter/Esc to close[white]")
	return strings.Join(lines, "\n")
}

func (a *App) promptSocketQueueExplain() {
	closeModal := func() {
		a.pages.RemovePage("socket-queue-explain")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}

	modal := tview.NewModal().
		SetText(buildSocketQueueExplainText(false)).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(_ int, _ string) {
			closeModal()
		})
	modal.SetTitle(" Send-Q / Recv-Q Explain ")
	modal.SetBorder(true)
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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

	a.pages.RemovePage("socket-queue-explain")
	a.pages.AddPage("socket-queue-explain", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(modal)
}

func (h *HistoryApp) promptSocketQueueExplain() {
	closeModal := func() {
		h.pages.RemovePage("history-socket-queue-explain")
		h.app.SetFocus(h.panel)
		h.updateStatusBar()
	}

	modal := tview.NewModal().
		SetText(buildSocketQueueExplainText(true)).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(_ int, _ string) {
			closeModal()
		})
	modal.SetTitle(" Send-Q / Recv-Q Explain ")
	modal.SetBorder(true)
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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

	h.pages.RemovePage("history-socket-queue-explain")
	h.pages.AddPage("history-socket-queue-explain", modal, true, true)
	h.updateStatusBar()
	h.app.SetFocus(modal)
}
