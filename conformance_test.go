package weft

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
)

const conformanceDir = "testdata/conformance"

func loadVectors(t *testing.T, name string, v any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(conformanceDir, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

func TestConformance_NoUnknownFiles(t *testing.T) {
	known := map[string]bool{
		"header.json": true, "traceparent.json": true, "socket_frame.json": true,
		"reserved_keys.json": true, "service_names.json": true, "grpc_metadata.json": true,
	}
	entries, err := os.ReadDir(conformanceDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") && !known[e.Name()] {
			t.Errorf("%s has no conformance runner", e.Name())
		}
	}
	for name := range known {
		if _, err := os.Stat(filepath.Join(conformanceDir, name)); err != nil {
			t.Errorf("missing %s", name)
		}
	}
}

func TestConformance_Header(t *testing.T) {
	var cases []struct {
		Name  string `json:"name"`
		Input struct {
			TsNs     int64  `json:"ts_ns"`
			Level    string `json:"level"`
			TraceID  string `json:"trace_id"`
			SpanName string `json:"span_name"`
			SpanID   string `json:"span_id"`
			ParentID string `json:"parent_id"`
			DurNs    int64  `json:"dur_ns"`
			Fmt      string `json:"fmt"`
			Kind     string `json:"kind"`
			Body     string `json:"body"`
		} `json:"input"`
		Expect string `json:"expect"`
	}
	loadVectors(t, "header", &cases)
	for _, c := range cases {
		in := c.Input
		h := lineHeader{
			ts: time.Unix(0, in.TsNs), level: in.Level, traceID: in.TraceID, spanName: in.SpanName,
			spanID: in.SpanID, parentID: in.ParentID, durNs: in.DurNs, format: in.Fmt, kind: in.Kind,
		}
		var got strings.Builder
		lf := &LogFile{w: &got}
		if err := lf.writeLine(h, []byte(in.Body)); err != nil {
			t.Fatal(err)
		}
		if got.String() != c.Expect+"\n" {
			t.Errorf("%s: got %q, want %q", c.Name, got.String(), c.Expect+"\n")
		}
	}
}

func TestConformance_Traceparent(t *testing.T) {
	var v struct {
		Parse []struct {
			Name   string `json:"name"`
			Header string `json:"header"`
			Expect *struct {
				TraceID string `json:"trace_id"`
				SpanID  string `json:"span_id"`
			} `json:"expect"`
		} `json:"parse"`
		Format []struct {
			Name    string `json:"name"`
			TraceID string `json:"trace_id"`
			SpanID  string `json:"span_id"`
			Expect  string `json:"expect"`
		} `json:"format"`
	}
	loadVectors(t, "traceparent", &v)
	for _, c := range v.Parse {
		traceID, spanID, ok := ParseTraceParent(c.Header)
		if c.Expect == nil {
			if ok {
				t.Errorf("%s: parsed %q, want rejection", c.Name, c.Header)
			}
			continue
		}
		if !ok || traceID != c.Expect.TraceID || spanID != c.Expect.SpanID {
			t.Errorf("%s: got (%q, %q, %v), want (%q, %q, true)", c.Name, traceID, spanID, ok, c.Expect.TraceID, c.Expect.SpanID)
		}
	}
	for _, c := range v.Format {
		if got := FormatTraceParent(c.TraceID, c.SpanID); got != c.Expect {
			t.Errorf("%s: got %q, want %q", c.Name, got, c.Expect)
		}
	}
}

func TestConformance_SocketFrame(t *testing.T) {
	var cases []struct {
		Name  string `json:"name"`
		Input struct {
			App     string  `json:"app"`
			Line    string  `json:"line"`
			TraceID *string `json:"trace_id"`
		} `json:"input"`
		ExpectFrame string `json:"expect_frame"`
		ExpectError bool   `json:"expect_error"`
	}
	loadVectors(t, "socket_frame", &cases)
	for _, c := range cases {
		fs := newFakeSocket(t)
		tr := newSocketTransport(fs.path)
		ctx := context.Background()
		if c.Input.TraceID != nil {
			ctx = ContextWithTraceID(ctx, *c.Input.TraceID)
		}
		err := tr.sendLine(ctx, c.Input.App, c.Input.Line)
		if c.ExpectError {
			if err == nil {
				t.Errorf("%s: want an error", c.Name)
			}
			time.Sleep(20 * time.Millisecond)
			if got := fs.snapshot(); len(got) != 0 {
				t.Errorf("%s: rejected line was written: %q", c.Name, got)
			}
			tr.close()
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.Name, err)
			tr.close()
			continue
		}
		fs.waitForN(t, 1)
		if got := fs.snapshot()[0]; got != strings.TrimSuffix(c.ExpectFrame, "\n") {
			t.Errorf("%s: got %q, want %q", c.Name, got, c.ExpectFrame)
		}
		tr.close()
	}
}

type reservedCase struct {
	Name         string         `json:"name"`
	Stream       string         `json:"stream"`
	Service      string         `json:"service"`
	Extra        map[string]any `json:"extra"`
	Expect       map[string]any `json:"expect"`
	Present      []string       `json:"present"`
	TypePattern  string         `json:"type_pattern"`
	AbsentValues map[string]any `json:"absent_values"`
}

func TestConformance_ReservedKeys(t *testing.T) {
	var v struct {
		TsPattern string         `json:"ts_pattern"`
		Cases     []reservedCase `json:"cases"`
	}
	loadVectors(t, "reserved_keys", &v)
	tsRe := regexp.MustCompile(v.TsPattern)
	for _, c := range v.Cases {
		fs := newFakeSocket(t)
		cl, err := New(WithSocket(fs.path))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		switch c.Stream {
		case "heartbeat":
			go cl.HeartbeatWithFields(ctx, c.Service, time.Hour, c.Extra)
		case "runtime":
			cl.emitRuntimeStats(ctx, c.Service+"/runtime", c.Service, time.Now(), c.Extra)
		default:
			t.Fatalf("%s: unknown stream %q", c.Name, c.Stream)
		}
		fs.waitForN(t, 1)
		cancel()
		cl.Close()
		frame := fs.snapshot()[0]
		payload := map[string]any{}
		if err := json.Unmarshal([]byte(frame[strings.Index(frame, ":")+1:]), &payload); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		for k, want := range c.Expect {
			if !reflect.DeepEqual(payload[k], want) {
				t.Errorf("%s: %s = %v, want %v", c.Name, k, payload[k], want)
			}
		}
		for _, k := range c.Present {
			if _, ok := payload[k]; !ok {
				t.Errorf("%s: missing %s", c.Name, k)
			}
		}
		for k, bad := range c.AbsentValues {
			if reflect.DeepEqual(payload[k], bad) {
				t.Errorf("%s: %s kept the caller's value %v", c.Name, k, bad)
			}
		}
		if ts, _ := payload["ts"].(string); !tsRe.MatchString(ts) {
			t.Errorf("%s: ts %q is not RFC 3339 UTC", c.Name, ts)
		}
		if c.TypePattern != "" {
			if typ, _ := payload["type"].(string); !regexp.MustCompile(c.TypePattern).MatchString(typ) {
				t.Errorf("%s: type %q does not match %s", c.Name, typ, c.TypePattern)
			}
		}
	}
}

func TestConformance_ServiceNames(t *testing.T) {
	var v struct {
		Valid   []string `json:"valid"`
		Invalid []string `json:"invalid"`
	}
	loadVectors(t, "service_names", &v)
	for _, s := range v.Valid {
		if !validService(s) {
			t.Errorf("validService(%q) = false, want true", s)
		}
	}
	for _, s := range v.Invalid {
		if validService(s) {
			t.Errorf("validService(%q) = true, want false", s)
		}
	}
}

func sortedPairs(md metadata.MD) [][2]string {
	var out [][2]string
	for k, vs := range md {
		for _, v := range vs {
			out = append(out, [2]string{k, v})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

func pairsToMD(pairs [][2]string) metadata.MD {
	md := metadata.MD{}
	for _, p := range pairs {
		md.Append(p[0], p[1])
	}
	return md
}

func TestConformance_GRPCMetadata(t *testing.T) {
	var v struct {
		Outgoing []struct {
			Name string `json:"name"`
			Ctx  struct {
				TraceID string `json:"trace_id"`
				SpanID  string `json:"span_id"`
			} `json:"ctx"`
			Metadata [][2]string `json:"metadata"`
			Expect   [][2]string `json:"expect"`
		} `json:"outgoing"`
		Incoming []struct {
			Name     string      `json:"name"`
			Metadata [][2]string `json:"metadata"`
			Expect   *struct {
				TraceID string  `json:"trace_id"`
				SpanID  *string `json:"span_id"`
			} `json:"expect"`
		} `json:"incoming"`
	}
	loadVectors(t, "grpc_metadata", &v)
	for _, c := range v.Outgoing {
		ctx := context.Background()
		if c.Ctx.TraceID != "" {
			ctx = ContextWithTraceID(ctx, c.Ctx.TraceID)
		}
		if c.Ctx.SpanID != "" {
			ctx = ContextWithSpanID(ctx, c.Ctx.SpanID)
		}
		if len(c.Metadata) > 0 {
			ctx = metadata.NewOutgoingContext(ctx, pairsToMD(c.Metadata))
		}
		md, _ := metadata.FromOutgoingContext(GRPC.Outgoing(ctx))
		got, want := sortedPairs(md), sortedPairs(pairsToMD(c.Expect))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", c.Name, got, want)
		}
	}
	for _, c := range v.Incoming {
		ctx := metadata.NewIncomingContext(context.Background(), pairsToMD(c.Metadata))
		ctx = GRPC.Incoming(ctx)
		traceID, hasTrace := TraceIDFromContext(ctx)
		spanID, hasSpan := CurrentSpanID(ctx)
		if c.Expect == nil {
			if hasTrace || hasSpan {
				t.Errorf("%s: got trace %q span %q, want neither", c.Name, traceID, spanID)
			}
			continue
		}
		if !hasTrace || traceID != c.Expect.TraceID {
			t.Errorf("%s: trace = %q, want %q", c.Name, traceID, c.Expect.TraceID)
		}
		switch {
		case c.Expect.SpanID == nil && hasSpan:
			t.Errorf("%s: span = %q, want none", c.Name, spanID)
		case c.Expect.SpanID != nil && (!hasSpan || spanID != *c.Expect.SpanID):
			t.Errorf("%s: span = %q, want %q", c.Name, spanID, *c.Expect.SpanID)
		}
	}
}
