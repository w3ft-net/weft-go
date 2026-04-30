//go:build !linux

package weft

// openFDs is a non-Linux stub. The /proc/self/fd technique only
// works on Linux; rather than pretend to a number we return 0 and
// let consumers spot it as missing data. Tests on macOS dev boxes
// see this stub.
func openFDs() int { return 0 }
