//go:build windows

package platform

import (
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProcessIDs lists running processes whose executable name matches one of
// names. It uses a toolhelp snapshot instead of PowerShell/WMI so it stays
// cheap enough to poll in the background.
func ProcessIDs(names ...string) map[int]string {
	wanted := make([][]uint16, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		encoded := windows.StringToUTF16(trimmed)
		wanted = append(wanted, encoded[:len(encoded)-1])
	}
	found := make(map[int]string)
	if len(wanted) == 0 {
		return found
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return found
	}
	defer windows.CloseHandle(snapshot)

	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		for _, candidate := range wanted {
			if equalFoldUTF16(&entry.ExeFile, candidate) {
				found[int(entry.ProcessID)] = windows.UTF16ToString(entry.ExeFile[:])
				break
			}
		}
	}
	return found
}

// equalFoldUTF16 compares a fixed-size process name buffer with a UTF-16 name
// using ASCII case folding, which avoids allocating a string per process.
func equalFoldUTF16(buffer *[windows.MAX_PATH]uint16, want []uint16) bool {
	limit := len(want)
	for index := 0; index < limit; index++ {
		value := buffer[index]
		if value == 0 {
			return false
		}
		if asciiLower(value) != asciiLower(want[index]) {
			return false
		}
	}
	return buffer[limit] == 0
}

func asciiLower(value uint16) uint16 {
	if value >= 'A' && value <= 'Z' {
		return value + ('a' - 'A')
	}
	return value
}

// PIDSetKey renders a stable key for a process id set so callers can detect
// changes without sorting in place.
func PIDSetKey(ids map[int]string) string {
	if len(ids) == 0 {
		return ""
	}
	list := make([]int, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	sort.Ints(list)
	var builder strings.Builder
	for index, id := range list {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.Itoa(id))
	}
	return builder.String()
}
