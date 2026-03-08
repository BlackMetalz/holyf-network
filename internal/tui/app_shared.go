package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/actions"
)

const (
	defaultBlockMinutes = 10
	maxBlockMinutes     = 1440

	actionLogModalLimit      = 20
	inMemoryActionLogMax     = 500
	actionLogRotateLimit     = 500
	actionHistoryDirName     = ".holyf-network"
	actionHistoryFileName    = "history.log"
	actionHistoryDisplayPath = "~/.holyf-network/history.log"
)

func shortStatus(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func formatBlockDuration(duration time.Duration) string {
	minutes := int(duration / time.Minute)
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	seconds := int(duration / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprintf("%ds", seconds)
}

func formatRemainingDuration(duration time.Duration) string {
	if duration <= 0 {
		return "00:00"
	}

	totalSeconds := int(duration.Round(time.Second) / time.Second)
	if totalSeconds < 0 {
		totalSeconds = 0
	}

	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%dh%02dm", hours, minutes)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func buildBlockActionSummary(
	spec actions.PeerBlockSpec,
	duration time.Duration,
	beforeCount int,
	beforeErr error,
	afterCount int,
	afterErr error,
	socketErr error,
	flowErr error,
) string {
	killPart := buildKillPart(beforeCount, beforeErr, afterCount, afterErr)

	dropPart := "drop ok"
	if socketErr != nil && flowErr != nil {
		dropPart = "drop partial"
	}

	return fmt.Sprintf(
		"Blocked %s:%d | %s | %s | expires in %s",
		spec.PeerIP,
		spec.LocalPort,
		killPart,
		dropPart,
		formatRemainingDuration(duration),
	)
}

func buildKillOnlyActionSummary(
	spec actions.PeerBlockSpec,
	beforeCount int,
	beforeErr error,
	afterCount int,
	afterErr error,
	socketErr error,
	flowErr error,
) string {
	killPart := buildKillPart(beforeCount, beforeErr, afterCount, afterErr)

	dropPart := "drop ok"
	if socketErr != nil && flowErr != nil {
		dropPart = "drop partial"
	}

	return fmt.Sprintf(
		"Killed connections for %s:%d | %s | %s",
		spec.PeerIP,
		spec.LocalPort,
		killPart,
		dropPart,
	)
}

func buildKillPart(beforeCount int, beforeErr error, afterCount int, afterErr error) string {
	killPart := "killed ?/? flows"
	if beforeErr == nil && afterErr == nil {
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
	} else if beforeErr == nil {
		killPart = fmt.Sprintf("killed ?/%d flows", beforeCount)
	}
	return killPart
}

func formatActiveBlockDetail(entry activeBlockEntry) string {
	summary := strings.TrimSpace(entry.Summary)
	if summary == "" {
		summary = fmt.Sprintf("Blocked %s:%d", entry.Spec.PeerIP, entry.Spec.LocalPort)
	}
	expiresText := formatRemainingDuration(time.Until(entry.ExpiresAt))
	if entry.ExpiresAt.IsZero() {
		expiresText = "n/a (unmanaged)"
	}
	return fmt.Sprintf("[dim]Summary:[white] %s\n[dim]Expires in:[white] %s", summary, expiresText)
}

func formatBlockedSpec(spec actions.PeerBlockSpec) string {
	return fmt.Sprintf("%s -> :%d", spec.PeerIP, spec.LocalPort)
}

func formatBlockedListSecondary(entry activeBlockEntry) string {
	secondary := "drop unknown | expires in n/a"
	summary := strings.TrimSpace(entry.Summary)
	if summary != "" {
		parts := strings.Split(summary, " | ")
		if len(parts) > 1 {
			secondary = strings.Join(parts[1:], " | ")
		}
	}

	expires := "n/a"
	if !entry.ExpiresAt.IsZero() {
		expires = formatRemainingDuration(time.Until(entry.ExpiresAt))
	}
	if strings.Contains(secondary, "expires in ") {
		idx := strings.LastIndex(secondary, "expires in ")
		secondary = secondary[:idx] + "expires in " + expires
	} else {
		secondary = secondary + " | expires in " + expires
	}
	return secondary
}

func sanitizeActionLogMessage(message string) string {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return ""
	}
	if !strings.HasPrefix(msg, "Blocked ") {
		return msg
	}

	parts := strings.Split(msg, " | ")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "expires in ") {
			continue
		}
		if p == "killed 0/0 flows" {
			continue
		}
		filtered = append(filtered, p)
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, " | ")
}
