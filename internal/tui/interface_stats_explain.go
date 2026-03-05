package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func buildInterfaceStatsExplainText() string {
	lines := []string{
		"  [yellow]RX / TX[white]: interface throughput (bytes/sec) across all traffic on this NIC.",
		"  [yellow]Packets RX / TX[white]: packet rate (packets/sec) on this NIC, not per process.",
		"",
		"  [dim]Why bytes and packets both matter:[white]",
		"  - High packets with low bytes => many small packets (chatty traffic).",
		"  - High bytes with lower packets => larger payload transfers.",
		"",
		"  [yellow]Errors / Drops[white]: cumulative interface counters from kernel/NIC stats.",
		"  [dim]These are totals since boot/driver reset, not per-interval deltas.[white]",
		"",
		"  [dim]First refresh shows baseline only. Rates appear from next sample onward.[white]",
		"",
		"  [dim]Press Enter/Esc to close[white]",
	}
	return strings.Join(lines, "\n")
}

func (a *App) promptInterfaceStatsExplain() {
	closeModal := func() {
		a.pages.RemovePage("interface-stats-explain")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}

	modal := tview.NewModal().
		SetText(buildInterfaceStatsExplainText()).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(_ int, _ string) {
			closeModal()
		})
	modal.SetTitle(" Interface Stats Explain ")
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

	a.pages.RemovePage("interface-stats-explain")
	a.pages.AddPage("interface-stats-explain", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(modal)
}
