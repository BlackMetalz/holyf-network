//go:build linux

package kernelapi

import (
	"strings"

	"github.com/google/nftables"
	"golang.org/x/sys/unix"
)

// Detect probes which kernel networking APIs are available at runtime.
func Detect() Capabilities {
	var caps Capabilities

	// HasNetlinkSockDiag: try opening a NETLINK_SOCK_DIAG socket.
	if fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM, unix.NETLINK_SOCK_DIAG); err == nil {
		unix.Close(fd)
		caps.HasNetlinkSockDiag = true
	}

	// HasSockDestroy: available since kernel 4.9.
	caps.HasSockDestroy = kernelVersionAtLeast(4, 9)

	// HasNfConntrack: try opening a NETLINK_NETFILTER socket.
	if fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM, unix.NETLINK_NETFILTER); err == nil {
		unix.Close(fd)
		caps.HasNfConntrack = true
	}

	// HasNftables: try creating an nftables netlink connection.
	caps.HasNftables = probeNftables()

	return caps
}

// kernelVersionAtLeast returns true if the running kernel is >= major.minor.
func kernelVersionAtLeast(major, minor int) bool {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		return false
	}

	release := unix.ByteSliceToString(utsname.Release[:])
	// release is like "5.15.0-100-generic"
	var kmajor, kminor int
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return false
	}
	for _, c := range parts[0] {
		if c >= '0' && c <= '9' {
			kmajor = kmajor*10 + int(c-'0')
		}
	}
	for _, c := range parts[1] {
		if c >= '0' && c <= '9' {
			kminor = kminor*10 + int(c-'0')
		}
	}

	if kmajor > major {
		return true
	}
	return kmajor == major && kminor >= minor
}

// probeNftables tests whether the nftables netlink API is reachable.
func probeNftables() bool {
	c, err := nftables.New()
	if err != nil {
		return false
	}
	// Try a harmless list-tables operation. If the kernel or permissions
	// prevent it, we treat nftables as unavailable.
	_, err = c.ListTables()
	return err == nil
}
