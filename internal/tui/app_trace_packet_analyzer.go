package tui

import (
	"fmt"
)

const (
	traceSeverityInfo = "INFO"
	traceSeverityWarn = "WARN"
	traceSeverityCrit = "CRIT"
)

type tracePacketDiagnosis struct {
	Severity   string
	Confidence string
	Issue      string
	Signal     string
	Likely     string
	Check      string
}

func analyzeTracePacket(result tracePacketResult) tracePacketDiagnosis {
	confidence := tracePacketConfidence(result.DecodedPackets)
	dropValue := tracePacketMetricValue(result.DroppedByKernel)
	baseSignal := fmt.Sprintf(
		"Decoded %d | SYN %d | SYN-ACK %d | RST %d | Drop %s",
		result.DecodedPackets,
		result.SynCount,
		result.SynAckCount,
		result.RstCount,
		dropValue,
	)

	if result.CaptureErr != nil {
		return tracePacketDiagnosis{
			Severity:   traceSeverityCrit,
			Confidence: "HIGH",
			Issue:      "Capture failed",
			Signal:     baseSignal,
			Likely:     "tcpdump exited with an error before a reliable sample was collected.",
			Check:      "Verify privileges, interface name, and tcpdump availability on host.",
		}
	}

	if result.DroppedByKernel > 0 {
		capturedForRatio := result.Captured
		if capturedForRatio < 0 {
			capturedForRatio = result.DecodedPackets
		}
		totalObserved := capturedForRatio + result.DroppedByKernel
		dropRatio := 0.0
		if totalObserved > 0 {
			dropRatio = float64(result.DroppedByKernel) / float64(totalObserved)
		}

		severity := traceSeverityWarn
		if dropRatio >= 0.10 || result.DroppedByKernel >= 100 {
			severity = traceSeverityCrit
		}
		return tracePacketDiagnosis{
			Severity:   severity,
			Confidence: confidence,
			Issue:      "tcpdump dropped packets",
			Signal: fmt.Sprintf(
				"%s | drop ratio %.1f%%",
				baseSignal,
				dropRatio*100,
			),
			Likely: "capture path is overloaded, so packet sample can be incomplete.",
			Check:  "Reduce scope/duration, retry at lower host load, or increase tcpdump buffer (-B) if needed.",
		}
	}

	if result.DecodedPackets > 0 && result.RstCount >= 5 {
		rstRatio := float64(result.RstCount) / float64(result.DecodedPackets)
		if rstRatio >= 0.20 {
			severity := traceSeverityWarn
			if rstRatio >= 0.40 || result.RstCount >= 20 {
				severity = traceSeverityCrit
			}
			return tracePacketDiagnosis{
				Severity:   severity,
				Confidence: confidence,
				Issue:      "RST pressure",
				Signal: fmt.Sprintf(
					"%s | RST ratio %.1f%%",
					baseSignal,
					rstRatio*100,
				),
				Likely: "connections are being actively reset by peer, middlebox, or local policy.",
				Check:  "Inspect app logs and firewall reject/reset rules on both ends.",
			}
		}
	}

	if result.SynCount >= 3 {
		if result.SynAckCount == 0 {
			severity := traceSeverityWarn
			if result.SynCount >= 10 {
				severity = traceSeverityCrit
			}
			return tracePacketDiagnosis{
				Severity:   severity,
				Confidence: confidence,
				Issue:      "SYN seen but no SYN-ACK",
				Signal:     baseSignal,
				Likely:     "return path, firewall policy, or peer reachability may be blocking handshake replies.",
				Check:      "Verify route/firewall/NAT/security-group between source and destination.",
			}
		}
		synAckRatio := float64(result.SynAckCount) / float64(result.SynCount)
		if synAckRatio < 0.40 {
			return tracePacketDiagnosis{
				Severity:   traceSeverityWarn,
				Confidence: confidence,
				Issue:      "Low SYN-ACK ratio",
				Signal: fmt.Sprintf(
					"%s | SYN-ACK ratio %.1f%%",
					baseSignal,
					synAckRatio*100,
				),
				Likely: "only part of handshake attempts receive replies.",
				Check:  "Check intermittent path loss, policy throttling, or peer-side saturation.",
			}
		}
	}

	if result.DecodedPackets < 10 {
		return tracePacketDiagnosis{
			Severity:   traceSeverityInfo,
			Confidence: confidence,
			Issue:      "Low packet sample",
			Signal:     baseSignal,
			Likely:     "sample size is too small for a strong packet-level conclusion.",
			Check:      "Increase duration/packet cap and capture during the problematic window.",
		}
	}

	return tracePacketDiagnosis{
		Severity:   traceSeverityInfo,
		Confidence: confidence,
		Issue:      "No strong packet-level anomaly",
		Signal:     baseSignal,
		Likely:     "handshake/reset signals look stable in this bounded capture.",
		Check:      "Correlate with Connection States, Interface Stats, and Conntrack panels for host-level context.",
	}
}

func tracePacketConfidence(decodedPackets int) string {
	switch {
	case decodedPackets >= 100:
		return "HIGH"
	case decodedPackets >= 30:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func tracePacketSeverityStyled(severity string) string {
	switch severity {
	case traceSeverityCrit:
		return "[red]CRIT[white]"
	case traceSeverityWarn:
		return "[yellow]WARN[white]"
	default:
		return "[green]INFO[white]"
	}
}

func tracePacketConfidenceStyled(confidence string) string {
	switch confidence {
	case "HIGH":
		return "[green]HIGH[white]"
	case "MEDIUM":
		return "[yellow]MEDIUM[white]"
	default:
		return "[aqua]LOW[white]"
	}
}
