//go:build !linux

package weft

import "os"

// pointStdioAt is a no-op off Linux, where weftd does not run.
func pointStdioAt(*os.File) error { return nil }
