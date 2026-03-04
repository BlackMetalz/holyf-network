package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// top_connections.go — Parses /proc/net/tcp to find the most active connections.
// Uses tx_queue + rx_queue as activity proxy (accurate bytes need eBPF in v3).

// Connection represents a single TCP connection from /proc/net/tcp.
type Connection struct {
	LocalIP    string
	LocalPort  int
	RemoteIP   string
	RemotePort int
	State      string
	TxQueue    int64  // Bytes waiting to be sent
	RxQueue    int64  // Bytes waiting to be read
	Activity   int64  // TxQueue + RxQueue (sort key)
	Inode      string // Socket inode number (for PID lookup)
	PID        int    // Process ID owning this socket (0 = unknown)
	ProcName   string // Process name from /proc/[pid]/comm
}

// CollectTopTalkers parses /proc/net/tcp + tcp6 and returns
// connections sorted by queue activity (descending). When limit > 0,
// only the top N items are returned.
// Each connection is enriched with PID and process name when available.
func CollectTopTalkers(limit int) ([]Connection, error) {
	var allConns []Connection

	files := []string{"/proc/net/tcp", "/proc/net/tcp6"}
	parsed := false

	for _, file := range files {
		conns, err := parseTCPConnections(file)
		if err != nil {
			continue
		}
		allConns = append(allConns, conns...)
		parsed = true
	}

	if !parsed {
		return nil, fmt.Errorf("cannot read /proc/net/tcp (requires Linux)")
	}

	// Build inode→PID map once, then enrich all connections
	inodeMap := buildInodeToPIDMap()
	for i := range allConns {
		if pid, ok := inodeMap[allConns[i].Inode]; ok {
			allConns[i].PID = pid
			allConns[i].ProcName = getProcessName(pid)
		}
	}

	// Sort by activity (tx_queue + rx_queue) descending
	sort.Slice(allConns, func(i, j int) bool {
		return allConns[i].Activity > allConns[j].Activity
	})

	// Limit results when requested.
	if limit > 0 && len(allConns) > limit {
		allConns = allConns[:limit]
	}

	return allConns, nil
}

// parseTCPConnections reads a /proc/net/tcp file and extracts connections.
//
// File format (columns):
//
//	sl local_address rem_address st tx_queue:rx_queue ...
//	 0: 0100007F:0050 0100007F:C000 01 00000100:00000000 ...
//
// Fields by index:
//
//	[0] = "0:" (slot number)
//	[1] = local address:port (hex)
//	[2] = remote address:port (hex)
//	[3] = state (hex)
//	[4] = tx_queue:rx_queue (hex)
func parseTCPConnections(filePath string) ([]Connection, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", filePath, err)
	}

	isIPv6 := strings.Contains(filePath, "tcp6")
	lines := strings.Split(string(content), "\n")
	var conns []Connection

	for i, line := range lines {
		if i == 0 {
			continue // Skip header
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		// Parse state — skip LISTEN (0A), we want active connections
		stateHex := fields[3]
		if stateHex == "0A" {
			continue
		}
		stateName := tcpStateMap[stateHex]
		if stateName == "" {
			stateName = "UNKNOWN"
		}

		// Parse addresses
		localIP, localPort, err := parseHexAddress(fields[1], isIPv6)
		if err != nil {
			continue
		}
		remoteIP, remotePort, err := parseHexAddress(fields[2], isIPv6)
		if err != nil {
			continue
		}

		// Parse queue sizes
		txQueue, rxQueue := parseQueueSizes(fields[4])

		// Parse inode (field index 9)
		inode := ""
		if len(fields) > 9 {
			inode = fields[9]
		}

		conns = append(conns, Connection{
			LocalIP:    localIP,
			LocalPort:  localPort,
			RemoteIP:   remoteIP,
			RemotePort: remotePort,
			State:      stateName,
			TxQueue:    txQueue,
			RxQueue:    rxQueue,
			Activity:   txQueue + rxQueue,
			Inode:      inode,
		})
	}

	return conns, nil
}

// parseHexAddress converts hex address from /proc/net/tcp to human-readable.
//
// IPv4 example: "0100007F:0050" → ("127.0.0.1", 80)
//   - IP is in little-endian byte order: 0100007F → 7F.00.00.01 → 127.0.0.1
//   - Port is in hex: 0050 → 80
//
// IPv6 example: "00000000000000000000000001000000:0050" → ("::1", 80)
func parseHexAddress(hexAddr string, isIPv6 bool) (string, int, error) {
	parts := strings.Split(hexAddr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address format: %s", hexAddr)
	}

	// Parse port (always big-endian hex)
	port, err := strconv.ParseInt(parts[1], 16, 32)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %s", parts[1])
	}

	// Parse IP
	var ip string
	if isIPv6 {
		ip = parseHexIPv6(parts[0])
	} else {
		ip = parseHexIPv4(parts[0])
	}

	return ip, int(port), nil
}

// parseHexIPv4 converts a hex IPv4 address to dotted notation.
// Input is little-endian: "0100007F" → 127.0.0.1
func parseHexIPv4(hex string) string {
	if len(hex) != 8 {
		return hex
	}

	// Parse as 32-bit integer
	val, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return hex
	}

	// Linux stores IPv4 in little-endian in /proc/net/tcp
	// Byte 0 (least significant) is the first octet
	return fmt.Sprintf("%d.%d.%d.%d",
		val&0xFF,
		(val>>8)&0xFF,
		(val>>16)&0xFF,
		(val>>24)&0xFF,
	)
}

// parseHexIPv6 converts a hex IPv6 address to a simplified string.
// /proc/net/tcp6 stores IPv6 as 4 groups of 32-bit little-endian words.
func parseHexIPv6(hex string) string {
	if len(hex) != 32 {
		return hex
	}

	// Parse 4 groups of 4 bytes (each group is little-endian)
	var groups [4]uint32
	for i := 0; i < 4; i++ {
		chunk := hex[i*8 : (i+1)*8]
		val, err := strconv.ParseUint(chunk, 16, 32)
		if err != nil {
			return hex
		}
		// Reverse byte order within each 32-bit word
		groups[i] = uint32(((val & 0xFF) << 24) |
			(((val >> 8) & 0xFF) << 16) |
			(((val >> 16) & 0xFF) << 8) |
			((val >> 24) & 0xFF))
	}

	// Format as IPv6
	ip := fmt.Sprintf("%04x:%04x:%04x:%04x:%04x:%04x:%04x:%04x",
		groups[0]>>16, groups[0]&0xFFFF,
		groups[1]>>16, groups[1]&0xFFFF,
		groups[2]>>16, groups[2]&0xFFFF,
		groups[3]>>16, groups[3]&0xFFFF,
	)

	// Simplify common cases
	if ip == "0000:0000:0000:0000:0000:0000:0000:0001" {
		return "::1"
	}
	if ip == "0000:0000:0000:0000:0000:0000:0000:0000" {
		return "::"
	}
	// For IPv4-mapped IPv6 (::ffff:x.x.x.x), show the IPv4 part
	if groups[0] == 0 && groups[1] == 0 && groups[2] == 0x0000FFFF {
		return fmt.Sprintf("::ffff:%d.%d.%d.%d",
			(groups[3]>>24)&0xFF,
			(groups[3]>>16)&0xFF,
			(groups[3]>>8)&0xFF,
			groups[3]&0xFF,
		)
	}

	return ip
}

// parseQueueSizes parses the tx_queue:rx_queue field (hex:hex).
// Example: "00000100:00000000" → (256, 0)
func parseQueueSizes(field string) (int64, int64) {
	parts := strings.Split(field, ":")
	if len(parts) != 2 {
		return 0, 0
	}

	tx, _ := strconv.ParseInt(parts[0], 16, 64)
	rx, _ := strconv.ParseInt(parts[1], 16, 64)
	return tx, rx
}

// buildInodeToPIDMap scans /proc/[pid]/fd/ to build a map from socket inode → PID.
//
// How it works:
//  1. List all /proc/[pid] directories (numeric names = process IDs)
//  2. For each PID, list /proc/[pid]/fd/ (file descriptors)
//  3. Each fd is a symlink; socket fds point to "socket:[12345]" where 12345 is the inode
//  4. Match the inode number to our Connection.Inode field
//
// Requires root to read other processes' fd directories.
// Errors are silently ignored (permission denied, process exited, etc.)
func buildInodeToPIDMap() map[string]int {
	result := make(map[string]int)

	// List all /proc entries
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return result
	}

	for _, entry := range entries {
		// Only process numeric directory names (PIDs)
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		// List all file descriptors for this PID
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // Permission denied or process exited
		}

		for _, fd := range fds {
			// Read the symlink target, e.g. "socket:[12345]"
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}

			// Extract inode from "socket:[12345]"
			if strings.HasPrefix(link, "socket:[") && strings.HasSuffix(link, "]") {
				inode := link[8 : len(link)-1] // Strip "socket:[" and "]"
				result[inode] = pid
			}
		}
	}

	return result
}

// getProcessName reads the process name from /proc/[pid]/comm.
// Returns empty string if the process no longer exists or can't be read.
func getProcessName(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
