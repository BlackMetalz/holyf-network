package tui

import (
	"errors"
	"strings"
	"testing"

	tuishared "github.com/BlackMetalz/holyf-network/internal/tui/shared"
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
