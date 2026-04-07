package podlookup

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BlackMetalz/holyf-network/internal/collector"
)

// EnumerateNetworkNamespaces scans /proc/*/ns/net and returns one
// representative PID per unique network namespace inode.
func EnumerateNetworkNamespaces() []NetNSEntry {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var result []NetNSEntry

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == 0 {
			continue
		}

		link, err := os.Readlink(fmt.Sprintf("/proc/%d/ns/net", pid))
		if err != nil {
			continue
		}

		// link is like "net:[4026532261]"
		if seen[link] {
			continue
		}
		seen[link] = true

		inode := link
		if strings.HasPrefix(link, "net:[") && strings.HasSuffix(link, "]") {
			inode = link[5 : len(link)-1]
		}

		result = append(result, NetNSEntry{Inode: inode, PID: pid})
	}

	return result
}

// FindPortOwner scans all network namespaces for a socket using the target port.
// It returns the first match with resolved pod info, or nil if not found.
func FindPortOwner(targetPort int) (*PodLookupResult, int) {
	namespaces := EnumerateNetworkNamespaces()

	for _, ns := range namespaces {
		result := findPortInNamespace(ns, targetPort)
		if result != nil {
			return result, len(namespaces)
		}
	}

	return nil, len(namespaces)
}

// findPortInNamespace reads /proc/{pid}/net/tcp{,6} and searches for targetPort.
func findPortInNamespace(ns NetNSEntry, targetPort int) *PodLookupResult {
	files := []string{
		fmt.Sprintf("/proc/%d/net/tcp", ns.PID),
		fmt.Sprintf("/proc/%d/net/tcp6", ns.PID),
	}

	for _, file := range files {
		conns, err := collector.ParseTCPConnections(file)
		if err != nil {
			continue
		}

		for _, conn := range conns {
			if conn.LocalPort != targetPort && conn.RemotePort != targetPort {
				continue
			}

			// Found a match. Resolve the owning PID via inode.
			ownerPID := resolveSocketOwnerPID(ns.PID, conn.Inode)
			if ownerPID == 0 {
				ownerPID = ns.PID
			}

			result := &PodLookupResult{
				PID:      ownerPID,
				ProcName: collector.GetProcessName(ownerPID),
				Port:     targetPort,
				LocalIP:  conn.LocalIP,
				State:    conn.State,
				NetNS:    fmt.Sprintf("net:[%s]", ns.Inode),
			}

			// Resolve pod info from cgroup/environ/crictl.
			podInfo := ResolvePodInfo(ownerPID)
			if podInfo != nil {
				result.ContainerID = podInfo.ContainerID
				result.PodName = podInfo.PodName
				result.PodNamespace = podInfo.PodNamespace
				result.Deployment = podInfo.Deployment
			}

			return result
		}
	}

	return nil
}

// resolveSocketOwnerPID finds the PID that owns a specific socket inode
// within the network namespace of nsPID.
func resolveSocketOwnerPID(nsPID int, targetInode string) int {
	if targetInode == "" || targetInode == "0" {
		return 0
	}

	// Get the netns inode for filtering.
	nsLink, err := os.Readlink(fmt.Sprintf("/proc/%d/ns/net", nsPID))
	if err != nil {
		return 0
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == 0 {
			continue
		}

		// Only scan PIDs in the same network namespace.
		pidNS, err := os.Readlink(fmt.Sprintf("/proc/%d/ns/net", pid))
		if err != nil || pidNS != nsLink {
			continue
		}

		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if link == "socket:["+targetInode+"]" {
				return pid
			}
		}
	}

	return 0
}
