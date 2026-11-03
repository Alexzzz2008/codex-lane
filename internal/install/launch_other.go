//go:build !windows

package install

import "errors"

func Launch(string, string) error {
	return errors.New("仅支持 Windows")
}
