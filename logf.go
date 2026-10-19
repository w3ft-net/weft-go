package weft

import (
	"context"
	"fmt"
	"log/slog"
)

// Infof logs a formatted line through slog's default logger. ctx's trace and
// span go with it, so the message never has to spell the trace ID. slog has no
// printf-style call, and the stdlib log package has no ctx; this is both.
func Infof(ctx context.Context, format string, args ...any) {
	logf(ctx, slog.LevelInfo, format, args)
}

// Warnf is Infof for something wrong but survivable.
func Warnf(ctx context.Context, format string, args ...any) {
	logf(ctx, slog.LevelWarn, format, args)
}

// Errorf is Infof for a failure.
func Errorf(ctx context.Context, format string, args ...any) {
	logf(ctx, slog.LevelError, format, args)
}

func logf(ctx context.Context, level slog.Level, format string, args []any) {
	l := slog.Default()
	if !l.Enabled(ctx, level) {
		return
	}
	l.Log(ctx, level, fmt.Sprintf(format, args...))
}
