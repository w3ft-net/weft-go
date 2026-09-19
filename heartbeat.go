package weft

import "strings"

// validService mirrors the server-side rule: [a-zA-Z0-9._-]+, 1-128
// chars. Heartbeat validates the service name client-side so a bad
// name fails fast instead of producing a junk log path.
func validService(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' ||
			r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' ||
			r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return !strings.ContainsRune(s, ' ')
}
