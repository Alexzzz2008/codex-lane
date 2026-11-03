//go:build !windows

package platform

import (
	"sort"
	"strconv"
	"strings"
)

// ProcessIDs is only implemented on Windows.
func ProcessIDs(...string) map[int]string { return map[int]string{} }

// PIDSetKey renders a stable key for a process id set.
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
