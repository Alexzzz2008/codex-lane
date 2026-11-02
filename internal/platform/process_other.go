//go:build !windows

package platform

func FindProcessByLocalPort(port int) string { return "" }
