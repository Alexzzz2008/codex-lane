//go:build windows

package platform

import (
	"errors"
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
			if errors.Is(err, registry.ErrNotExist) {
				return nil
			}
			return err
		}
		defer key.Close()
		err = key.DeleteValue(autoStartValue)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, autoStartPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(autoStartValue, `"`+exe+`"`)
}
