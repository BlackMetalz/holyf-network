# Day 1 - Mar 02, 2026

# Sprint 1: Project Setup & TUI Framework
Build TUI and demo first, no real interaction at all. All is mock data, like "Loading..."

## Epic 1 (CLI Skeleton)

Init Module: `go mod init github.com/BlackMetalz/holyf-network`

No idea If i really need that in future, LOL.

So Gemini generated 3 files
- `cmd/root.go`: this is where cobra root cli, where it handle/add flag for commands. 
- `internal/network/interface.go`: this is where it list all network interface, and detect default interface.
- `main.go`: this is where it execute the root command.


### ListInterfaces() function
It parses `/sys/class/net/` in Linux, we only support Linux right now!

### DetectDefaultInterface() function
It parses /proc/net/route

### Output of Epic 1
```bash
kienlt@Luongs-MacBook-Pro holyf-network % go run . -v
holyf-network version 0.1.0
kienlt@Luongs-MacBook-Pro holyf-network % go run . -h
HolyF-network - A terminal UI dashboard for monitoring network health on Linux servers.

Usage:
  holyf-network [flags]

Flags:
  -h, --help               help for holyf-network
  -i, --interface string   Network interface to monitor (default: auto-detect)
      --list-interfaces    List available network interfaces and exit
  -r, --refresh int        Refresh interval in seconds (1-300) (default 30)
  -v, --version            version for holyf-network
kienlt@Luongs-MacBook-Pro holyf-network % go run . --list-interfaces
Available network interfaces:
  - gif0
  - stf0
  ......
```

## Epic 2 (TUI Framework)

So Gemini generated 4 files
- `internal/tui/app.go`: this is where it create tui app, and handle key event. Build grid 2x2 + status bar.
- `internal/tui/layout.go`: this is where it create layout for tui app. Struct `App` + key handler.
- `internal/tui/panels.go`: this is where it create panels for tui app.
- `internal/tui/help.go`: this is where it create help modal for tui app.


Run it: `./holyf-network -i interace-name-here`

`Tab` for change panel!
`?` for help!
`q` for quit!

## Output of Epic 2

Demo:
![output-epic-2](../images/day01/01.png)

