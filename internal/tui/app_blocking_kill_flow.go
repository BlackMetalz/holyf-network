package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/actions"
	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type peerKillTarget struct {
	PeerIP    string
	LocalPort int
	Count     int
}

func parseBlockMinutes(raw string) (int, error) {
	minutes, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || minutes < 0 || minutes > maxBlockMinutes {
		return 0, fmt.Errorf("Block minutes must be 0-%d", maxBlockMinutes)
	}
	return minutes, nil
}

func isKillOnlyMinutes(minutes int) bool {
	return minutes == 0
}

// promptKillPeer confirms and applies a temporary firewall block for a peer.
func (a *App) promptKillPeer() {
	if a.focusIndex != 2 {
		a.setStatusNote("Focus Top Connections before kill-peer", 5*time.Second)
		return
	}

	filteredPort := 0
	if a.portFilter != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(a.portFilter))
		if err != nil || parsed < 1 || parsed > 65535 {
			a.setStatusNote("Current port filter must be 1-65535", 5*time.Second)
			return
		}
		filteredPort = parsed
	}
	a.enterKillFlowPause()

	suggested, hasSuggested := a.selectedPeerKillTarget()
	if !hasSuggested {
		suggested, hasSuggested = a.selectPeerKillTarget()
	}
	defaultPeer := ""
	defaultPort := ""
	helpText := fmt.Sprintf("Enter peer IP + local port + block minutes (0=kill only, default %d).", defaultBlockMinutes)
	if hasSuggested {
		defaultPeer = suggested.PeerIP
		defaultPort = strconv.Itoa(suggested.LocalPort)
		helpText = fmt.Sprintf("Suggested: %s -> local port %d (%d matches in view).", suggested.PeerIP, suggested.LocalPort, suggested.Count)
	}
	if filteredPort > 0 {
		defaultPort = strconv.Itoa(filteredPort)
		if hasSuggested {
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
			a.setStatusNote("Invalid peer IP", 5*time.Second)
			return
		}

		port := filteredPort
		if filteredPort == 0 {
			parsedPort, err := strconv.Atoi(strings.TrimSpace(portInput.GetText()))
			if err != nil || parsedPort < 1 || parsedPort > 65535 {
				a.setStatusNote("Invalid local port", 5*time.Second)
				return
			}
			port = parsedPort
		}

		minutes, err := parseBlockMinutes(minutesInput.GetText())
		if err != nil {
			a.setStatusNote(err.Error(), 5*time.Second)
			return
		}

		target := peerKillTarget{
			PeerIP:    peerIP,
			LocalPort: port,
			Count:     a.countPeerMatches(peerIP, port),
		}
		spec := actions.PeerBlockSpec{PeerIP: target.PeerIP, LocalPort: target.LocalPort}
		if !isKillOnlyMinutes(minutes) && a.hasActiveBlock(spec) {
			a.setStatusNote(fmt.Sprintf("Already blocked %s:%d", target.PeerIP, target.LocalPort), 5*time.Second)
			return
		}

		a.pages.RemovePage("kill-peer-form")
		a.updateStatusBar()
		a.promptKillPeerConfirm(target, minutes)
	}

	form.AddButton("Next", func() {
		submit()
	})
	form.AddButton("Cancel", func() {
		a.pages.RemovePage("kill-peer-form")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.exitKillFlowPause()
		a.updateStatusBar()
	})
	form.SetCancelFunc(func() {
		a.pages.RemovePage("kill-peer-form")
		a.app.SetFocus(a.panels[a.focusIndex])
		a.exitKillFlowPause()
		a.updateStatusBar()
	})
	inputCount := 2
	if filteredPort == 0 {
		inputCount = 3
	}
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			a.pages.RemovePage("kill-peer-form")
			a.app.SetFocus(a.panels[a.focusIndex])
			a.exitKillFlowPause()
			a.updateStatusBar()
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
			a.app.SetFocus(form)
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

	a.pages.AddPage("kill-peer-form", modal, true, true)
	a.updateStatusBar()
	form.SetFocus(0)
	a.app.SetFocus(form)
}

func (a *App) promptKillPeerConfirm(target peerKillTarget, minutes int) {
	killOnly := isKillOnlyMinutes(minutes)
	duration := time.Duration(minutes) * time.Minute
	label := "Block " + formatBlockDuration(duration)
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
			"Kill active connections for peer %s -> local port %d?\n\nMatches in current view: %d\nThis terminates active flows only (no block rule or timer).",
			target.PeerIP,
			target.LocalPort,
			target.Count,
		)
		if target.Count == 0 {
			text = fmt.Sprintf(
				"Kill active connections for peer %s -> local port %d?\n\nMatches in current view: 0 (manual target)\nThis terminates active flows only (no block rule or timer).",
				target.PeerIP,
				target.LocalPort,
			)
		}
	}

	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{label, "Cancel"}).
		SetDoneFunc(func(_ int, button string) {
			a.pages.RemovePage("kill-peer")
			a.app.SetFocus(a.panels[a.focusIndex])
			a.exitKillFlowPause()
			a.updateStatusBar()
			if button != label {
				return
			}

			// Snapshot latestTalkers on UI goroutine to avoid data race.
			snapshotTalkers := append([]collector.Connection(nil), a.latestTalkers...)
			if killOnly {
				a.setStatusNote(fmt.Sprintf("Killing %s:%d...", target.PeerIP, target.LocalPort), 4*time.Second)
				go a.killPeerConnectionsOnly(target, snapshotTalkers)
				return
			}

			a.setStatusNote(fmt.Sprintf("Blocking %s:%d...", target.PeerIP, target.LocalPort), 4*time.Second)
			go a.blockPeerForDuration(target, duration, snapshotTalkers)
		})
	modal.SetTitle(" Kill Peer ")
	modal.SetBorder(true)

	a.pages.AddPage("kill-peer", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(modal)
}
