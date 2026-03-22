package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestAppendTraceHistoryPersistsDailySegments(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	a := &App{
		traceHistory:        make([]traceHistoryEntry, 0, 32),
		traceHistoryDataDir: dataDir,
		traceHistoryLoaded:  true,
	}

	base := time.Date(2026, 3, 22, 10, 0, 0, 0, time.Local)
	for i := 0; i < 3; i++ {
		result := tracePacketResult{
			Request: tracePacketRequest{
				Interface:   "eth0",
				PeerIP:      "203.0.113.10",
				Port:        443,
				Scope:       traceScopePeerPort,
				Direction:   traceDirectionIn,
				DurationSec: 10,
				PacketCap:   2000,
			},
			Filter:         "tcp and host 203.0.113.10 and port 443",
			StartedAt:      base.AddDate(0, 0, i).Add(-10 * time.Second),
			EndedAt:        base.AddDate(0, 0, i),
			Captured:       20,
			DecodedPackets: 20,
			SynCount:       4,
			SynAckCount:    4,
			RstCount:       0,
		}
		a.appendTraceHistory(result)
	}

	wantFiles := []string{
		traceHistorySegmentFileName(base),
		traceHistorySegmentFileName(base.AddDate(0, 0, 1)),
		traceHistorySegmentFileName(base.AddDate(0, 0, 2)),
	}
	for _, name := range wantFiles {
		if _, err := os.Stat(filepath.Join(dataDir, name)); err != nil {
			t.Fatalf("expected segment file %s: %v", name, err)
		}
	}

	loaded, err := readTraceHistoryEntriesFromDir(dataDir)
	if err != nil {
		t.Fatalf("read trace history dir: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 loaded entries, got=%d", len(loaded))
	}
	recent := a.recentTraceHistory(traceHistoryModalLimit)
	if len(recent) != 3 {
		t.Fatalf("recent trace history count mismatch: got=%d want=3", len(recent))
	}
	if !recent[0].CapturedAt.After(recent[len(recent)-1].CapturedAt) {
		t.Fatalf("expected recent entries in newest-first order")
	}
}

func TestPruneTraceHistoryDataDirByAge(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.Local)
	old := traceHistorySegmentFileName(now.AddDate(0, 0, -10))
	cur := traceHistorySegmentFileName(now)
	if err := os.WriteFile(filepath.Join(dir, old), []byte("{\"captured_at\":\"2026-03-20T10:00:00Z\"}\n"), 0o600); err != nil {
		t.Fatalf("write old segment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, cur), []byte("{\"captured_at\":\"2026-03-30T10:00:00Z\"}\n"), 0o600); err != nil {
		t.Fatalf("write current segment: %v", err)
	}

	if err := pruneTraceHistoryDataDirByAge(dir, 24*7, now); err != nil {
		t.Fatalf("prune trace history: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, old)); err == nil {
		t.Fatalf("old segment should be pruned")
	}
	if _, err := os.Stat(filepath.Join(dir, cur)); err != nil {
		t.Fatalf("current segment should be kept: %v", err)
	}
}

func TestHandleKeyEventTOpensTraceHistoryModal(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	ret := a.handleKeyEvent(tcell.NewEventKey(tcell.KeyRune, 't', 0))
	if ret != nil {
		t.Fatalf("t should be handled")
	}
	name, _ := a.pages.GetFrontPage()
	if name != traceHistoryPage {
		t.Fatalf("expected %q page, got %q", traceHistoryPage, name)
	}
}

func TestBuildTraceHistoryDetailTextMasksSensitiveIP(t *testing.T) {
	t.Parallel()

	entry := traceHistoryEntry{
		CapturedAt:       time.Date(2026, 3, 22, 10, 1, 2, 0, time.UTC),
		Interface:        "eth0",
		PeerIP:           "203.0.113.10",
		Port:             443,
		Scope:            "Peer + Port",
		Preset:           "Peer + Port",
		Direction:        "IN",
		DurationSec:      10,
		PacketCap:        2000,
		Filter:           "tcp and host 203.0.113.10 and port 443",
		Status:           "completed",
		Saved:            true,
		PCAPPath:         "/tmp/holyf-network-captures/trace-20260322-100102-203_0_113_10-443.pcap",
		Captured:         12,
		ReceivedByFilter: 12,
		DroppedByKernel:  0,
		DecodedPackets:   12,
		SynCount:         2,
		SynAckCount:      2,
		RstCount:         0,
		Severity:         "INFO",
		Confidence:       "LOW",
		Issue:            "No strong packet-level anomaly",
		Signal:           "Decoded 12 | SYN 2 | SYN-ACK 2 | RST 0 | Drop 0",
		Likely:           "stable sample",
		Check:            "correlate panels",
		Sample:           []string{"IP 203.0.113.10.443 > 172.25.110.116.22: Flags [S], seq 1"},
	}

	text := buildTraceHistoryDetailText(entry, true)
	if strings.Contains(text, "203.0.113.10") || strings.Contains(text, "172.25.110.116") {
		t.Fatalf("expected masked ips in detail text, got=%q", text)
	}
	if !strings.Contains(text, "Trace Analyzer") {
		t.Fatalf("expected Trace Analyzer section, got=%q", text)
	}
	if !strings.Contains(text, "Profile: [green]General triage[white]") {
		t.Fatalf("expected profile line in detail text, got=%q", text)
	}
	if !strings.Contains(text, "Category: [green]Peer + Port[white]") {
		t.Fatalf("expected category line in detail text, got=%q", text)
	}
	if !strings.Contains(text, "trace-20260322-100102-masked.pcap") {
		t.Fatalf("expected masked pcap display path, got=%q", text)
	}
}

func TestTraceHistoryCategoryFallbackFromScope(t *testing.T) {
	t.Parallel()

	if got := traceHistoryCategory(traceHistoryEntry{Preset: "SYN/RST only"}); got != "SYN/RST only" {
		t.Fatalf("expected explicit preset category, got=%q", got)
	}
	if got := traceHistoryCategory(traceHistoryEntry{Scope: "5-tuple"}); got != "5-tuple" {
		t.Fatalf("expected 5-tuple fallback, got=%q", got)
	}
	if got := traceHistoryCategory(traceHistoryEntry{Scope: "Custom (Peer+Port)"}); got != "Custom" {
		t.Fatalf("expected custom fallback, got=%q", got)
	}
	if got := traceHistoryCategory(traceHistoryEntry{Scope: "Peer only"}); got != "Peer only" {
		t.Fatalf("expected peer-only fallback, got=%q", got)
	}
	if got := traceHistoryCategory(traceHistoryEntry{}); got != "Peer + Port" {
		t.Fatalf("expected default category, got=%q", got)
	}
}

func TestBuildTraceHistoryCompareTextShowsRequestedDiffs(t *testing.T) {
	t.Parallel()

	baseline := traceHistoryEntry{
		CapturedAt:        time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC),
		PeerIP:            "203.0.113.10",
		Port:              443,
		Preset:            "Peer + Port",
		DecodedPackets:    100,
		ReceivedByFilter:  100,
		DroppedByKernel:   1,
		SynCount:          20,
		SynAckCount:       18,
		RstCount:          2,
		Severity:          "INFO",
		Issue:             "baseline stable",
		Captured:          100,
		CapturedEstimated: false,
	}
	incident := traceHistoryEntry{
		CapturedAt:        time.Date(2026, 3, 22, 10, 5, 0, 0, time.UTC),
		PeerIP:            "203.0.113.10",
		Port:              443,
		Preset:            "SYN/RST only",
		DecodedPackets:    120,
		ReceivedByFilter:  120,
		DroppedByKernel:   12,
		SynCount:          30,
		SynAckCount:       12,
		RstCount:          24,
		Severity:          "WARN",
		Issue:             "incident unstable",
		Captured:          120,
		CapturedEstimated: false,
	}

	text := buildTraceHistoryCompareText(baseline, incident, true)
	for _, want := range []string{
		"Trace Compare (Baseline vs Incident)",
		"Drop ratio:",
		"RST ratio:",
		"SYN-ACK ratio:",
		"Top Flags Changed",
		"RST:",
		"SYN-ACK:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected compare text to contain %q, got=%q", want, text)
		}
	}
}

func TestShowTraceHistoryCompareModalOpensComparePage(t *testing.T) {
	t.Parallel()

	a := newPhase3TestApp()
	baseline := traceHistoryEntry{
		CapturedAt:       time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC),
		PeerIP:           "203.0.113.10",
		Port:             443,
		DecodedPackets:   100,
		ReceivedByFilter: 100,
		DroppedByKernel:  1,
		SynCount:         20,
		SynAckCount:      18,
		RstCount:         2,
	}
	incident := traceHistoryEntry{
		CapturedAt:       time.Date(2026, 3, 22, 10, 5, 0, 0, time.UTC),
		PeerIP:           "203.0.113.10",
		Port:             443,
		DecodedPackets:   120,
		ReceivedByFilter: 120,
		DroppedByKernel:  12,
		SynCount:         30,
		SynAckCount:      12,
		RstCount:         24,
	}

	a.showTraceHistoryCompareModal(baseline, incident, nil)
	name, _ := a.pages.GetFrontPage()
	if name != traceHistoryComparePage {
		t.Fatalf("expected %q page, got %q", traceHistoryComparePage, name)
	}
}
