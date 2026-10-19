//go:build linux

package weft

import (
	"os"
	"syscall"
)

// pointStdioAt makes fd 1 and fd 2 refer to f. Dup3 rather than Dup2 because
// linux/arm64 has no dup2.
func pointStdioAt(f *os.File) error {
	for _, fd := range []int{1, 2} {
		if err := syscall.Dup3(int(f.Fd()), fd, 0); err != nil {
			return err
		}
	}
	return nil
}
