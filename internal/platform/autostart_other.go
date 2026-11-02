//go:build !windows

package platform

func IsAutoStart() bool { return false }

func SetAutoStart(enable bool) error { return nil }
