package weft

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// parsedLine is one headered line, split the way weftd reads it.
type parsedLine struct {
	version, ts, level, traceID, spanName, spanID, parentID, durNs, format, kind string
	body                                                                         map[string]any
	rawBody                                                                      string
}

func parseLine(t *testing.T, line string) parsedLine {
	t.Helper()
	if !strings.HasPrefix(line, "\x1e") {
		t.Fatalf("line %q has no header", line)
	}
	end := strings.Index(line[1:], "\x1e")
	if end < 0 {
		t.Fatalf("line %q: header not closed", line)
	}
	v := strings.Split(line[1:1+end], "\x1f")
	if len(v) != 10 {
		t.Fatalf("header has %d values, want 10: %q", len(v), v)
	}
	p := parsedLine{version: v[0], ts: v[1], level: v[2], traceID: v[3], spanName: v[4],
		spanID: v[5], parentID: v[6], durNs: v[7], format: v[8], kind: v[9], rawBody: line[2+end:]}
	if p.format == "json" {
		if err := json.Unmarshal([]byte(p.rawBody), &p.body); err != nil {
			t.Fatalf("json body %q: %v", p.rawBody, err)
		}
	}
	return p
}

// captureLines runs fn with slog's default logger writing headered lines to a
// buffer, and returns the lines.
func captureLines(t *testing.T, fn func()) []parsedLine {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewLogHandler(&buf, nil)))
	defer slog.SetDefault(prev)
	fn()
	var out []parsedLine
	for _, l := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if l != "" {
			out = append(out, parseLine(t, l))
		}
	}
	return out
}

func TestLogHandler_HeaderCarriesFields(t *testing.T) {
	ctx := ContextWithSpanID(ContextWithTraceID(context.Background(), "abc123"), "feedface00000000")
	lines := captureLines(t, func() {
		slog.WarnContext(ctx, "disk low", "pct", 91)
	})
	if len(lines) != 1 {
		t.Fatalf("got %d lines", len(lines))
	}
	l := lines[0]
	if l.version != "v1" || l.level != "WARN" || l.traceID != "abc123" || l.spanID != "feedface00000000" || l.format != "json" || l.kind != "" {
		t.Errorf("header = %+v", l)
	}
	if l.ts == "" {
		t.Error("ts empty, want unix nanoseconds")
	}
	if l.body["msg"] != "disk low" || l.body["pct"] != float64(91) {
		t.Errorf("body = %v", l.body)
	}
	for _, k := range []string{"time", "level", "trace_id"} {
		if _, ok := l.body[k]; ok {
			t.Errorf("body repeats %q, which the header carries", k)
		}
	}
}

func TestLogHandler_NoTraceWithoutContext(t *testing.T) {
	lines := captureLines(t, func() { slog.Info("hello") })
	if lines[0].traceID != "" || lines[0].spanID != "" {
		t.Errorf("header = %+v, want no trace or span", lines[0])
	}
}

func TestLogHandler_WithAttrsAndGroups(t *testing.T) {
	lines := captureLines(t, func() {
		slog.Default().With("k", "v").WithGroup("g").Info("hi", "x", 1)
	})
	b := lines[0].body
	if b["k"] != "v" {
		t.Errorf("body = %v, want k=v", b)
	}
	if g, _ := b["g"].(map[string]any); g["x"] != float64(1) {
		t.Errorf("body = %v, want g.x=1", b)
	}
}

func TestLogHandler_MessageNewlinesStayOnOneLine(t *testing.T) {
	var buf bytes.Buffer
	slog.New(NewLogHandler(&buf, nil)).Info("a\nb")
	if n := strings.Count(buf.String(), "\n"); n != 1 {
		t.Errorf("output has %d newlines, want 1: %q", n, buf.String())
	}
}

func TestLogHandler_RespectsLevelOption(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(NewLogHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	l.Info("quiet")
	l.Warn("loud")
	if strings.Contains(buf.String(), "quiet") || !strings.Contains(buf.String(), "loud") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestLineHeader_DropsUnsafeValues(t *testing.T) {
	b := lineHeader{traceID: "a\x1eb", level: "INFO", spanName: "x\x1fy"}.appendTo(nil)
	if got := strings.Count(string(b), "\x1e"); got != 2 {
		t.Errorf("header has %d \\x1e, want exactly the two brackets: %q", got, b)
	}
	if got := strings.Count(string(b), "\x1f"); got != 9 {
		t.Errorf("header has %d \\x1f, want 9: %q", got, b)
	}
}

func TestInitLogging_NoPathLogsJSONWithTraceIDToFallback(t *testing.T) {
	t.Setenv(EnvLogFile, "")
	defer slog.SetDefault(slog.Default())
	var buf bytes.Buffer
	lf := InitLogging("", &buf)
	if lf != nil {
		t.Fatalf("InitLogging = %v; want nil", lf)
	}
	slog.InfoContext(ContextWithTraceID(context.Background(), "abc"), "hi", "k", "v")
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output %q: %v", buf.String(), err)
	}
	if rec["msg"] != "hi" || rec["trace_id"] != "abc" || rec["k"] != "v" {
		t.Errorf("record = %v", rec)
	}
}

func TestInitLogging_WritesFileAndRoutesStdlibLog(t *testing.T) {
	path := t.TempDir() + "/app.log"
	defer slog.SetDefault(slog.Default())
	lf := InitLogging(path, io.Discard)
	if lf == nil {
		t.Fatal("no log file")
	}
	defer lf.Close()
	slog.Info("from slog")
	stdlibLog("from log")
	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("file has %d lines: %q", len(lines), data)
	}
	for i, want := range []string{"from slog", "from log"} {
		if got := parseLine(t, lines[i]).body["msg"]; got != want {
			t.Errorf("line %d msg = %v, want %q", i, got, want)
		}
	}
}

func TestInitLogging_UsesEnv(t *testing.T) {
	path := t.TempDir() + "/env.log"
	t.Setenv(EnvLogFile, path)
	defer slog.SetDefault(slog.Default())
	lf := InitLogging("", io.Discard)
	if lf == nil {
		t.Fatal("no log file")
	}
	defer lf.Close()
	slog.Info("x")
	if data, _ := os.ReadFile(path); len(data) == 0 {
		t.Error("env-named file is empty")
	}
}

func TestCapture_FileLinesBecomeKindedText(t *testing.T) {
	var buf bytes.Buffer
	lf := &LogFile{w: &buf}
	orig := os.Stdout
	restore, err := lf.captureFile(&os.Stdout, "stdout")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(os.Stdout, "first\nsecond\n")
	restore()
	if os.Stdout != orig {
		t.Error("restore did not put os.Stdout back")
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines: %q", len(lines), buf.String())
	}
	for i, want := range []string{"first", "second"} {
		p := parseLine(t, lines[i])
		if p.kind != "stdout" || p.format != "text" || p.level != "" || p.traceID != "" || p.rawBody != want {
			t.Errorf("line %d = %+v, want text/stdout %q", i, p, want)
		}
	}
}

func stdlibLog(msg string) { log.Print(msg) }

func TestKindHandler_SetsKindInHeader(t *testing.T) {
	var buf bytes.Buffer
	lf := &LogFile{w: &buf}
	slog.New(lf.KindHandler("queries", nil)).Info("query: x")
	slog.New(lf.Handler(nil)).Info("plain")
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if k := parseLine(t, lines[0]).kind; k != "queries" {
		t.Errorf("kind = %q, want queries", k)
	}
	if k := parseLine(t, lines[1]).kind; k != "" {
		t.Errorf("kind = %q, want the source's own", k)
	}
}

func TestInitLogging_UnopenableFileFallsBackAndReports(t *testing.T) {
	defer slog.SetDefault(slog.Default())
	var buf bytes.Buffer
	if lf := InitLogging(t.TempDir()+"/missing-dir/x.log", &buf); lf != nil {
		t.Fatal("got a LogFile for an unopenable path")
	}
	if !strings.Contains(buf.String(), "open log file") {
		t.Errorf("fallback output = %q, want the open error", buf.String())
	}
	slog.Info("still logging")
	if !strings.Contains(buf.String(), "still logging") {
		t.Error("logging did not continue on the fallback")
	}
}

func TestNilLogFileIsInert(t *testing.T) {
	var lf *LogFile
	if err := lf.CaptureStdio(); err != nil {
		t.Error(err)
	}
	if err := lf.Close(); err != nil {
		t.Error(err)
	}
}
