//go:build windows

package platform

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	afInet                = 2
	tcpTableOwnerPIDAll   = 5
	processQueryLimitedIn = 0x1000
)

type mibTcpRowOwnerPID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwnerPID   uint32
}

var modiphlpapi = syscall.NewLazyDLL("iphlpapi.dll")
var procGetExtendedTcpTable = modiphlpapi.NewProc("GetExtendedTcpTable")

// FindProcessByLocalPort returns "name (pid)" for the process owning the
// given loopback TCP port, or an empty string when it cannot be resolved.
func FindProcessByLocalPort(port int) string {
	if port <= 0 || port > 65535 {
		return ""
	}
	size := uint32(16 * 1024)
	for attempts := 0; attempts < 3; attempts++ {
		buffer := make([]byte, size)
		err, _, _ := procGetExtendedTcpTable.Call(
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(unsafe.Pointer(&size)),
			0,
			uintptr(afInet),
			uintptr(tcpTableOwnerPIDAll),
			0,
		)
		if err == 0 {
			return lookupPIDInTable(buffer, uint32(port))
		}
		if err != uintptr(syscall.ERROR_INSUFFICIENT_BUFFER) {
			return ""
		}
	}
	return ""
}

func lookupPIDInTable(buffer []byte, port uint32) string {
	if len(buffer) < 4 {
		return ""
	}
	entries := binary.LittleEndian.Uint32(buffer)
	rowSize := int(unsafe.Sizeof(mibTcpRowOwnerPID{}))
	for index := uint32(0); index < entries; index++ {
		offset := 4 + int(index)*rowSize
		if offset+rowSize > len(buffer) {
			break
		}
		row := (*mibTcpRowOwnerPID)(unsafe.Pointer(&buffer[offset]))
		if localPort(row.LocalPort) != port || row.OwnerPID == 0 {
			continue
		}
		return processLabel(row.OwnerPID)
	}
	return ""
}

func localPort(networkOrder uint32) uint32 {
	return uint32(binary.BigEndian.Uint16([]byte{
		byte(networkOrder),
		byte(networkOrder >> 8),
	}))
}

func processLabel(pid uint32) string {
	handle, err := windows.OpenProcess(processQueryLimitedIn, false, pid)
	if err != nil {
		return fmt.Sprintf("PID %d", pid)
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 512)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return fmt.Sprintf("PID %d", pid)
	}
	return fmt.Sprintf("%s (%d)", filepath.Base(windows.UTF16ToString(buffer[:size])), pid)
}
