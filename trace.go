package weft

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// NewTraceID returns a fresh 32-lowercase-hex-char trace ID (16
// random bytes via crypto/rand — not math/rand, since trace IDs are
// exact-match correlation keys across concurrent processes).
func NewTraceID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("weft: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// FormatTraceParent renders traceID as a W3C traceparent header
// value. weft has no span model: parent-id is always the all-zero
// placeholder, flags is always "01" (sampled).
func FormatTraceParent(traceID string) string {
	return "00-" + traceID + "-0000000000000000-01"
}

// ParseTraceParent extracts the trace-id segment from an inbound W3C
// traceparent header. ok=false on empty/malformed input (wrong
// segment count, wrong lengths, non-hex, uppercase, or an all-zero
// trace-id) — callers should fall back to NewTraceID rather than
// adopt a value that fails validation. version/parent-id/flags are
// validated for shape but otherwise ignored; weft is not a
// span-aware receiver.
func ParseTraceParent(header string) (traceID string, ok bool) {
	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return "", false
	}
	version, tid, parentID, flags := parts[0], parts[1], parts[2], parts[3]
	if len(version) != 2 || len(tid) != 32 || len(parentID) != 16 || len(flags) != 2 {
		return "", false
	}
	if !isLowerHex(version) || !isLowerHex(tid) || !isLowerHex(parentID) || !isLowerHex(flags) {
		return "", false
	}
	if allZero(tid) {
		return "", false
	}
	return tid, true
}

func isLowerHex(s string) bool {
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func allZero(s string) bool {
	for _, r := range s {
		if r != '0' {
			return false
		}
	}
	return true
}
