//go:build windows

package platform

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	autoStartPath  = `Software\Microsoft\Windows\CurrentVersion\Run`
	autoStartValue = "CodexLane"
)

func IsAutoStart() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, autoStartPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetStringValue(autoStartValue)
	return err == nil && value != ""
}

func SetAutoStart(enable bool) error {
	if !enable {
		key, err := registry.OpenKey(registry.CURRENT_USER, autoStartPath, registry.SET_VALUE)
		if err != nil {
			return nil
		}
		defer key.Close()
		return key.DeleteValue(autoStartValue)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, autoStartPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(autoStartValue, `"`+exe+`"`)
}
