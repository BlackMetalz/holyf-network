package collector

import (
	"fmt"
	"os"
	"strings"
)

const tcpListenStateHex = "0A"

// CollectListenPorts returns local TCP ports currently in LISTEN state.
func CollectListenPorts() (map[int]struct{}, error) {
	files := []string{"/proc/net/tcp", "/proc/net/tcp6"}
	listenPorts := make(map[int]struct{})
	parsed := false

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		isIPv6 := strings.Contains(file, "tcp6")
		for port := range parseListenPorts(string(content), isIPv6) {
			listenPorts[port] = struct{}{}
		}
		parsed = true
	}

	if !parsed {
		return nil, fmt.Errorf("cannot read /proc/net/tcp listen sockets (requires Linux)")
	}

	return listenPorts, nil
}

func parseListenPorts(content string, isIPv6 bool) map[int]struct{} {
	ports := make(map[int]struct{})
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		if i == 0 {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if fields[3] != tcpListenStateHex {
			continue
		}

		_, localPort, err := parseHexAddress(fields[1], isIPv6)
		if err != nil {
			continue
		}
		ports[localPort] = struct{}{}
	}

	return ports
}
