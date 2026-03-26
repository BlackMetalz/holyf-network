package replay

const HelpText = `[yellow]Replay Shortcuts[white]
──────────────────────────
[yellow]][white]             Next snapshot
[yellow][[white]             Previous snapshot
[yellow]a / e[white]         Jump to oldest / latest snapshot
[yellow]t[white]             Jump to specific time
[yellow]g[white]             Toggle replay view CONN <-> TRACE
[yellow]h[white]             Open replay trace history modal (c=compare inside)
[yellow]Shift+S[white]       Search timeline (all loaded snapshots)
[yellow]L[white]             Follow latest snapshots
[yellow]o[white]             Toggle replay IN/OUT direction
[yellow]Up/Down[white]     Select row
[yellow]f[white]           Port filter / clear all filters
[yellow]/[white]           Search (contains text in current snapshot)
[yellow]Shift+B[white]     Sort by Bandwidth (press again: DESC/ASC)
[yellow]Shift+C[white]     Sort by Conns (press again: DESC/ASC)
[yellow]Shift+P[white]     Sort by Port (press again: DESC/ASC)
[yellow]i[white]           Explain Send-Q / Recv-Q / TX/s / RX/s
[yellow]Shift+I[white]     Alias of i (Explain queue/bandwidth columns)
[yellow]m[white]           Toggle sensitive IP mask
[yellow]x[white]           Toggle skip-empty snapshots
[yellow]?[white]           Toggle this help
[yellow]q[white]           Quit

[dim]Replay is read-only (no kill/block actions)[white]
[dim]Trace-only fallback auto-enables when no connections snapshots exist.[white]
[dim]Press Esc or any key to close[white]`

func StatusHotkeysForPage(page string) (styled string, plain string) {
	switch page {
	case "history-help":
		return "[dim]any key[white]=close", "any key=close"
	case "history-filter", "history-search", "history-jump-time":
		return "[dim]Enter[white]=apply [dim]Esc[white]=cancel", "Enter=apply Esc=cancel"
	case "history-timeline-search":
		return "[dim]Enter[white]=search [dim]Esc[white]=cancel", "Enter=search Esc=cancel"
	case "history-timeline-results":
		return "[dim]Up/Down[white]=select [dim]Enter[white]=jump [dim]Esc[white]=close", "Up/Down=select Enter=jump Esc=close"
	case "history-trace-history":
		return "[dim]Up/Down[white]=select [dim]Enter[white]=detail [dim]c[white]=compare [dim]Esc[white]=close", "Up/Down=select Enter=detail c=compare Esc=close"
	case "history-trace-history-detail", "history-trace-history-compare", "history-socket-queue-explain":
		return "[dim]Enter[white]=close [dim]Esc[white]=close", "Enter=close Esc=close"
	default:
		return "[dim][=prev ]=next a e t f / Shift+S Shift+B/C/P o g h m i Shift+I x z L ? q[white]", "[=prev ]=next a e t f / Shift+S Shift+B/C/P o g h m i Shift+I x z L ? q"
	}
}
