package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ThresholdBand defines warning/critical cutoffs for a metric.
type ThresholdBand struct {
	Warn float64
	Crit float64
}

// HealthThresholds controls the health strip severity coloring.
type HealthThresholds struct {
	RetransPercent   ThresholdBand
	DropsPerSec      ThresholdBand
	ConntrackPercent ThresholdBand
}

// DefaultHealthThresholds returns sane defaults for server monitoring.
func DefaultHealthThresholds() HealthThresholds {
	return HealthThresholds{
		RetransPercent: ThresholdBand{
			Warn: 2.0,
			Crit: 5.0,
		},
		DropsPerSec: ThresholdBand{
			Warn: 10.0,
			Crit: 50.0,
		},
		ConntrackPercent: ThresholdBand{
			Warn: 70.0,
			Crit: 85.0,
		},
	}
}

// Normalize fixes invalid values while preserving user intent.
func (h *HealthThresholds) Normalize() {
	normalizeBand(&h.RetransPercent)
	normalizeBand(&h.DropsPerSec)
	normalizeBand(&h.ConntrackPercent)
}

func normalizeBand(b *ThresholdBand) {
	if b.Warn < 0 {
		b.Warn = 0
	}
	if b.Crit < 0 {
		b.Crit = 0
	}
	if b.Crit > 0 && b.Warn > b.Crit {
		b.Warn, b.Crit = b.Crit, b.Warn
	}
	if b.Crit == 0 {
		b.Crit = b.Warn
	}
}

// LoadHealthThresholds reads a small TOML-style config file.
//
// Supported sections:
//
//	[retrans_percent]
//	[drops_per_sec]
//	[conntrack_percent]
//
// Supported keys in each section: warn, crit
func LoadHealthThresholds(path string) (HealthThresholds, error) {
	thresholds := DefaultHealthThresholds()
	if strings.TrimSpace(path) == "" {
		return thresholds, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return thresholds, err
	}
	defer file.Close()

	section := ""
	lineNo := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNo++
		line := stripComment(strings.TrimSpace(scanner.Text()))
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return thresholds, fmt.Errorf("%s:%d invalid section syntax", path, lineNo)
			}
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return thresholds, fmt.Errorf("%s:%d invalid key=value", path, lineNo)
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		raw := strings.TrimSpace(parts[1])
		raw = strings.Trim(raw, "\"'")

		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return thresholds, fmt.Errorf("%s:%d invalid number %q", path, lineNo, raw)
		}

		band := thresholdBandBySection(&thresholds, section)
		if band == nil {
			continue
		}

		switch key {
		case "warn":
			band.Warn = value
		case "crit":
			band.Crit = value
		}
	}
	if err := scanner.Err(); err != nil {
		return thresholds, err
	}

	thresholds.Normalize()
	return thresholds, nil
}

func thresholdBandBySection(thresholds *HealthThresholds, section string) *ThresholdBand {
	switch section {
	case "retrans_percent":
		return &thresholds.RetransPercent
	case "drops_per_sec":
		return &thresholds.DropsPerSec
	case "conntrack_percent":
		return &thresholds.ConntrackPercent
	default:
		return nil
	}
}

func stripComment(line string) string {
	inQuote := false
	var quote byte

	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch == '"' || ch == '\'' {
			if inQuote && ch == quote {
				inQuote = false
				quote = 0
			} else if !inQuote {
				inQuote = true
				quote = ch
			}
			continue
		}
		if !inQuote && (ch == '#' || ch == ';') {
			return strings.TrimSpace(line[:i])
		}
	}
	return strings.TrimSpace(line)
}
