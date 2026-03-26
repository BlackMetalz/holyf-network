package trace

import (
	"strings"
	"testing"
	"time"
)

func testRenderOptions() RenderOptions {
	return RenderOptions{
		SensitiveIP:       true,
		SeverityInfo:      "INFO",
		FormatPreviewIP:   func(ip string, _ bool) string { return "masked:" + ip },
		MaskSensitiveText: func(s string, _ bool) string { return strings.ReplaceAll(s, "203.0.113.10", "masked") },
		ShortStatus:       func(s string, _ int) string { return s },
		MaskPath:          func(s string, _ bool) string { return strings.ReplaceAll(s, "203.0.113.10", "masked") },
		MetricDisplay: func(v int, est bool) string {
			if est {
				return "est"
			}
			return "val"
		},
		MetricValue:      func(v int) string { return "mv" },
		SeverityStyled:   func(s string) string { return s },
		ConfidenceStyled: func(s string) string { return s },
	}
}

func TestCategoryFallbackFromScope(t *testing.T) {
	if got := Category(Entry{Preset: "SYN/RST only"}); got != "SYN/RST only" {
		t.Fatalf("got %q", got)
	}
	if got := Category(Entry{Scope: "5-tuple"}); got != "5-tuple" {
		t.Fatalf("got %q", got)
	}
	if got := Category(Entry{Scope: "Custom (Peer+Port)"}); got != "Custom" {
		t.Fatalf("got %q", got)
	}
	if got := Category(Entry{Scope: "Peer only"}); got != "Peer only" {
		t.Fatalf("got %q", got)
	}
	if got := Category(Entry{}); got != "Peer + Port" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildCompareTextShowsRequestedDiffs(t *testing.T) {
	baseline := Entry{CapturedAt: time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC), PeerIP: "203.0.113.10", Port: 443, Preset: "Peer + Port", DecodedPackets: 100, ReceivedByFilter: 100, DroppedByKernel: 1, SynCount: 20, SynAckCount: 18, RstCount: 2, Captured: 100}
	incident := Entry{CapturedAt: time.Date(2026, 3, 22, 10, 5, 0, 0, time.UTC), PeerIP: "203.0.113.10", Port: 443, Preset: "SYN/RST only", DecodedPackets: 120, ReceivedByFilter: 120, DroppedByKernel: 12, SynCount: 30, SynAckCount: 12, RstCount: 24, Captured: 120}
	text := BuildCompareText(baseline, incident, testRenderOptions())
	for _, want := range []string{"Trace Compare (Baseline vs Incident)", "Drop ratio:", "RST ratio:", "SYN-ACK ratio:", "Top Flags Changed", "RST:", "SYN-ACK:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}
