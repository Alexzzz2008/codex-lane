package platform

import (
	"fmt"
	"runtime"
)

func OpenURL(value string) error {
	switch runtime.GOOS {
	case "windows":
		return StartProcess("rundll32.exe", "url.dll,FileProtocolHandler", value)
	case "darwin":
		return StartProcess("open", value)
	case "linux":
		return StartProcess("xdg-open", value)
	default:
		return fmt.Errorf("opening URLs is not supported on %s", runtime.GOOS)
	}
}
