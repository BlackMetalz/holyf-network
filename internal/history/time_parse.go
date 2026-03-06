package history

import (
	"fmt"
	"strings"
	"time"
)

// ParseReplayTime parses replay/jump timestamps in server local timezone semantics.
// Supported formats:
// - RFC3339
// - YYYY-MM-DD HH:MM[:SS]
// - YYYY-MM-DDTHH:MM[:SS]
// - YYYY/MM/DD HH:MM[:SS]
// - HH:MM[:SS] (today)
// - yesterday HH:MM[:SS]
func ParseReplayTime(raw string, now time.Time) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}

	loc := now.Location()
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
	} {
		if ts, err := time.ParseInLocation(layout, trimmed, loc); err == nil {
			return ts, nil
		}
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "yesterday ") {
		clock := strings.TrimSpace(trimmed[len("yesterday "):])
		if ts, err := parseReplayClockOnly(clock, now.AddDate(0, 0, -1)); err == nil {
			return ts, nil
		}
	}

	if ts, err := parseReplayClockOnly(trimmed, now); err == nil {
		return ts, nil
	}

	return time.Time{}, fmt.Errorf("unsupported time format")
}

func parseReplayClockOnly(raw string, base time.Time) (time.Time, error) {
	loc := base.Location()
	clock := strings.TrimSpace(raw)
	if clock == "" {
		return time.Time{}, fmt.Errorf("empty clock")
	}

	for _, layout := range []string{"15:04:05", "15:04"} {
		if parsed, err := time.ParseInLocation(layout, clock, loc); err == nil {
			return time.Date(
				base.Year(),
				base.Month(),
				base.Day(),
				parsed.Hour(),
				parsed.Minute(),
				parsed.Second(),
				0,
				loc,
			), nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid clock")
}
