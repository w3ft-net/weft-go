package weft

import (
	"bufio"
	"io"
	"os"
	"time"
)

// CaptureStdio sends the process's stdout and stderr into the log file, as
// text lines of kind "stdout" and "stderr" (so they read as
// /log/<ns>/<svc>/stdout.log and stderr.log), with no level. It does two
// things:
//
//  1. os.Stdout and os.Stderr are replaced with pipes read by this package,
//     so fmt.Println and friends become headered lines. They carry no trace
//     ID: Go has no goroutine-local context, so code that wants one logs
//     through slog with a ctx.
//  2. File descriptors 1 and 2 are pointed at the log file, so output that
//     bypasses os.Stdout (a runtime panic, cgo) still lands there. Such lines
//     have no header, and weftd ships them with the ingest time and no level.
//
// It does nothing when stdout is a terminal or the LogFile is nil. It takes over the process's
// output, so call it deliberately, once, after InitLogging. Stop any
// systemd StandardOutput=append: redirect to the same file, or each line is
// written twice.
func (l *LogFile) CaptureStdio() error {
	if l == nil {
		return nil
	}
	if st, err := os.Stdout.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
		return nil
	}
	if _, err := l.captureFile(&os.Stdout, "stdout"); err != nil {
		return err
	}
	if _, err := l.captureFile(&os.Stderr, "stderr"); err != nil {
		return err
	}
	if l.f != nil {
		return pointStdioAt(l.f)
	}
	return nil
}

// captureFile replaces *target with a pipe whose lines go into the log file
// under kind. The returned func restores the original.
func (l *LogFile) captureFile(target **os.File, kind string) (restore func(), err error) {
	orig := *target
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	*target = w
	done := make(chan struct{})
	go func() {
		defer close(done)
		l.pumpLines(r, kind)
	}()
	return func() {
		*target = orig
		w.Close()
		<-done
		r.Close()
	}, nil
}

// pumpLines turns r's lines into headered text lines. A line longer than the
// buffer is written in pieces rather than dropped.
func (l *LogFile) pumpLines(r io.Reader, kind string) {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, _, err := br.ReadLine()
		if len(line) > 0 {
			_ = l.writeLine(lineHeader{ts: time.Now(), format: "text", kind: kind}, line)
		}
		if err != nil {
			return
		}
	}
}
