//go:build !linux

package kernelapi

// NewSocketManager returns the exec-based SocketManager on non-Linux platforms.
func NewSocketManager() SocketManager { return NewExecSocketManager() }

// NewConntrackManager returns the exec-based ConntrackManager on non-Linux platforms.
func NewConntrackManager() ConntrackManager { return NewExecConntrackManager() }

// NewFirewall returns the exec-based Firewall on non-Linux platforms.
func NewFirewall() Firewall { return NewExecFirewall() }
