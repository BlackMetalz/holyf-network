package network

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// ListInterfaces returns a list of network interface names, excluding loopback.
// On Linux, it reads /sys/class/net/. On other OS, it uses the net package.
func ListInterfaces() ([]string, error) {
	// Try Linux-specific path first (more reliable for our use case)
	entries, err := os.ReadDir("/sys/class/net")
	if err == nil {
		var interfaces []string
		for _, entry := range entries {
			name := entry.Name()
			if name == "lo" {
				continue // Skip loopback
			}
			interfaces = append(interfaces, name)
		}
		if len(interfaces) == 0 {
			return nil, fmt.Errorf("no network interfaces found (excluding loopback)")
		}
		return interfaces, nil
	}

	// Fallback: use Go's net package (works cross-platform, e.g. macOS)
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list interfaces: %w", err)
	}

	var interfaces []string
	for _, iface := range ifaces {
		// Skip loopback and interfaces that are down
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		interfaces = append(interfaces, iface.Name)
	}

	if len(interfaces) == 0 {
		return nil, fmt.Errorf("no network interfaces found (excluding loopback)")
	}
	return interfaces, nil
}

// DetectDefaultInterface tries to find the interface used for the default route.
// On Linux, it parses /proc/net/route. Otherwise, falls back to first non-loopback interface.
func DetectDefaultInterface() (string, error) {
	// Try Linux-specific: parse /proc/net/route
	data, err := os.ReadFile("/proc/net/route")
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if i == 0 {
				continue // Skip header: "Iface Destination Gateway ..."
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			// Destination "00000000" means default route
			if fields[1] == "00000000" {
				return fields[0], nil
			}
		}
	}

	// Fallback: return first non-loopback interface
	interfaces, err := ListInterfaces()
	if err != nil {
		return "", fmt.Errorf("cannot detect default interface: %w", err)
	}
	return interfaces[0], nil
}
