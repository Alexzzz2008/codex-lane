//go:build !windows

package platform

import (
	"context"
	"errors"
)

func WatchWindowProcesses(context.Context, chan<- int) error {
	return errors.New("窗口事件监听仅支持 Windows")
}

func ProcessExecutable(int) (string, error) {
	return "", errors.New("进程路径查询仅支持 Windows")
}
