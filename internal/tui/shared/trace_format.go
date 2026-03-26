package shared

import (
	"regexp"
	"strings"
	"time"

	tuitrace "github.com/BlackMetalz/holyf-network/internal/tui/trace"
)

func ShortStatus(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func FormatApproxDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d >= time.Hour {
		return d.Round(time.Minute).String()
	}
	if d >= time.Minute {
		return d.Round(time.Second).String()
	}
	return d.Round(100 * time.Millisecond).String()
}

func BlankIfUnknown(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func TracePacketSeverityColor(severity string) string {
	return tuitrace.SeverityColor(severity, "INFO")
}

func TraceHistoryCategory(entry tuitrace.Entry) string {
	return tuitrace.Category(entry)
}

func TraceHistoryPortLabel(port int) string {
	return tuitrace.PortLabel(port)
}

var (
	traceIPv4Regex = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	traceIPv6Regex = regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){2,}[0-9a-fA-F:.]*[0-9a-fA-F]{1,4}\b`)
)

func MaskSensitiveIPsInText(raw string, sensitiveIP bool) string {
	if !sensitiveIP || strings.TrimSpace(raw) == "" {
		return raw
	}
	out := traceIPv4Regex.ReplaceAllStringFunc(raw, func(token string) string {
		return MaskIP(token) // MaskIP handles basic IPv4 structure validation gracefully enough for display
	})
	out = traceIPv6Regex.ReplaceAllStringFunc(out, func(token string) string {
		return MaskIP(token)
	})
	return out
}
