package podlookup

import (
	"fmt"
	"strconv"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/podlookup"
	"github.com/BlackMetalz/holyf-network/internal/tui/blocking"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const pageName = "pod-lookup-form"

// PromptPodLookup shows an input modal to enter a port number for K8s pod lookup.
func PromptPodLookup(ctx blocking.UIContext, prefilledPort string) {
	portInput := tview.NewInputField().
		SetLabel("Port number: ").
		SetFieldWidth(10).
		SetText(prefilledPort)
	portInput.SetAcceptanceFunc(tview.InputFieldInteger)

	form := tview.NewForm().AddFormItem(portInput)
	form.SetItemPadding(0)
	form.SetButtonsAlign(tview.AlignRight)

	cancelFunc := func() {
		ctx.RemovePage(pageName)
		ctx.UpdateStatusBar()
		ctx.RestoreFocus()
	}

	submit := func() {
		portStr := portInput.GetText()
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			ctx.SetStatusNote("Invalid port number", 4*time.Second)
			return
		}

		ctx.RemovePage(pageName)
		ctx.UpdateStatusBar()
		ctx.RestoreFocus()
		ctx.SetStatusNote(fmt.Sprintf("Scanning namespaces for port %d...", port), 10*time.Second)

		go runPodLookup(ctx, port)
	}

	form.AddButton("Lookup", submit)
	form.AddButton("Cancel", cancelFunc)

	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			cancelFunc()
			return nil
		case tcell.KeyEnter:
			submit()
			return nil
		}
		return event
	})

	helpLine := tview.NewTextView().
		SetDynamicColors(true).
		SetText("  [dim]Scan all network namespaces to find the pod owning this port[white]")

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(helpLine, 1, 0, false).
				AddItem(form, 0, 1, true),
				60, 0, true).
			AddItem(nil, 0, 1, false),
			7, 0, true).
		AddItem(nil, 0, 1, false)

	ctx.AddPage(pageName, modal, true, true)
	ctx.SetFocus(form)
}

func runPodLookup(ctx blocking.UIContext, port int) {
	start := time.Now()
	result, nsCount := podlookup.FindPortOwner(port)
	elapsed := time.Since(start)

	ctx.QueueUpdateDraw(func() {
		if result != nil {
			ctx.SetStatusNote(fmt.Sprintf("Found port %d owner: %s", port, result.PodName), 6*time.Second)
			ShowPodLookupResult(ctx, result)
		} else {
			ctx.SetStatusNote(fmt.Sprintf("Port %d not found in %d namespaces", port, nsCount), 6*time.Second)
			ShowPodLookupNotFound(ctx, port, nsCount, elapsed)
		}
	})
}
