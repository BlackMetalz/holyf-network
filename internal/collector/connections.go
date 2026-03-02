package collector

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// connections.go — Parses /proc/net/tcp and /proc/net/tcp6 to count TCP connection states.

// tcpStateMap maps hex state codes from /proc/net/tcp to human-readable names.
// These are defined in the Linux kernel: include/net/tcp_states.h
// I would give a link here for more detail: https://github.com/torvalds/linux/blob/master/include/net/tcp_states.h
var tcpStateMap = map[string]string{
	"01": "ESTABLISHED",
	"02": "SYN_SENT",
	"03": "SYN_RECV",
	"04": "FIN_WAIT1",
	"05": "FIN_WAIT2",
	"06": "TIME_WAIT",
	"07": "CLOSE",
	"08": "CLOSE_WAIT",
	"09": "LAST_ACK",
	"0A": "LISTEN",
	"0B": "CLOSING",
}

// displayOrder defines the order states are shown in the panel.
// Most important/common states first.
var displayOrder = []string{
	"ESTABLISHED",
	"TIME_WAIT",
	"CLOSE_WAIT",
	"LISTEN",
	"SYN_SENT",
	"SYN_RECV",
	"FIN_WAIT1",
	"FIN_WAIT2",
	"LAST_ACK",
	"CLOSING",
	"CLOSE",
}

// ConnectionData holds parsed TCP connection state counts.
type ConnectionData struct {
	States map[string]int // state name → count
	Total  int            // sum of all connections
}

// CollectConnections reads /proc/net/tcp and /proc/net/tcp6,
// counts connections in each TCP state, and returns the results.
func CollectConnections() (ConnectionData, error) {
	data := ConnectionData{
		States: make(map[string]int),
	}

	// Parse both IPv4 and IPv6 TCP connections
	files := []string{"/proc/net/tcp", "/proc/net/tcp6"}
	parsed := false

	for _, file := range files {
		err := parseProcNetTCP(file, data.States)
		if err != nil {
			// File might not exist (e.g., no IPv6, or not Linux)
			continue
		}
		parsed = true
	}

	if !parsed {
		return data, fmt.Errorf("cannot read /proc/net/tcp (requires Linux)")
	}

	// Calculate total
	for _, count := range data.States {
		data.Total += count
	}

	return data, nil
}

// parseProcNetTCP reads one /proc/net/tcp file and adds state counts to the map.
//
// File format (each line after header):
//
//	sl  local_address rem_address   st tx_queue:rx_queue ...
//	 0: 0100007F:0277 00000000:0000 0A 00000000:00000000 ...
//	                                ^^
//	                                state (hex)
func parseProcNetTCP(filePath string, states map[string]int) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", filePath, err)
	}

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // Skip header line
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue // Skip empty or malformed lines
		}

		// Field index 3 is the state in hex (e.g., "01", "0A")
		stateHex := fields[3]
		stateName, ok := tcpStateMap[stateHex]
		if !ok {
			stateName = "UNKNOWN"
		}

		states[stateName]++
	}

	return nil
}

// SortedStates returns states in display order, filtering out zero-count states.
func (d ConnectionData) SortedStates() []StateCount {
	var result []StateCount

	// First, add states in preferred display order
	seen := make(map[string]bool)
	for _, name := range displayOrder {
		count, exists := d.States[name]
		if exists && count > 0 {
			result = append(result, StateCount{Name: name, Count: count})
			seen[name] = true
		}
	}

	// Then, add any unknown states not in displayOrder
	var extras []StateCount
	for name, count := range d.States {
		if !seen[name] && count > 0 {
			extras = append(extras, StateCount{Name: name, Count: count})
		}
	}
	sort.Slice(extras, func(i, j int) bool {
		return extras[i].Count > extras[j].Count
	})
	result = append(result, extras...)

	return result
}

// StateCount is a name+count pair for display.
type StateCount struct {
	Name  string
	Count int
}
