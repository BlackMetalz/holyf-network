package tui

import (
	"fmt"
	"strings"
	"time"
)

const (
	defaultBlockMinutes = 10
	maxBlockMinutes     = 1440

	actionLogModalLimit      = 20
	inMemoryActionLogMax     = 500
	actionLogRotateLimit     = 500
	traceHistoryModalLimit   = 20
	diagnosisHistoryLimit    = 20
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
