package blocking

import (
	"fmt"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/actions"
	"github.com/BlackMetalz/holyf-network/internal/collector"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
)

func BlockPeerForDuration(ctx UIContext, m *Manager, target PeerKillTarget, duration time.Duration, snapshotTalkers []collector.Connection) {
	spec := actions.PeerBlockSpec{
		PeerIP:    target.PeerIP,
		LocalPort: target.LocalPort,
	}
	blockedAt := time.Now()
	expiresAt := blockedAt.Add(duration)

	if err := actions.BlockPeer(spec); err != nil {
		ctx.AddActionLog(fmt.Sprintf("block %s:%d failed: %s", spec.PeerIP, spec.LocalPort, tuishared.ShortStatus(err.Error(), 60)))
		ctx.QueueUpdateDraw(func() {
			ctx.SetStatusNote("Block failed: "+tuishared.ShortStatus(err.Error(), 64), 8*time.Second)
		})
		return
	}

	m.AddActiveBlock(ActiveBlockEntry{
		Spec:      spec,
		StartedAt: blockedAt,
		ExpiresAt: expiresAt,
		Summary:   fmt.Sprintf("Blocked %s:%d | processing...", spec.PeerIP, spec.LocalPort),
	})

	tuples := matchingBlockTuplesFromSnapshot(snapshotTalkers, target.PeerIP, target.LocalPort)
	killReport := actions.KillPeerFlows(spec, tuples, actions.DefaultKillConvergeOptions())

	dropWarningParts := make([]string, 0, 2)
	if killReport.SocketErr != nil {
		dropWarningParts = append(dropWarningParts, "socket "+tuishared.ShortStatus(killReport.SocketErr.Error(), 28))
	}
	if killReport.FlowErr != nil {
		dropWarningParts = append(dropWarningParts, "flow "+tuishared.ShortStatus(killReport.FlowErr.Error(), 28))
	}
	dropWarning := ""
	if len(dropWarningParts) > 0 {
		dropWarning = " (drop partial: " + strings.Join(dropWarningParts, "; ") + ")"
	}
	actionSummary := BuildBlockActionSummary(spec, duration, killReport)
	m.UpdateActiveBlockSummary(spec, actionSummary)
	ctx.AddActionLog(actionSummary)

	ctx.QueueUpdateDraw(func() {
		ctx.SetStatusNote(fmt.Sprintf("Blocked %s:%d for %s%s", target.PeerIP, target.LocalPort, FormatBlockDuration(duration), dropWarning), 8*time.Second)
		ShowBlockSummaryPopup(ctx, actionSummary)
		ctx.RefreshData()
	})

	// Wait for expiration... (we handle this in a persistent goroutine elsewhere ideally, but since this was in app_blocking_runtime, we'll keep the sleep structure)
	// TODO: Replace with a block-manager timer, doing it here relies on the app stopChan
	// For now, sleep:
	time.Sleep(duration)

	if !m.HasActiveBlock(spec) {
		return
	}

	if err := actions.UnblockPeer(spec); err != nil {
		ctx.AddActionLog(fmt.Sprintf("auto-unblock %s:%d failed: %s", spec.PeerIP, spec.LocalPort, tuishared.ShortStatus(err.Error(), 60)))
		ctx.QueueUpdateDraw(func() {
			ctx.SetStatusNote("Auto-unblock failed: "+tuishared.ShortStatus(err.Error(), 64), 8*time.Second)
		})
		return
	}
	m.RemoveActiveBlock(spec)
	ctx.AddActionLog(fmt.Sprintf("auto-unblocked %s:%d (expired)", spec.PeerIP, spec.LocalPort))

	ctx.QueueUpdateDraw(func() {
		ctx.SetStatusNote(fmt.Sprintf("Unblocked %s:%d", target.PeerIP, target.LocalPort), 6*time.Second)
		ctx.RefreshData()
	})
}

func KillPeerConnectionsOnly(ctx UIContext, target PeerKillTarget, snapshotTalkers []collector.Connection) {
	spec := actions.PeerBlockSpec{
		PeerIP:    target.PeerIP,
		LocalPort: target.LocalPort,
	}

	tuples := matchingBlockTuplesFromSnapshot(snapshotTalkers, target.PeerIP, target.LocalPort)
	killReport := actions.KillPeerFlows(spec, tuples, actions.DefaultKillConvergeOptions())

	dropWarningParts := make([]string, 0, 2)
	if killReport.SocketErr != nil {
		dropWarningParts = append(dropWarningParts, "socket "+tuishared.ShortStatus(killReport.SocketErr.Error(), 28))
	}
	if killReport.FlowErr != nil {
		dropWarningParts = append(dropWarningParts, "flow "+tuishared.ShortStatus(killReport.FlowErr.Error(), 28))
	}
	dropWarning := ""
	if len(dropWarningParts) > 0 {
		dropWarning = " (drop partial: " + strings.Join(dropWarningParts, "; ") + ")"
	}
	actionSummary := BuildKillOnlyActionSummary(spec, killReport)
	ctx.AddActionLog(actionSummary)

	ctx.QueueUpdateDraw(func() {
		ctx.SetStatusNote(fmt.Sprintf("Killed connections for %s:%d%s", target.PeerIP, target.LocalPort, dropWarning), 8*time.Second)
		ShowBlockSummaryPopup(ctx, actionSummary)
		ctx.RefreshData()
	})
}

func matchingBlockTuplesFromSnapshot(conns []collector.Connection, peerIP string, localPort int) []actions.SocketTuple {
	if len(conns) == 0 {
		return nil
	}

	normalizedPeer := tuishared.NormalizeIP(peerIP)
	seen := make(map[string]struct{})
	tuples := make([]actions.SocketTuple, 0, 8)

	for _, conn := range conns {
		if conn.LocalPort != localPort {
			continue
		}
		if !strings.EqualFold(conn.State, "ESTABLISHED") {
			continue
		}

		remoteIP := tuishared.NormalizeIP(conn.RemoteIP)
		if remoteIP != normalizedPeer {
			continue
		}

		localIP := tuishared.NormalizeIP(conn.LocalIP)
		if localIP == "" || conn.RemotePort < 1 || conn.RemotePort > 65535 {
			continue
		}

		key := fmt.Sprintf("%s|%d|%s|%d", localIP, conn.LocalPort, remoteIP, conn.RemotePort)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		tuples = append(tuples, actions.SocketTuple{
			LocalIP:    localIP,
			LocalPort:  conn.LocalPort,
			RemoteIP:   remoteIP,
			RemotePort: conn.RemotePort,
		})
	}

	return tuples
}

const (
	defaultBlockMinutes = 5
	maxBlockMinutes     = 1440
)

func BuildBlockActionSummary(
	spec actions.PeerBlockSpec,
	duration time.Duration,
	report actions.KillConvergeReport,
) string {
	killPart, remainingPart := BuildKillPart(report)

	dropPart := "drop ok"
	if report.SocketErr != nil && report.FlowErr != nil {
		dropPart = "drop partial"
	}

	parts := []string{
		fmt.Sprintf("Blocked %s:%d", spec.PeerIP, spec.LocalPort),
		killPart,
	}
	if remainingPart != "" {
		parts = append(parts, remainingPart)
	}
	parts = append(parts, dropPart, "expires in "+FormatBlockDuration(duration))
	return strings.Join(parts, " | ")
}

func BuildKillOnlyActionSummary(
	spec actions.PeerBlockSpec,
	report actions.KillConvergeReport,
) string {
	killPart, remainingPart := BuildKillPart(report)

	dropPart := "drop ok"
	if report.SocketErr != nil && report.FlowErr != nil {
		dropPart = "drop partial"
	}

	parts := []string{
		fmt.Sprintf("Killed connections for %s:%d", spec.PeerIP, spec.LocalPort),
		killPart,
	}
	if remainingPart != "" {
		parts = append(parts, remainingPart)
	}
	parts = append(parts, dropPart)
	return strings.Join(parts, " | ")
}

func BuildKillPart(report actions.KillConvergeReport) (killPart string, remainingPart string) {
	killPart = "killed ?/? flows"
	if report.BeforeCountErr == nil && report.AfterCountErr == nil {
		beforeCount := report.BeforeActiveCount
		afterCount := report.AfterActiveCount
		if beforeCount < 0 {
			beforeCount = 0
		}
		if afterCount < 0 {
			afterCount = 0
		}
		if afterCount > beforeCount {
			afterCount = beforeCount
		}
		killPart = fmt.Sprintf("killed %d/%d flows", beforeCount-afterCount, beforeCount)
		if report.IsPartial() {
			remainingPart = fmt.Sprintf("remaining %d (storm/race)", afterCount)
		}
	} else if report.BeforeCountErr == nil {
		beforeCount := report.BeforeActiveCount
		if beforeCount < 0 {
			beforeCount = 0
		}
		killPart = fmt.Sprintf("killed ?/%d flows", beforeCount)
	}
	return killPart, remainingPart
}

func FormatBlockDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	if minutes == 0 {
		return "kill-only"
	}
	if minutes == 1 {
		return "1 min"
	}
	if minutes < 60 {
		return fmt.Sprintf("%d mins", minutes)
	}
	hours := float64(d.Minutes()) / 60.0
	return fmt.Sprintf("%.1fh", hours)
}
