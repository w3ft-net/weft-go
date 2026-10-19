package weft

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

// EnvLogFile names the file the weft SDK writes headered log lines to.
// Unset, the SDK writes no log file and logging is left as the program had it.
const EnvLogFile = "WEFT_LOG_FILE"

// LogFile is the file the SDK owns: every line in it carries a line header, so
// weftd can read timestamp, level, trace and span straight off it and the
// source needs no schema extraction. Nothing else should write to it.
type LogFile struct {
	mu sync.Mutex
	w  io.Writer
	f  *os.File // nil when built over a caller's writer
}

// InitLogging sets up logging for the process. Call it once, first thing in
// main.
//
// With a path (or $WEFT_LOG_FILE when path is empty) it opens the file and
// makes slog's default logger write headered lines to it, the trace and span
// of each call's ctx in the header. The stdlib log package is routed through
// slog, so it lands there too. Name the same file as a source in weftd's
// config. The returned LogFile can capture stdout and stderr and take a second
// stream (KindHandler).
//
// With neither set it logs JSON lines to fallback instead (stderr, usually),
// each with the ctx's trace ID as a trace_id attribute, and returns a nil
// LogFile.
//
// A file that cannot be opened is reported through the fallback and logging
// carries on there: a process should not die over its log destination.
// CaptureStdio and Close do nothing on the nil LogFile, so
// InitLogging("", os.Stderr).CaptureStdio() is a complete setup.
func InitLogging(path string, fallback io.Writer) *LogFile {
	if path == "" {
		path = os.Getenv(EnvLogFile)
	}
	var openErr error
	if path != "" {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
		if err == nil {
			lf := &LogFile{w: f, f: f}
			slog.SetDefault(slog.New(lf.Handler(nil)))
			return lf
		}
		openErr = err
	}
	slog.SetDefault(slog.New(traceStamp{slog.NewJSONHandler(fallback, nil)}))
	if openErr != nil {
		Errorf(context.Background(), "weft: open log file %s: %v", path, openErr)
	}
	return nil
}

// traceStamp adds the ctx's trace ID to each record, for output that has no
// line header to carry it.
type traceStamp struct{ slog.Handler }

func (h traceStamp) Handle(ctx context.Context, r slog.Record) error {
	if id, ok := TraceIDFromContext(ctx); ok {
		r.AddAttrs(slog.String("trace_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceStamp) WithAttrs(a []slog.Attr) slog.Handler { return traceStamp{h.Handler.WithAttrs(a)} }

func (h traceStamp) WithGroup(n string) slog.Handler { return traceStamp{h.Handler.WithGroup(n)} }

// NewLogHandler is a slog.Handler that writes headered lines to w. Use
// InitLogging for the file; this is for a writer you manage yourself.
func NewLogHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	return (&LogFile{w: w}).Handler(opts)
}

// Handler returns a slog.Handler writing to the file. The body is the record
// as JSON ({"msg": ..., attrs}); time, level, trace ID and span fields go in
// the header instead, so they are not repeated in it.
func (l *LogFile) Handler(opts *slog.HandlerOptions) slog.Handler {
	return l.KindHandler("", opts)
}

// KindHandler is Handler for a second stream in the same file: its lines carry
// kind in the header, so they read as /log/<ns>/<svc>/<kind>.log beside the
// service's own. kind must be a path segment (letters, digits, - and _) and
// not one weft reserves, or weftd ignores it and files the line under the
// source's own kind.
func (l *LogFile) KindHandler(kind string, opts *slog.HandlerOptions) slog.Handler {
	var o slog.HandlerOptions
	if opts != nil {
		o = *opts
	}
	user := o.ReplaceAttr
	o.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 && (a.Key == slog.TimeKey || a.Key == slog.LevelKey) {
			return slog.Attr{}
		}
		if user != nil {
			return user(groups, a)
		}
		return a
	}
	return &logHandler{file: l, opts: o, kind: kind}
}

// Close closes the file.
func (l *LogFile) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	return l.f.Close()
}

// writeLine writes one headered line in a single Write, so lines from
// different goroutines (and the O_APPEND fds beside them) do not interleave.
func (l *LogFile) writeLine(h lineHeader, body []byte) error {
	line := h.appendTo(make([]byte, 0, 96+len(body)))
	line = append(line, body...)
	line = append(line, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.w.Write(line)
	return err
}

// Attribute keys a Span uses to pass its fields to the handler, which moves
// them into the header. Another handler just prints them.
const (
	attrSpanName = "span_name"
	attrSpanID   = "span_id"
	attrParentID = "parent_span_id"
	attrDuration = "duration_ns"
)

type logHandler struct {
	file *LogFile
	opts slog.HandlerOptions
	kind string
	// derive replays the With* calls onto the per-record JSON handler.
	derive []func(slog.Handler) slog.Handler
}

func (h *logHandler) Enabled(_ context.Context, level slog.Level) bool {
	min := slog.LevelInfo
	if h.opts.Level != nil {
		min = h.opts.Level.Level()
	}
	return level >= min
}

func (h *logHandler) Handle(ctx context.Context, r slog.Record) error {
	hdr := lineHeader{ts: r.Time, level: r.Level.String(), format: "json", kind: h.kind}
	if hdr.ts.IsZero() {
		hdr.ts = time.Now()
	}
	hdr.traceID, _ = TraceIDFromContext(ctx)
	hdr.spanID, _ = CurrentSpanID(ctx)

	rest := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case attrSpanName:
			hdr.spanName = a.Value.String()
		case attrSpanID:
			hdr.spanID = a.Value.String()
		case attrParentID:
			hdr.parentID = a.Value.String()
		case attrDuration:
			hdr.durNs = a.Value.Int64()
		default:
			rest.AddAttrs(a)
		}
		return true
	})

	var body bytes.Buffer
	var jh slog.Handler = slog.NewJSONHandler(&body, &h.opts)
	for _, d := range h.derive {
		jh = d(jh)
	}
	if err := jh.Handle(ctx, rest); err != nil {
		return err
	}
	return h.file.writeLine(hdr, bytes.TrimRight(body.Bytes(), "\n"))
}

func (h *logHandler) with(d func(slog.Handler) slog.Handler) *logHandler {
	c := *h
	c.derive = append(append([]func(slog.Handler) slog.Handler(nil), h.derive...), d)
	return &c
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h.with(func(j slog.Handler) slog.Handler { return j.WithAttrs(attrs) })
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.with(func(j slog.Handler) slog.Handler { return j.WithGroup(name) })
}

var _ slog.Handler = (*logHandler)(nil)
