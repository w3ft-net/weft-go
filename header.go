package weft

import (
	"strconv"
	"strings"
	"time"
)

// lineHeader is the structured prefix weftd reads off each line of an
// SDK-written file (header v1). weftd moves the values onto the row's own
// fields, so the log body never has to carry them.
//
//	\x1e v1 \x1f ts \x1f lvl \x1f trace \x1f span_name \x1f span_id \x1f
//	parent_id \x1f dur_ns \x1f fmt \x1f kind \x1e body
//
// The order is fixed by the version; later versions only append. An empty
// value means "not supplied".
type lineHeader struct {
	ts       time.Time
	level    string
	traceID  string
	spanName string
	spanID   string
	parentID string
	durNs    int64
	format   string // "json" or "text"
	kind     string // "" = the source's own kind
}

// headerSafe reports whether v can sit in a header value: the separators and
// the line terminator would corrupt the framing.
func headerSafe(v string) bool {
	return !strings.ContainsAny(v, "\x1e\x1f\n")
}

// appendTo appends the header to b. A value that is not header-safe is left
// empty rather than corrupting the line.
func (h lineHeader) appendTo(b []byte) []byte {
	val := func(v string) string {
		if headerSafe(v) {
			return v
		}
		return ""
	}
	b = append(b, 0x1e)
	b = append(b, "v1"...)
	b = append(b, 0x1f)
	if !h.ts.IsZero() {
		b = strconv.AppendInt(b, h.ts.UnixNano(), 10)
	}
	for _, v := range []string{h.level, h.traceID, h.spanName, h.spanID, h.parentID} {
		b = append(b, 0x1f)
		b = append(b, val(v)...)
	}
	b = append(b, 0x1f)
	if h.durNs > 0 {
		b = strconv.AppendInt(b, h.durNs, 10)
	}
	b = append(b, 0x1f)
	b = append(b, val(h.format)...)
	b = append(b, 0x1f)
	b = append(b, val(h.kind)...)
	return append(b, 0x1e)
}
