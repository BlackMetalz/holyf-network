//go:build !linux

package kernelapi

// Detect returns empty capabilities on non-Linux platforms.
func Detect() Capabilities { return Capabilities{} }
