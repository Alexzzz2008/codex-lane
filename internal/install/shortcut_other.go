//go:build !windows

package install

import "errors"

func DesktopDirectory() (string, error) {
	return "", errors.New("仅支持 Windows")
}

func ShortcutPath() (string, error) {
	return "", errors.New("仅支持 Windows")
}

func CreateShortcut(string) (string, error) {
	return "", errors.New("仅支持 Windows")
}
