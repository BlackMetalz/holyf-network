package podlookup

import (
	"fmt"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/podlookup"
	"github.com/BlackMetalz/holyf-network/internal/tui/blocking"
	tuioverlays "github.com/BlackMetalz/holyf-network/internal/tui/overlays"
	"github.com/gdamore/tcell/v2"
)

const resultPageName = "pod-lookup-result"

// ShowPodLookupResult displays the pod lookup result in a modal.
func ShowPodLookupResult(ctx blocking.UIContext, result *podlookup.PodLookupResult) {
	var sb strings.Builder
	sb.WriteString("\n")

	writeField := func(label, value string) {
		if value == "" {
			value = "[dim]-[white]"
		}
		fmt.Fprintf(&sb, "  [yellow]%-16s[white] %s\n", label+":", value)
	}

	writeField("PID", fmt.Sprintf("%d", result.PID))
	writeField("Process", result.ProcName)
	writeField("Container ID", result.ContainerID)
	writeField("Pod", result.PodName)
	writeField("Namespace", result.PodNamespace)
	writeField("Deployment", result.Deployment)
	writeField("Network NS", result.NetNS)
	writeField("Local IP", result.LocalIP)
	writeField("State", result.State)

	sb.WriteString("\n  [dim]Press Esc to close[white]\n")

	showResultModal(ctx, fmt.Sprintf(" K8s Pod Lookup: port %d ", result.Port), sb.String())
}

// ShowPodLookupNotFound displays a not-found message.
func ShowPodLookupNotFound(ctx blocking.UIContext, port int, nsCount int, elapsed time.Duration) {
	text := fmt.Sprintf(
		"\n  Port [yellow]%d[white] not found in any network namespace.\n"+
			"  Scanned [aqua]%d[white] namespaces in %s.\n\n"+
			"  [dim]Press Esc to close[white]\n",
		port, nsCount, elapsed.Truncate(time.Millisecond))

	showResultModal(ctx, fmt.Sprintf(" K8s Pod Lookup: port %d ", port), text)
}

func showResultModal(ctx blocking.UIContext, title, text string) {
	modal, view := tuioverlays.CreateCenteredTextViewModal(title, text)

	closeFunc := func() {
		ctx.RemovePage(resultPageName)
		ctx.UpdateStatusBar()
		ctx.RestoreFocus()
	}

	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc, tcell.KeyEnter:
			closeFunc()
			return nil
		}
		if event.Key() == tcell.KeyRune {
			closeFunc()
			return nil
		}
		return event
	})

	ctx.RemovePage(resultPageName)
	ctx.AddPage(resultPageName, modal, true, true)
	ctx.SetFocus(view)
}
