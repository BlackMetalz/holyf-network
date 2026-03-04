package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadHealthThresholdsRetransSampleOverrides(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "health.toml")
	content := `
[retrans_percent]
warn = 3
crit = 7

[retrans_sample]
min_established = 30
min_out_segs_per_sec = 90
`

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	thresholds, err := LoadHealthThresholds(path)
	if err != nil {
		t.Fatalf("load thresholds: %v", err)
	}

	if thresholds.RetransPercent.Warn != 3 {
		t.Fatalf("warn mismatch: got=%v want=3", thresholds.RetransPercent.Warn)
	}
	if thresholds.RetransPercent.Crit != 7 {
		t.Fatalf("crit mismatch: got=%v want=7", thresholds.RetransPercent.Crit)
	}
	if thresholds.RetransMinEstablished != 30 {
		t.Fatalf("min_established mismatch: got=%d want=30", thresholds.RetransMinEstablished)
	}
	if thresholds.RetransMinOutSegsPerSec != 90 {
		t.Fatalf("min_out_segs_per_sec mismatch: got=%v want=90", thresholds.RetransMinOutSegsPerSec)
	}
}

func TestHealthThresholdsNormalizeRetransSample(t *testing.T) {
	t.Parallel()

	thresholds := HealthThresholds{
		RetransMinEstablished:   -5,
		RetransMinOutSegsPerSec: -10,
	}
	thresholds.Normalize()

	if thresholds.RetransMinEstablished != 0 {
		t.Fatalf("min_established should clamp to 0, got=%d", thresholds.RetransMinEstablished)
	}
	if thresholds.RetransMinOutSegsPerSec != 0 {
		t.Fatalf("min_out_segs_per_sec should clamp to 0, got=%v", thresholds.RetransMinOutSegsPerSec)
	}
}
