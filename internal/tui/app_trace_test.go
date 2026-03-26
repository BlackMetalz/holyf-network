package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/tui/actionlog"
	"github.com/BlackMetalz/holyf-network/internal/tui/livetrace"
	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
	tuitrace "github.com/BlackMetalz/holyf-network/internal/tui/trace"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestBuildTracePacketFilter(t *testing.T) {
	t.Parallel()

	req := tracePacketRequest{
		PeerIP: "203.0.113.10",
		Port:   443,
		Scope:  traceScopePeerPort,
	}
	if got := buildTracePacketFilter(req); got != "tcp and host 203.0.113.10 and port 443" {
		t.Fatalf("unexpected peer+port filter: %q", got)
	}

	req.Scope = traceScopePeerOnly
	if got := buildTracePacketFilter(req); got != "tcp and host 203.0.113.10" {
		t.Fatalf("unexpected peer-only filter: %q", got)
	}
}

func TestBuildTracePacketFilterPresets(t *testing.T) {
	t.Parallel()

	req := tracePacketRequest{
		PeerIP:          "203.0.113.10",
		Port:            443,
		Scope:           traceScopePeerPort,
		Preset:          traceFilterPresetSynRstOnly,
		TupleLocalIP:    "172.25.110.116",
		TupleRemoteIP:   "203.0.113.10",
		TupleLocalPort:  22,
		TupleRemotePort: 41334,
	}
	if got := buildTracePacketFilter(req); got != "tcp and host 203.0.113.10 and port 443 and (tcp[tcpflags] & (tcp-syn|tcp-rst) != 0)" {
		t.Fatalf("unexpected syn/rst filter: %q", got)
	}

	req.Preset = traceFilterPresetFiveTuple
	if got := buildTracePacketFilter(req); got != "tcp and ((src host 172.25.110.116 and src port 22 and dst host 203.0.113.10 and dst port 41334) or (src host 203.0.113.10 and src port 41334 and dst host 172.25.110.116 and dst port 22))" {
		t.Fatalf("unexpected 5-tuple filter: %q", got)
	}

	req.Preset = traceFilterPresetCustom
	req.CustomClause = "tcp[13] & 0x10 != 0"
	if got := buildTracePacketFilter(req); got != "tcp and host 203.0.113.10 and port 443 and (tcp[13] & 0x10 != 0)" {
		t.Fatalf("unexpected custom filter: %q", got)
	}
}

func TestBuildTracePacketFilterAppendsCustomClauseOnStrategy(t *testing.T) {
	t.Parallel()

	req := tracePacketRequest{
		PeerIP:       "203.0.113.10",
		Port:         443,
		Scope:        traceScopePeerPort,
		Preset:       traceFilterPresetSynRstOnly,
		CustomClause: "tcp[13] & 0x10 != 0",
	}
	want := "tcp and host 203.0.113.10 and port 443 and (tcp[tcpflags] & (tcp-syn|tcp-rst) != 0) and (tcp[13] & 0x10 != 0)"
	if got := buildTracePacketFilter(req); got != want {
		t.Fatalf("unexpected strategy+custom filter: %q", got)
	}
}

func TestTracePacketFilterPresetSlug(t *testing.T) {
	t.Parallel()

	cases := []struct {
		preset tracePacketFilterPreset
		want   string
	}{
		{preset: traceFilterPresetPeerPort, want: "peer-port"},
		{preset: traceFilterPresetPeerOnly, want: "peer-only"},
		{preset: traceFilterPresetFiveTuple, want: "five-tuple"},
		{preset: traceFilterPresetSynRstOnly, want: "syn-rst"},
		{preset: traceFilterPresetCustom, want: "custom"},
	}

	for _, tc := range cases {
		if got := tc.preset.Slug(); got != tc.want {
			t.Fatalf("unexpected slug for preset=%v: got=%q want=%q", tc.preset, got, tc.want)
		}
	}
}

func TestTracePacketCaptureProfileDefaults(t *testing.T) {
	t.Parallel()

	seedWithPort := tracePacketSeed{PeerIP: "203.0.113.10", Port: 443}
	general, ok := traceCaptureProfileDefaultsFor(traceCaptureProfileGeneral, seedWithPort, tuishared.TopConnectionIncoming)
	if !ok {
		t.Fatalf("general profile should return defaults")
	}
	if general.Preset != traceFilterPresetPeerPort || general.Direction != traceDirectionIn {
		t.Fatalf("unexpected general defaults: %+v", general)
	}

	handshake, ok := traceCaptureProfileDefaultsFor(traceCaptureProfileHandshake, seedWithPort, tuishared.TopConnectionOutgoing)
	if !ok {
		t.Fatalf("handshake profile should return defaults")
	}
	if handshake.Preset != traceFilterPresetSynRstOnly || handshake.Direction != traceDirectionOut {
		t.Fatalf("unexpected handshake defaults: %+v", handshake)
	}

	loss, ok := traceCaptureProfileDefaultsFor(traceCaptureProfilePacketLoss, tracePacketSeed{PeerIP: "203.0.113.10"}, tuishared.TopConnectionIncoming)
	if !ok {
		t.Fatalf("packet-loss profile should return defaults")
	}
	if loss.Preset != traceFilterPresetPeerOnly || loss.Direction != traceDirectionAny {
		t.Fatalf("unexpected packet-loss defaults: %+v", loss)
	}

	if _, ok := traceCaptureProfileDefaultsFor(traceCaptureProfileCustom, seedWithPort, tuishared.TopConnectionIncoming); ok {
		t.Fatalf("custom profile should not force defaults")
	}
}

func TestShouldSubmitTracePacketOnEnter(t *testing.T) {
	t.Parallel()

	form := tview.NewForm()
	input := tview.NewInputField().SetLabel("Peer")
	drop := tview.NewDropDown().SetLabel("Preset").SetOptions([]string{"A", "B"}, nil)
	check := tview.NewCheckbox().SetLabel("Save")
	form.AddFormItem(input)
	form.AddFormItem(drop)
	form.AddFormItem(check)
	form.AddButton("Start", nil)

	form.SetFocus(0)
	if !shouldSubmitTracePacketOnEnter(form, drop, drop, drop) {
		t.Fatalf("expected enter submit on input field focus")
	}

	form.SetFocus(1)
	if shouldSubmitTracePacketOnEnter(form, drop, drop, drop) {
		t.Fatalf("dropdown focus should not auto submit")
	}

	form.SetFocus(2)
	if shouldSubmitTracePacketOnEnter(form, drop, drop, drop) {
		t.Fatalf("checkbox focus should not auto submit")
	}

	form.SetFocus(3)
	if shouldSubmitTracePacketOnEnter(form, drop, drop, drop) {
		t.Fatalf("button focus should not auto submit")
	}
}

func TestShouldCloseTracePacketFormOnEsc(t *testing.T) {
	t.Parallel()

	if !shouldCloseTracePacketFormOnEscByState(false, false, false) {
		t.Fatalf("expected esc to close form when no dropdown is open")
	}
	if shouldCloseTracePacketFormOnEscByState(true, false, false) {
		t.Fatalf("expected esc to close dropdown (not form) when profile dropdown is open")
	}
	if shouldCloseTracePacketFormOnEscByState(false, true, false) {
		t.Fatalf("expected esc to close dropdown (not form) when preset dropdown is open")
	}
	if shouldCloseTracePacketFormOnEscByState(false, false, true) {
		t.Fatalf("expected esc to close dropdown (not form) when direction dropdown is open")
	}
}

func TestParseTracePacketCounters(t *testing.T) {
	t.Parallel()

	raw := `tcpdump: listening on eth0, link-type EN10MB (Ethernet), snapshot length 262144 bytes
12 packets captured
17 packets received by filter
1 packets dropped by kernel`

	captured, receivedByFilter, droppedByKernel := parseTracePacketCounters(raw)
	if captured != 12 || receivedByFilter != 17 || droppedByKernel != 1 {
		t.Fatalf("unexpected counters: captured=%d recv=%d drop=%d", captured, receivedByFilter, droppedByKernel)
	}
}

func TestParseTracePacketCountersMissingRowsStayNA(t *testing.T) {
	t.Parallel()

	captured, receivedByFilter, droppedByKernel := parseTracePacketCounters("tcpdump: no summary lines")
	if captured != -1 || receivedByFilter != -1 || droppedByKernel != -1 {
		t.Fatalf("expected -1 defaults, got captured=%d recv=%d drop=%d", captured, receivedByFilter, droppedByKernel)
	}
}

func TestParseTracePacketIntRange(t *testing.T) {
	t.Parallel()

	v, err := parseTracePacketIntRange(" 10 ", 1, 60, "Duration")
	if err != nil || v != 10 {
		t.Fatalf("expected valid parse, got v=%d err=%v", v, err)
	}
	if _, err := parseTracePacketIntRange("0", 1, 60, "Duration"); err == nil {
		t.Fatalf("expected error for out-of-range value")
	}
	if _, err := parseTracePacketIntRange("abc", 1, 60, "Duration"); err == nil {
		t.Fatalf("expected error for invalid integer")
	}
}

func TestBuildTracePacketActionSummaryIncludesKeyFields(t *testing.T) {
	t.Parallel()

	result := tracePacketResult{
		Request: tracePacketRequest{
			PeerIP:    "203.0.113.10",
			Port:      443,
			Scope:     traceScopePeerPort,
			Direction: traceDirectionIn,
		},
		Captured:        12,
		DroppedByKernel: 1,
		RstCount:        2,
		Saved:           true,
		PCAPPath:        "/tmp/holyf-network-captures/trace-test.pcap",
	}

	summary := buildTracePacketActionSummary(result, false)
	for _, want := range []string{
		"Trace ok 203.0.113.10:443",
		"mode=General triage dir=IN scope=Peer + Port",
		"captured=12 drop=1 rst=2",
		"saved=/tmp/holyf-network-captures/trace-test.pcap",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("expected summary to contain %q, got: %q", want, summary)
		}
	}
}

func TestBuildTracePacketActionSummaryMasksSensitiveIP(t *testing.T) {
	t.Parallel()

	result := tracePacketResult{
		Request: tracePacketRequest{
			PeerIP:    "203.0.113.10",
			Port:      443,
			Scope:     traceScopePeerPort,
			Direction: traceDirectionIn,
		},
		Captured:        12,
		DroppedByKernel: 1,
		RstCount:        2,
		Saved:           true,
		PCAPPath:        "/tmp/holyf-network-captures/trace-203_0_113_10-443.pcap",
	}

	summary := buildTracePacketActionSummary(result, true)
	if strings.Contains(summary, "203.0.113.10") {
		t.Fatalf("expected masked peer ip in summary, got: %q", summary)
	}
	if strings.Contains(summary, "trace-203_0_113_10-443.pcap") {
		t.Fatalf("expected masked pcap path in summary, got: %q", summary)
	}
	if !strings.Contains(summary, "xxx.xxx.113.10") {
		t.Fatalf("expected masked peer format in summary, got: %q", summary)
	}
}

func TestTracePacketMetricDisplayEstimated(t *testing.T) {
	t.Parallel()

	if got := tracePacketMetricDisplay(47, true); got != "47 (est.)" {
		t.Fatalf("unexpected estimated display: %q", got)
	}
	if got := tracePacketMetricDisplay(-1, true); got != "n/a" {
		t.Fatalf("unexpected n/a display: %q", got)
	}
}

func TestTracePacketScopeDisplayPresets(t *testing.T) {
	t.Parallel()

	if got := tracePacketScopeDisplay(tracePacketRequest{Scope: traceScopePeerPort}); got != "Peer + Port" {
		t.Fatalf("unexpected default scope display: %q", got)
	}
	if got := tracePacketScopeDisplay(tracePacketRequest{Preset: traceFilterPresetFiveTuple, Scope: traceScopePeerPort}); got != "5-tuple" {
		t.Fatalf("unexpected 5-tuple scope display: %q", got)
	}
	if got := tracePacketScopeDisplay(tracePacketRequest{Preset: traceFilterPresetSynRstOnly, Scope: traceScopePeerPort}); got != "SYN/RST only" {
		t.Fatalf("unexpected syn/rst scope display: %q", got)
	}
	if got := tracePacketScopeDisplay(tracePacketRequest{Preset: traceFilterPresetCustom, Scope: traceScopePeerOnly}); got != "Custom (Peer)" {
		t.Fatalf("unexpected custom peer scope display: %q", got)
	}
}

func TestShouldDowngradeTracePacketReadWarning(t *testing.T) {
	t.Parallel()

	result := tracePacketResult{
		TimedOut:       true,
		DecodedPackets: 10,
		ReadErr:        errors.New("pcap read failed: reading from file /tmp/x.pcap, tcpdump: pcap_loop: truncated dump file"),
	}
	if !shouldDowngradeTracePacketReadWarning(result) {
		t.Fatalf("expected timeout-boundary read warning to be downgraded")
	}

	result.TimedOut = false
	if shouldDowngradeTracePacketReadWarning(result) {
		t.Fatalf("expected non-timeout warning to keep warning severity")
	}
}

func TestMaskSensitiveIPsInText(t *testing.T) {
	t.Parallel()

	line := "IP 14.231.106.188.41334 > 172.25.110.116.22: Flags [.], ack 1, win 1"
	masked := maskSensitiveIPsInText(line, true)
	if strings.Contains(masked, "14.231.106.188") || strings.Contains(masked, "172.25.110.116") {
		t.Fatalf("expected ipv4 addresses to be masked, got: %q", masked)
	}
	if !strings.Contains(masked, "xxx.xxx.106.188") || !strings.Contains(masked, "xxx.xxx.110.116") {
		t.Fatalf("expected masked ipv4 output, got: %q", masked)
	}
}

func TestBuildTracePacketResultTextMasksSensitiveParts(t *testing.T) {
	t.Parallel()

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
		Filter:      "tcp and host 203.0.113.10 and port 443",
		Saved:       true,
		PCAPPath:    "/tmp/holyf-network-captures/trace-20260322-003941-203_0_113_10-443.pcap",
		SampleLines: []string{"IP 203.0.113.10.443 > 172.25.110.116.22: Flags [S], seq 1"},
	}

	text := buildTracePacketResultText(result, true)
	if strings.Contains(text, "203.0.113.10") || strings.Contains(text, "172.25.110.116") {
		t.Fatalf("expected masked ips in result text, got: %q", text)
	}
	if !strings.Contains(text, "Trace Analyzer") {
		t.Fatalf("expected analyzer section in result text, got: %q", text)
	}
	if !strings.Contains(text, "trace-20260322-003941-masked.pcap") {
		t.Fatalf("expected masked pcap path in result text, got: %q", text)
	}
}

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

func TestAppendTraceHistoryPersistsDailySegments(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	engine := livetrace.NewEngine(dataDir)
	engine.MarkHistoryLoaded() // don't load from disk
	a := &App{
		actionLogger: actionlog.NewLogger(""),
		traceEngine:  engine,
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
		tuitrace.SegmentFileName(base),
		tuitrace.SegmentFileName(base.AddDate(0, 0, 1)),
		tuitrace.SegmentFileName(base.AddDate(0, 0, 2)),
	}
	for _, name := range wantFiles {
		if _, err := os.Stat(filepath.Join(dataDir, name)); err != nil {
			t.Fatalf("expected segment file %s: %v", name, err)
		}
	}

	loaded, err := tuitrace.ReadEntriesFromDir(dataDir)
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
	old := tuitrace.SegmentFileName(now.AddDate(0, 0, -10))
	cur := tuitrace.SegmentFileName(now)
	if err := os.WriteFile(filepath.Join(dir, old), []byte("{\"captured_at\":\"2026-03-20T10:00:00Z\"}\n"), 0o600); err != nil {
		t.Fatalf("write old segment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, cur), []byte("{\"captured_at\":\"2026-03-30T10:00:00Z\"}\n"), 0o600); err != nil {
		t.Fatalf("write current segment: %v", err)
	}

	if err := tuitrace.PruneDataDirByAge(dir, 24*7, now); err != nil {
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

	entry := tuitrace.Entry{
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
	if !strings.Contains(text, "Mode: [green]General triage[white]") {
		t.Fatalf("expected mode line in detail text, got=%q", text)
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

	if got := traceHistoryCategory(tuitrace.Entry{Preset: "SYN/RST only"}); got != "SYN/RST only" {
		t.Fatalf("expected explicit preset category, got=%q", got)
	}
	if got := traceHistoryCategory(tuitrace.Entry{Scope: "5-tuple"}); got != "5-tuple" {
		t.Fatalf("expected 5-tuple fallback, got=%q", got)
	}
	if got := traceHistoryCategory(tuitrace.Entry{Scope: "Custom (Peer+Port)"}); got != "Custom" {
		t.Fatalf("expected custom fallback, got=%q", got)
	}
	if got := traceHistoryCategory(tuitrace.Entry{Scope: "Peer only"}); got != "Peer only" {
		t.Fatalf("expected peer-only fallback, got=%q", got)
	}
	if got := traceHistoryCategory(tuitrace.Entry{}); got != "Peer + Port" {
		t.Fatalf("expected default category, got=%q", got)
	}
}

func TestBuildTraceHistoryCompareTextShowsRequestedDiffs(t *testing.T) {
	t.Parallel()

	baseline := tuitrace.Entry{
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
	incident := tuitrace.Entry{
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
	baseline := tuitrace.Entry{
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
	incident := tuitrace.Entry{
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
