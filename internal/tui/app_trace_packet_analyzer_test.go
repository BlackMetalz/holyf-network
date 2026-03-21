package tui

import (
	"errors"
	"strings"
	"testing"
)

func TestAnalyzeTracePacketCaptureErrorIsCritical(t *testing.T) {
	t.Parallel()

	diag := analyzeTracePacket(tracePacketResult{
		DecodedPackets: 20,
		CaptureErr:     errors.New("permission denied"),
	})
	if diag.Severity != traceSeverityCrit {
		t.Fatalf("expected CRIT severity, got %q", diag.Severity)
	}
	if diag.Confidence != "HIGH" {
		t.Fatalf("expected HIGH confidence on capture error, got %q", diag.Confidence)
	}
	if !strings.Contains(diag.Issue, "Capture failed") {
		t.Fatalf("unexpected issue: %q", diag.Issue)
	}
}

func TestAnalyzeTracePacketDropsCanEscalateToCrit(t *testing.T) {
	t.Parallel()

	diag := analyzeTracePacket(tracePacketResult{
		Captured:        90,
		DecodedPackets:  90,
		DroppedByKernel: 10,
	})
	if diag.Severity != traceSeverityCrit {
		t.Fatalf("expected CRIT severity for 10%% drop ratio, got %q", diag.Severity)
	}
	if !strings.Contains(diag.Signal, "drop ratio 10.0%") {
		t.Fatalf("unexpected signal: %q", diag.Signal)
	}
}

func TestAnalyzeTracePacketRstPressureWarn(t *testing.T) {
	t.Parallel()

	diag := analyzeTracePacket(tracePacketResult{
		DecodedPackets: 20,
		RstCount:       5,
	})
	if diag.Severity != traceSeverityWarn {
		t.Fatalf("expected WARN severity for rst pressure, got %q", diag.Severity)
	}
	if diag.Issue != "RST pressure" {
		t.Fatalf("unexpected issue: %q", diag.Issue)
	}
}

func TestAnalyzeTracePacketSynWithoutSynAck(t *testing.T) {
	t.Parallel()

	diag := analyzeTracePacket(tracePacketResult{
		DecodedPackets: 30,
		SynCount:       6,
		SynAckCount:    0,
	})
	if diag.Severity != traceSeverityWarn {
		t.Fatalf("expected WARN severity for missing syn-ack, got %q", diag.Severity)
	}
	if diag.Issue != "SYN seen but no SYN-ACK" {
		t.Fatalf("unexpected issue: %q", diag.Issue)
	}
}

func TestAnalyzeTracePacketLowSample(t *testing.T) {
	t.Parallel()

	diag := analyzeTracePacket(tracePacketResult{
		DecodedPackets: 8,
	})
	if diag.Severity != traceSeverityInfo {
		t.Fatalf("expected INFO severity for low sample, got %q", diag.Severity)
	}
	if diag.Issue != "Low packet sample" {
		t.Fatalf("unexpected issue: %q", diag.Issue)
	}
}

func TestAnalyzeTracePacketStableDefault(t *testing.T) {
	t.Parallel()

	diag := analyzeTracePacket(tracePacketResult{
		DecodedPackets: 42,
		SynCount:       12,
		SynAckCount:    10,
		RstCount:       1,
	})
	if diag.Severity != traceSeverityInfo {
		t.Fatalf("expected INFO severity for stable sample, got %q", diag.Severity)
	}
	if diag.Issue != "No strong packet-level anomaly" {
		t.Fatalf("unexpected issue: %q", diag.Issue)
	}
	if diag.Confidence != "MEDIUM" {
		t.Fatalf("expected MEDIUM confidence, got %q", diag.Confidence)
	}
}

func TestTracePacketConfidenceBuckets(t *testing.T) {
	t.Parallel()

	cases := []struct {
		decoded int
		want    string
	}{
		{decoded: 0, want: "LOW"},
		{decoded: 29, want: "LOW"},
		{decoded: 30, want: "MEDIUM"},
		{decoded: 99, want: "MEDIUM"},
		{decoded: 100, want: "HIGH"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if got := tracePacketConfidence(tc.decoded); got != tc.want {
				t.Fatalf("decoded=%d expected %q, got %q", tc.decoded, tc.want, got)
			}
		})
	}
}
