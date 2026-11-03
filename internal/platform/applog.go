package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const appLogMaxBytes = 1 << 20

var appLogMu sync.Mutex

// AppLog appends a startup and lifecycle line to
// %LOCALAPPDATA%\CodexLane\logs\app.log. It is best effort: logging never
// interrupts the application.
func AppLog(format string, args ...any) {
	appLogMu.Lock()
	defer appLogMu.Unlock()

	directory := filepath.Join(instanceDataDir(), "logs")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return
	}
	path := filepath.Join(directory, "app.log")
	if info, err := os.Stat(path); err == nil && info.Size() > appLogMaxBytes {
		_ = os.Rename(path, filepath.Join(directory, "app.previous.log"))
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	line := fmt.Sprintf("%s [%d] %s\n", time.Now().Format("2006-01-02 15:04:05.000"), os.Getpid(), fmt.Sprintf(format, args...))
	_, _ = file.WriteString(line)
}
