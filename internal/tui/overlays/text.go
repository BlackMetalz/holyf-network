package overlays

import "strings"

func BuildSocketQueueExplainText(aggregate bool) string {
	lines := []string{
		"  [yellow]Send-Q[white]: bytes queued in kernel send buffer (waiting to be sent/acked).",
		"  [yellow]Recv-Q[white]: bytes received by kernel but not yet read by application.",
		"  [yellow]TX/s, RX/s[white]: throughput rate from conntrack byte delta over sample interval.",
		"",
		"  These are [yellow]backlog snapshot[white] values at one moment in time.",
		"  They are [yellow]NOT[white] throughput counters (not B/s, not total bytes sent/recv).",
		"  [dim]0B in Send-Q/Recv-Q does not mean no traffic; it means no queued backlog now.[white]",
		"  [dim]Bandwidth values need conntrack baseline + enough privileges.[white]",
	}
	if aggregate {
		lines = append(lines,
			"",
			"  In replay/group rows, Send-Q/Recv-Q are [yellow]sum of matched connections[white].",
		)
	} else {
		lines = append(lines,
			"",
			"  In Top Connections row, values belong to [yellow]that single socket[white].",
		)
	}
	lines = append(lines, "", "  [dim]Press Enter/Esc to close[white]")
	return strings.Join(lines, "\n")
}

func BuildInterfaceStatsExplainText() string {
	lines := []string{
		"  [yellow]RX / TX[white]: interface throughput (bytes/sec) across all traffic on this NIC.",
		"  [yellow]Packet rate[white]: packet rate on this NIC, not per process.",
		"  [yellow]Speed[white]: interface link speed and current utilization when the NIC reports speed.",
		"  [yellow]Traffic[white]: hidden when quiet; shown only when spike warn/crit needs attention.",
		"  [yellow]App Usage[white]: logical CPU cores used and RSS memory of the current holyf-network process, not host-wide metrics.",
		"  [dim]App CPU/Mem is sampled on the configured refresh interval (-r), not the 1s interface lane.[white]",
		"",
		"  [dim]Why bytes and packets both matter:[white]",
		"  - High packets with low bytes => many small packets (chatty traffic).",
		"  - High bytes with lower packets => larger payload transfers.",
		"",
		"  [yellow]Errors / Drops[white]: cumulative interface counters from kernel/NIC stats.",
		"  [dim]These are totals since boot/driver reset, not per-interval deltas.[white]",
		"",
		"  [dim]First refresh shows baseline only. Rates appear from next sample onward.[white]",
		"  [dim]App CPU needs two refresh-interval samples before a stable core value is shown.[white]",
		"",
		"  [dim]Press Enter/Esc to close[white]",
	}
	return strings.Join(lines, "\n")
}
