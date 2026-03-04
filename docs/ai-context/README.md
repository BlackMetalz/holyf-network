# AI Context Pack

This folder is the fastest way for a new AI agent (or new teammate, probably not LOLLL) to understand this repo before changing code.

## Start Here

1. Read `PROJECT_STRUCTURE.md` for package-level ownership.
2. Read `HIGH_LEVEL_DESIGN.md` for runtime behavior and data flow.
3. Read root `README.MD` for runtime requirements and user-facing shortcuts.

## Suggested Read Order In Code

1. `main.go`
2. `cmd/root.go`
3. `cmd/daemon.go` + `cmd/replay.go`
4. `internal/tui/app_core.go`
5. `internal/tui/history_app.go`
6. `internal/history/*.go`
7. `internal/tui/app_top_connections.go`
8. `internal/tui/app_blocking_kill_flow.go`
9. `internal/actions/peer_blocker.go`
10. `internal/collector/*.go`

## One-Command Sanity Check

```bash
go test ./...
```

## Scope Reminder

- Linux-first TUI network observability tool.
- Core runtime data comes from `/proc` and `/sys`.
- Active mitigation uses `iptables`/`ip6tables`, `conntrack`, and `ss`.
- Full functionality expects `sudo`.
