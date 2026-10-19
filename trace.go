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

// FormatTraceParent renders traceID and spanID as a W3C traceparent
// header value — the wire format's "parent-id" segment is spec'd to
// carry the sending span's own ID (it only reads as "parent" once
// the receiving end adopts it as the parent of whatever span it
// starts), so the parameter here is named for what the caller is
// actually handing over: its own ambient span ID. Empty spanID
// (no span active — e.g. nothing has called StartSpan yet) renders
// the all-zero placeholder. flags is always "01" (sampled).
func FormatTraceParent(traceID, spanID string) string {
	if spanID == "" {
		spanID = "0000000000000000"
	}
	return "00-" + traceID + "-" + spanID + "-01"
}

// ParseTraceParent extracts the trace-id and span-id segments from
// an inbound W3C traceparent header. ok=false on empty/malformed
// input (wrong segment count, wrong lengths, non-hex, uppercase, or
// an all-zero trace-id) — callers should fall back to NewTraceID
// rather than adopt a value that fails validation. An all-zero
// span-id is a normal, valid case (the caller has no span of its
// own) and comes back as "" — callers seed it via ContextWithSpanID
// the same way an absent one leaves the ambient span unset; version/
// flags are validated for shape but otherwise ignored.
func ParseTraceParent(header string) (traceID, spanID string, ok bool) {
	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return "", "", false
	}
	version, tid, sid, flags := parts[0], parts[1], parts[2], parts[3]
	if len(version) != 2 || len(tid) != 32 || len(sid) != 16 || len(flags) != 2 {
		return "", "", false
	}
	if !isLowerHex(version) || !isLowerHex(tid) || !isLowerHex(sid) || !isLowerHex(flags) {
		return "", "", false
	}
	if allZero(tid) {
		return "", "", false
	}
	if allZero(sid) {
		sid = ""
	}
	return tid, sid, true
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
