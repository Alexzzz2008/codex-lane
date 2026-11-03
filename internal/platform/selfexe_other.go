//go:build !windows

package platform

// EnsureSelfVisible is a no-op outside Windows.
func EnsureSelfVisible() {}
