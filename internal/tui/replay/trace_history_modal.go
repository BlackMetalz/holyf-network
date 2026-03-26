package replay

import (
	"fmt"
	"strings"
	"time"

	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	tuitrace "github.com/BlackMetalz/holyf-network/internal/tui/trace"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	HistoryTracePage        = "history-trace-history"
	HistoryTraceDetailPage  = "history-trace-history-detail"
	HistoryTraceComparePage = "history-trace-history-compare"
)

func ReplayTraceHistoryEntries(ctx UIContext) []tuitrace.Entry {
	entries, err := LoadTraceHistoryEntries(ctx.DataDir(), ctx.RangeBegin(), ctx.RangeEnd(), true)
	if err != nil || len(entries) == 0 {
		return nil
	}
	return entries
}

func PromptReplayTraceHistory(ctx UIContext) {
	entries := ReplayTraceHistoryEntries(ctx)
	if len(entries) == 0 {
		showReplayTraceHistoryEmptyModal(ctx)
		return
	}

	closeModal := func() {
		ctx.RemovePage(HistoryTraceComparePage)
		ctx.RemovePage(HistoryTraceDetailPage)
		ctx.RemovePage(HistoryTracePage)
		ctx.BackFocus()
	}

	list := tview.NewList().
		ShowSecondaryText(true).
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedBackgroundColor(tcell.ColorDarkCyan)
	list.SetBorder(true)
	list.SetTitle(" Replay Trace History ")

	for _, entry := range entries {
		mainText, secondary := tuitrace.FormatListItem(entry, traceRenderOptions(ctx.SensitiveIP()))
		list.AddItem(mainText, secondary, 0, nil)
	}

	openDetail := func() {
		idx := list.GetCurrentItem()
		if idx < 0 || idx >= len(entries) {
			return
		}
		showReplayTraceHistoryDetail(ctx, entries[idx], list)
	}

	compareMarked := -1
	rangeLabel := ctx.RangeLabel()
	footer := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft)
	refreshFooter := func() {
		text := fmt.Sprintf("  [dim]Enter: detail | c: mark baseline/compare | Esc: close | range=%s | data-dir=%s", rangeLabel, ctx.DataDir())
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
			ctx.SetStatusNote("Baseline marked. Move to incident row and press c again.", 5*time.Second)
			refreshFooter()
			return
		}
		if compareMarked == idx {
			compareMarked = -1
			ctx.SetStatusNote("Trace compare baseline cleared", 4*time.Second)
			refreshFooter()
			return
		}
		ShowReplayTraceHistoryCompare(ctx, entries[compareMarked], entries[idx], list)
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

	ctx.RemovePage(HistoryTraceComparePage)
	ctx.RemovePage(HistoryTraceDetailPage)
	ctx.RemovePage(HistoryTracePage)
	ctx.AddPage(HistoryTracePage, modal, true, true)
	ctx.UpdateStatusBar()
	ctx.SetFocus(list)
}

func showReplayTraceHistoryEmptyModal(ctx UIContext) {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft).
		SetText(
			"  No trace history in current replay scope/range\n\n" +
				fmt.Sprintf("  [dim]Data dir: %s[white]\n", strings.TrimSpace(ctx.DataDir())) +
				fmt.Sprintf("  [dim]Range: %s[white]\n\n", ctx.RangeLabel()) +
				"  [dim]Press Enter/Esc to close.[white]",
		)
	view.SetBorder(true)
	view.SetTitle(" Replay Trace History ")

	closeModal := func() {
		ctx.RemovePage(HistoryTracePage)
		ctx.BackFocus()
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

	ctx.RemovePage(HistoryTracePage)
	ctx.AddPage(HistoryTracePage, modal, true, true)
	ctx.UpdateStatusBar()
	ctx.SetFocus(view)
}

func showReplayTraceHistoryDetail(ctx UIContext, entry tuitrace.Entry, backFocus tview.Primitive) {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft).
		SetText(tuitrace.BuildDetailText(entry, traceRenderOptions(ctx.SensitiveIP())))
	view.SetBorder(true)
	view.SetTitle(" Replay Trace Detail ")

	closeModal := func() {
		ctx.RemovePage(HistoryTraceDetailPage)
		if backFocus != nil {
			ctx.SetFocus(backFocus)
		}
		ctx.UpdateStatusBar()
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

	ctx.RemovePage(HistoryTraceDetailPage)
	ctx.AddPage(HistoryTraceDetailPage, modal, true, true)
	ctx.UpdateStatusBar()
	ctx.SetFocus(view)
}

func ShowReplayTraceHistoryCompare(ctx UIContext, baseline, incident tuitrace.Entry, backFocus tview.Primitive) {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft).
		SetText(tuitrace.BuildCompareText(baseline, incident, traceRenderOptions(ctx.SensitiveIP())))
	view.SetBorder(true)
	view.SetTitle(" Replay Trace Compare ")

	closeModal := func() {
		ctx.RemovePage(HistoryTraceComparePage)
		if backFocus != nil {
			ctx.SetFocus(backFocus)
		}
		ctx.UpdateStatusBar()
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

	ctx.RemovePage(HistoryTraceComparePage)
	ctx.AddPage(HistoryTraceComparePage, modal, true, true)
	ctx.UpdateStatusBar()
	ctx.SetFocus(view)
}

func traceRenderOptions(sensitiveIP bool) tuitrace.RenderOptions {
	return tuitrace.RenderOptions{
		SensitiveIP:       sensitiveIP,
		SeverityInfo:      "INFO",
		FormatPreviewIP:   tuishared.FormatPreviewIP,
		MaskSensitiveText: tuishared.MaskSensitiveIPsInText,
		ShortStatus:       tuishared.ShortStatus,
		MaskPath: func(path string, sensitiveIP bool) string {
			if !sensitiveIP {
				return path
			}
			return "[masked]"
		},
		MetricDisplay: func(v int, est bool) string {
			if v < 0 {
				return "n/a"
			}
			s := fmt.Sprintf("%d", v)
			if est {
				s += " (est.)"
			}
			return s
		},
		MetricValue: func(v int) string {
			if v < 0 {
				return "n/a"
			}
			return fmt.Sprintf("%d", v)
		},
		SeverityStyled: func(sev string) string {
			return fmt.Sprintf("[%s]%s[white]", tuishared.TracePacketSeverityColor(sev), strings.ToUpper(tuishared.BlankIfUnknown(sev, "INFO")))
		},
		ConfidenceStyled: func(conf string) string {
			return fmt.Sprintf("[aqua]%s[white]", strings.ToUpper(tuishared.BlankIfUnknown(conf, "LOW")))
		},
	}
}
