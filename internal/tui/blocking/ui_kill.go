package blocking

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func ParseBlockMinutes(raw string) (int, error) {
	minutes, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || minutes < 0 || minutes > maxBlockMinutes {
		return 0, fmt.Errorf("Block minutes must be 0-%d", maxBlockMinutes)
	}
	return minutes, nil
}

func IsKillOnlyMinutes(minutes int) bool {
	return minutes == 0
}

func parsePeerIPInput(raw string) (string, bool) {
	peerIP := strings.TrimSpace(raw)
	peerIP = strings.TrimPrefix(peerIP, "[")
	peerIP = strings.TrimSuffix(peerIP, "]")
	peerIP = strings.TrimPrefix(peerIP, "::ffff:")
	parsed := net.ParseIP(peerIP)
	if parsed == nil {
		return "", false
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String(), true
	}
	return parsed.String(), true
}

func PromptKillPeer(ctx UIContext, m *Manager, suggested *PeerKillTarget) {
	m.EnterKillFlowPause(ctx)

	filteredPort := 0
	if ctx.PortFilter() != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(ctx.PortFilter()))
		if err == nil && parsed >= 1 && parsed <= 65535 {
			filteredPort = parsed
		}
	}

	defaultPeer := ""
	defaultPort := ""
	helpText := fmt.Sprintf("Enter peer IP + local port + block minutes (0=kill only, default %d).", defaultBlockMinutes)

	if suggested != nil {
		defaultPeer = suggested.PeerIP
		defaultPort = strconv.Itoa(suggested.LocalPort)
		helpText = fmt.Sprintf("Suggested: %s -> local port %d (%d matches in view).", suggested.PeerIP, suggested.LocalPort, suggested.Count)
	}

	if filteredPort > 0 {
		defaultPort = strconv.Itoa(filteredPort)
		if suggested != nil {
			helpText = fmt.Sprintf("Port filter active: local port %d. Suggested peer: %s (%d matches).", filteredPort, suggested.PeerIP, suggested.Count)
		} else {
			helpText = fmt.Sprintf("Port filter active: local port %d. Enter peer IP to block.", filteredPort)
		}
	}

	peerInput := tview.NewInputField().
		SetLabel("Peer IP: ").
		SetFieldWidth(30).
		SetText(defaultPeer)

	form := tview.NewForm().AddFormItem(peerInput)
	form.SetItemPadding(0)
	form.SetButtonsAlign(tview.AlignRight)

	var portInput *tview.InputField
	if filteredPort == 0 {
		portInput = tview.NewInputField().
			SetLabel("Local port: ").
			SetFieldWidth(8).
			SetText(defaultPort)
		portInput.SetAcceptanceFunc(tview.InputFieldInteger)
		form.AddFormItem(portInput)
	}

	minutesInput := tview.NewInputField().
		SetLabel("Block minutes: ").
		SetFieldWidth(6).
		SetText(strconv.Itoa(defaultBlockMinutes))
	minutesInput.SetAcceptanceFunc(tview.InputFieldInteger)
	form.AddFormItem(minutesInput)

	submit := func() {
		peerIP, ok := parsePeerIPInput(peerInput.GetText())
		if !ok {
			ctx.SetStatusNote("Invalid peer IP", 5*time.Second)
			return
		}

		port := filteredPort
		if filteredPort == 0 {
			parsedPort, err := strconv.Atoi(strings.TrimSpace(portInput.GetText()))
			if err != nil || parsedPort < 1 || parsedPort > 65535 {
				ctx.SetStatusNote("Invalid local port", 5*time.Second)
				return
			}
			port = parsedPort
		}

		minutes, err := ParseBlockMinutes(minutesInput.GetText())
		if err != nil {
			ctx.SetStatusNote(err.Error(), 5*time.Second)
			return
		}

		// Calculate matches natively here, or trust the suggested count if it matches
		matchCount := 0
		if suggested != nil && suggested.PeerIP == peerIP && suggested.LocalPort == port {
			matchCount = suggested.Count
		}

		target := PeerKillTarget{
			PeerIP:    peerIP,
			LocalPort: port,
			Count:     matchCount,
		}

		spec := target.ToSpec()
		if !IsKillOnlyMinutes(minutes) && m.HasActiveBlock(spec) {
			ctx.SetStatusNote(fmt.Sprintf("Already blocked %s:%d", target.PeerIP, target.LocalPort), 5*time.Second)
			return
		}

		ctx.RemovePage("kill-peer-form")
		ctx.UpdateStatusBar()
		PromptKillPeerConfirm(ctx, m, target, minutes)
	}

	form.AddButton("Next", func() {
		submit()
	})

	cancelFunc := func() {
		ctx.RemovePage("kill-peer-form")
		ctx.RestoreFocus()
		m.ExitKillFlowPause(ctx)
	}

	form.AddButton("Cancel", cancelFunc)
	form.SetCancelFunc(cancelFunc)

	inputCount := 2
	if filteredPort == 0 {
		inputCount = 3
	}
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			cancelFunc()
			return nil
		case tcell.KeyTab:
			currentItem, _ := form.GetFocusedItemIndex()
			if currentItem < 0 || currentItem >= inputCount {
				currentItem = -1
			}
			next := currentItem + 1
			if next >= inputCount {
				next = 0
			}
			form.SetFocus(next)
			ctx.SetFocus(form)
			return nil
		case tcell.KeyEnter:
			submit()
			return nil
		}
		return event
	})
	form.SetBorder(true)
	form.SetTitle(" Kill Peer ")

	helpLine := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetText("  [dim]" + helpText + "[white]")

	modalHeight := 11
	if filteredPort == 0 {
		modalHeight = 12
	}

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(helpLine, 1, 0, false).
				AddItem(form, 0, 1, true),
				84, 0, true).
			AddItem(nil, 0, 1, false),
			modalHeight, 0, true).
		AddItem(nil, 0, 1, false)

	ctx.AddPage("kill-peer-form", modal, true, true)
	ctx.UpdateStatusBar()
	form.SetFocus(0)
	ctx.SetFocus(form)
}

func PromptKillPeerConfirm(ctx UIContext, m *Manager, target PeerKillTarget, minutes int) {
	killOnly := IsKillOnlyMinutes(minutes)
	duration := time.Duration(minutes) * time.Minute
	label := "Block " + FormatBlockDuration(duration)
	text := fmt.Sprintf(
		"Block peer %s -> local port %d for %d minutes?\n\nMatches in current view: %d\nThis inserts a firewall block rule and attempts to terminate active flows.",
		target.PeerIP,
		target.LocalPort,
		minutes,
		target.Count,
	)
	if target.Count == 0 {
		text = fmt.Sprintf(
			"Block peer %s -> local port %d for %d minutes?\n\nMatches in current view: 0 (manual target)\nThis inserts a firewall block rule and attempts to terminate active flows.",
			target.PeerIP,
			target.LocalPort,
			minutes,
		)
	}
	if killOnly {
		label = "Kill Connections"
		text = fmt.Sprintf(
			"Kill active connections for peer %s -> local port %d?\n\nMatches in current view: %d\nThis re-scans matching flows and terminates active connections only (no block rule or timer).",
			target.PeerIP,
			target.LocalPort,
			target.Count,
		)
		if target.Count == 0 {
			text = fmt.Sprintf(
				"Kill active connections for peer %s -> local port %d?\n\nMatches in current view: 0 (manual target)\nThis re-scans matching flows and terminates active connections only (no block rule or timer).",
				target.PeerIP,
				target.LocalPort,
			)
		}
	}

	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{label, "Cancel"}).
		SetDoneFunc(func(_ int, button string) {
			ctx.RemovePage("kill-peer")
			ctx.RestoreFocus()
			m.ExitKillFlowPause(ctx)
			if button != label {
				return
			}

			// Snapshot latestTalkers from ctx to avoid data race.
			snapshotTalkers := append([]collector.Connection(nil), ctx.LatestTalkers()...)
			if killOnly {
				ctx.SetStatusNote(fmt.Sprintf("Killing %s:%d...", target.PeerIP, target.LocalPort), 4*time.Second)
				go KillPeerConnectionsOnly(ctx, target, snapshotTalkers)
				return
			}

			ctx.SetStatusNote(fmt.Sprintf("Blocking %s:%d...", target.PeerIP, target.LocalPort), 4*time.Second)
			go BlockPeerForDuration(ctx, m, target, duration, snapshotTalkers)
		})
	modal.SetTitle(" Kill Peer ")
	modal.SetBorder(true)

	ctx.AddPage("kill-peer", modal, true, true)
	ctx.UpdateStatusBar()
	ctx.SetFocus(modal)
}

func ShowBlockSummaryPopup(ctx UIContext, summary string) {
	modal := tview.NewModal().
		SetText(summary).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(_ int, _ string) {
			ctx.RemovePage("block-summary")
			ctx.UpdateStatusBar()
			ctx.RestoreFocus()
		})
	modal.SetTitle(" Block Summary ")
	modal.SetBorder(true)

	ctx.RemovePage("block-summary")
	ctx.AddPage("block-summary", modal, true, true)
	ctx.UpdateStatusBar()
	ctx.SetFocus(modal)
}
