package weft

import (
	"regexp"
	"testing"
)

var hexTraceIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestNewTraceID_ShapeAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewTraceID()
		if !hexTraceIDRe.MatchString(id) {
			t.Fatalf("NewTraceID() = %q, want 32 lowercase hex chars", id)
		}
		if allZero(id) {
			t.Fatalf("NewTraceID() = %q, want not all-zero", id)
		}
		if seen[id] {
			t.Fatalf("NewTraceID() produced a duplicate: %q", id)
		}
		seen[id] = true
	}
}

func TestFormatTraceParent(t *testing.T) {
	got := FormatTraceParent("0123456789abcdef0123456789abcdef")
	want := "00-0123456789abcdef0123456789abcdef-0000000000000000-01"
	if got != want {
		t.Errorf("FormatTraceParent = %q, want %q", got, want)
	}
}

func TestParseTraceParent(t *testing.T) {
	const validID = "0123456789abcdef0123456789abcdef"
	cases := []struct {
		name      string
		header    string
		wantID    string
		wantOK    bool
	}{
		{"valid", "00-" + validID + "-0000000000000000-01", validID, true},
		{"empty", "", "", false},
		{"wrong segment count", "00-" + validID + "-01", "", false},
		{"wrong trace-id length", "00-abcd-0000000000000000-01", "", false},
		{"wrong parent-id length", "00-" + validID + "-0000-01", "", false},
		{"wrong version length", "0-" + validID + "-0000000000000000-01", "", false},
		{"wrong flags length", "00-" + validID + "-0000000000000000-1", "", false},
		{"uppercase hex rejected", "00-" + "0123456789ABCDEF0123456789ABCDEF" + "-0000000000000000-01", "", false},
		{"non-hex chars", "00-" + "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz" + "-0000000000000000-01", "", false},
		{"all-zero trace-id", "00-00000000000000000000000000000000-0000000000000000-01", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, ok := ParseTraceParent(c.header)
			if ok != c.wantOK || id != c.wantID {
				t.Errorf("ParseTraceParent(%q) = (%q, %v), want (%q, %v)", c.header, id, ok, c.wantID, c.wantOK)
			}
		})
	}
}
