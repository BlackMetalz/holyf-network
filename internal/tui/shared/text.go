package shared

import (
	"fmt"
	"strings"
)

func NormalizeIP(ip string) string {
	return strings.TrimPrefix(ip, "::ffff:")
}

func MaskIP(ip string) string {
	if strings.Count(ip, ".") == 3 {
		parts := strings.Split(ip, ".")
		return fmt.Sprintf("xxx.xxx.%s.%s", parts[2], parts[3])
	}

	if strings.Contains(ip, ":") {
		parts := strings.Split(ip, ":")
		masked := 0
		for i := 0; i < len(parts) && masked < 2; i++ {
			if parts[i] == "" {
				continue
			}
			parts[i] = "xxxx"
			masked++
		}
		return strings.Join(parts, ":")
	}

	return ip
}

func FormatPreviewIP(ip string, sensitiveIP bool) string {
	displayIP := NormalizeIP(ip)
	if sensitiveIP {
		displayIP = MaskIP(displayIP)
	}
	return displayIP
}

func FormatPreviewEndpoint(ip string, port int, sensitiveIP bool) string {
	displayIP := FormatPreviewIP(ip, sensitiveIP)
	if strings.Contains(displayIP, ":") && !strings.Contains(displayIP, ".") {
		return fmt.Sprintf("[%s]:%d", displayIP, port)
	}
	return fmt.Sprintf("%s:%d", displayIP, port)
}

func TruncateEndpoint(endpoint string, width int) string {
	if len(endpoint) <= width {
		return endpoint
	}
	if width <= 3 {
		return endpoint[:width]
	}

	suffix := ""
	if idx := strings.LastIndex(endpoint, ":"); idx >= 0 {
		suffix = endpoint[idx:]
	}
	if suffix == "" || len(suffix) >= width-3 {
		return endpoint[:width-3] + "..."
	}

	prefixLen := width - len(suffix) - 3
	if prefixLen < 1 {
		prefixLen = 1
	}

	return endpoint[:prefixLen] + "..." + suffix
}

func TruncateRight(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}
