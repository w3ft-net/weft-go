//go:build linux

package weft

import "os"

// openFDs returns the number of open file descriptors held by the
// current process, read from /proc/self/fd. Linux-only because
// the rest of weft is Linux-only by design.
func openFDs() int {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0
	}
	return len(entries)
}
