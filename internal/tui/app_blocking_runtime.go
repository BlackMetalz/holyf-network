package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/actions"
	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/rivo/tview"
)

// blockPeerForDuration runs in a background goroutine. snapshotTalkers is a
// copy of latestTalkers captured on the UI goroutine to avoid a data race.
func (a *App) blockPeerForDuration(target peerKillTarget, duration time.Duration, snapshotTalkers []collector.Connection) {
	spec := actions.PeerBlockSpec{
		PeerIP:    target.PeerIP,
		LocalPort: target.LocalPort,
	}
	blockedAt := time.Now()
	expiresAt := blockedAt.Add(duration)

	if err := actions.BlockPeer(spec); err != nil {
		a.addActionLog(fmt.Sprintf("block %s:%d failed: %s", spec.PeerIP, spec.LocalPort, shortStatus(err.Error(), 60)))
		a.app.QueueUpdateDraw(func() {
			a.setStatusNote("Block failed: "+shortStatus(err.Error(), 64), 8*time.Second)
		})
		return
	}

	a.addActiveBlock(activeBlockEntry{
		Spec:      spec,
		StartedAt: blockedAt,
		ExpiresAt: expiresAt,
		Summary:   fmt.Sprintf("Blocked %s:%d | processing...", spec.PeerIP, spec.LocalPort),
	})

	tuples := matchingBlockTuplesFromSnapshot(snapshotTalkers, target.PeerIP, target.LocalPort)
	killReport := actions.KillPeerFlows(spec, tuples, actions.DefaultKillConvergeOptions())

	dropWarningParts := make([]string, 0, 2)
	if killReport.SocketErr != nil {
		dropWarningParts = append(dropWarningParts, "socket "+shortStatus(killReport.SocketErr.Error(), 28))
	}
	if killReport.FlowErr != nil {
		dropWarningParts = append(dropWarningParts, "flow "+shortStatus(killReport.FlowErr.Error(), 28))
	}
	dropWarning := ""
	if len(dropWarningParts) > 0 {
		dropWarning = " (drop partial: " + strings.Join(dropWarningParts, "; ") + ")"
	}
	actionSummary := buildBlockActionSummary(spec, duration, killReport)
	a.updateActiveBlockSummary(spec, actionSummary)
	a.addActionLog(actionSummary)

	a.app.QueueUpdateDraw(func() {
		a.setStatusNote(fmt.Sprintf("Blocked %s:%d for %s%s", target.PeerIP, target.LocalPort, formatBlockDuration(duration), dropWarning), 8*time.Second)
		a.showBlockSummaryPopup(actionSummary)
		a.refreshData()
	})

	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-a.stopChan:
		return
	}
	if !a.hasActiveBlock(spec) {
		return
	}

	if err := actions.UnblockPeer(spec); err != nil {
		a.addActionLog(fmt.Sprintf("auto-unblock %s:%d failed: %s", spec.PeerIP, spec.LocalPort, shortStatus(err.Error(), 60)))
		a.app.QueueUpdateDraw(func() {
			a.setStatusNote("Auto-unblock failed: "+shortStatus(err.Error(), 64), 8*time.Second)
		})
		return
	}
	a.removeActiveBlock(spec)
	a.addActionLog(fmt.Sprintf("auto-unblocked %s:%d (expired)", spec.PeerIP, spec.LocalPort))

	a.app.QueueUpdateDraw(func() {
		a.setStatusNote(fmt.Sprintf("Unblocked %s:%d", target.PeerIP, target.LocalPort), 6*time.Second)
		a.refreshData()
	})
}

func (a *App) killPeerConnectionsOnly(target peerKillTarget, snapshotTalkers []collector.Connection) {
	spec := actions.PeerBlockSpec{
		PeerIP:    target.PeerIP,
		LocalPort: target.LocalPort,
	}

	tuples := matchingBlockTuplesFromSnapshot(snapshotTalkers, target.PeerIP, target.LocalPort)
	killReport := actions.KillPeerFlows(spec, tuples, actions.DefaultKillConvergeOptions())

	dropWarningParts := make([]string, 0, 2)
	if killReport.SocketErr != nil {
		dropWarningParts = append(dropWarningParts, "socket "+shortStatus(killReport.SocketErr.Error(), 28))
	}
	if killReport.FlowErr != nil {
		dropWarningParts = append(dropWarningParts, "flow "+shortStatus(killReport.FlowErr.Error(), 28))
	}
	dropWarning := ""
	if len(dropWarningParts) > 0 {
		dropWarning = " (drop partial: " + strings.Join(dropWarningParts, "; ") + ")"
	}
	actionSummary := buildKillOnlyActionSummary(spec, killReport)
	a.addActionLog(actionSummary)

	a.app.QueueUpdateDraw(func() {
		a.setStatusNote(fmt.Sprintf("Killed connections for %s:%d%s", target.PeerIP, target.LocalPort, dropWarning), 8*time.Second)
		a.showBlockSummaryPopup(actionSummary)
		a.refreshData()
	})
}

func (a *App) showBlockSummaryPopup(summary string) {
	modal := tview.NewModal().
		SetText(summary).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(_ int, _ string) {
			a.pages.RemovePage("block-summary")
			a.updateStatusBar()
			a.app.SetFocus(a.panels[a.focusIndex])
		})
	modal.SetTitle(" Block Summary ")
	modal.SetBorder(true)

	a.pages.RemovePage("block-summary")
	a.pages.AddPage("block-summary", modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(modal)
}
