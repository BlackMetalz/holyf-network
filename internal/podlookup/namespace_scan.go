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

// FindPortOwners scans every network namespace and returns one PodLookupResult
// per namespace that has at least one socket touching targetPort. Useful when a
// node hosts multiple pods that all interact with the same well-known port
// (e.g. several clients connecting to different MongoDB instances on 27017).
// The second return value is the total namespace count scanned.
func FindPortOwners(targetPort int) ([]*PodLookupResult, int) {
	namespaces := EnumerateNetworkNamespaces()

	var results []*PodLookupResult
	for _, ns := range namespaces {
		if result := findPortInNamespace(ns, targetPort); result != nil {
			results = append(results, result)
		}
	}

	return results, len(namespaces)
}

// findPortInNamespace reads /proc/{pid}/net/tcp{,6} and searches for targetPort.
// It collects every socket touching the port (as local or remote), picks the
// owner (preferring LISTEN, else first match where LocalPort == targetPort),
// and returns the rest as peer connections.
func findPortInNamespace(ns NetNSEntry, targetPort int) *PodLookupResult {
	files := []string{
		fmt.Sprintf("/proc/%d/net/tcp", ns.PID),
		fmt.Sprintf("/proc/%d/net/tcp6", ns.PID),
	}

	var matches []collector.Connection
	for _, file := range files {
		conns, err := collector.ParseAllTCPConnections(file)
		if err != nil {
			continue
		}
		for _, conn := range conns {
			if conn.LocalPort != targetPort && conn.RemotePort != targetPort {
				continue
			}
			matches = append(matches, conn)
		}
	}

	if len(matches) == 0 {
		return nil
	}

	// Pick the owner socket: LISTEN wins; else first socket with LocalPort == targetPort;
	// else fall back to the first match (client-side socket).
	ownerIdx := -1
	for i := range matches {
		if matches[i].State == "LISTEN" {
			ownerIdx = i
			break
		}
	}
	if ownerIdx < 0 {
		for i := range matches {
			if matches[i].LocalPort == targetPort {
				ownerIdx = i
				break
			}
		}
	}
	if ownerIdx < 0 {
		ownerIdx = 0
	}
	owner := matches[ownerIdx]

	ownerPID := resolveSocketOwnerPID(ns.PID, owner.Inode)
	if ownerPID == 0 {
		ownerPID = ns.PID
	}

	result := &PodLookupResult{
		PID:      ownerPID,
		ProcName: collector.GetProcessName(ownerPID),
		Port:     targetPort,
		LocalIP:  owner.LocalIP,
		State:    owner.State,
		NetNS:    fmt.Sprintf("net:[%s]", ns.Inode),
	}

	for i, c := range matches {
		if i == ownerIdx || c.State == "LISTEN" {
			continue
		}
		result.Peers = append(result.Peers, PeerConnection{
			LocalIP:    c.LocalIP,
			LocalPort:  c.LocalPort,
			RemoteIP:   c.RemoteIP,
			RemotePort: c.RemotePort,
			State:      c.State,
		})
	}

	// Resolve pod info from cgroup/environ/crictl/var-log.
	if podInfo := ResolvePodInfo(ownerPID); podInfo != nil {
		result.ContainerID = podInfo.ContainerID
		result.PodName = podInfo.PodName
		result.PodNamespace = podInfo.PodNamespace
		result.Deployment = podInfo.Deployment
	}

	return result
}

// resolveSocketOwnerPID finds the PID that owns a specific socket inode
// within the network namespace of nsPID.
// It tries the representative nsPID first (common case), then falls back
// to scanning all PIDs in the same namespace.
func resolveSocketOwnerPID(nsPID int, targetInode string) int {
	if targetInode == "" || targetInode == "0" {
		return 0
	}

	socketTarget := "socket:[" + targetInode + "]"

	// Fast path: check the representative nsPID first.
	if findSocketInPID(nsPID, socketTarget) {
		return nsPID
	}

	// Slow path: scan all PIDs in the same network namespace.
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
		if err != nil || pid == 0 || pid == nsPID {
			continue
		}

		pidNS, err := os.Readlink(fmt.Sprintf("/proc/%d/ns/net", pid))
		if err != nil || pidNS != nsLink {
			continue
		}

		if findSocketInPID(pid, socketTarget) {
			return pid
		}
	}

	return 0
}

func findSocketInPID(pid int, socketTarget string) bool {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	fds, err := os.ReadDir(fdDir)
	if err != nil {
		return false
	}
	for _, fd := range fds {
		link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
		if err != nil {
			continue
		}
		if link == socketTarget {
			return true
		}
	}
	return false
}
