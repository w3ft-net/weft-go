# weft-go

A lightweight Go SDK for [weft](https://w3ft.net).

```go
import "github.com/w3ft-net/weft-go"
```

## Features by example

```go
import (
    "context"

    "github.com/w3ft-net/weft-go"
)

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

c, _ := weft.New()
defer c.Close()

// 1. Generic event emission. ctx's ambient trace ID (see "Trace
// propagation" below), if any, is attached automatically.
c.Send(ctx, "billing", map[string]any{"event": "charge", "amount": 4200})

// 2. Service heartbeats — long-running goroutine, cancelled via ctx.
go c.Heartbeat(ctx, "my-service", weft.DefaultHeartbeatInterval)

// 3. Go runtime telemetry — same shape as Heartbeat.
go c.RuntimeStats(ctx, "my-service", weft.DefaultRuntimeStatsInterval)
```

## Trace propagation

`weft.TraceMiddleware` establishes an ambient trace ID for each request —
adopted from an inbound W3C `traceparent` header, or freshly generated —
so every `Send`/`SendLine` call made while handling that request is
automatically correlated. `weft.InitLogging` (below) records the same ID
on every `log/slog` line written with that ctx; `weft.TraceHeaders`
propagates it to a downstream service.

```go
import (
    "log/slog"
    "net/http"
    "os"

    "github.com/w3ft-net/weft-go"
)

weft.InitLogging("", os.Stderr) // see "Logging to a file"

mux := http.NewServeMux()
mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    c.Send(ctx, "orders", map[string]any{"event": "created"}) // auto-tagged
    slog.InfoContext(ctx, "processing order")                 // trace ID in the line header

    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://inventory/check", nil)
    for k, v := range weft.TraceHeaders(ctx) {
        req.Header.Set(k, v) // propagate to a downstream service
    }
    http.DefaultClient.Do(req)
})
http.ListenAndServe(":8080", weft.TraceMiddleware(mux))
```

Query every log line for one request across every service with weft's
`trace <id>` shell verb.

## Logging to a file

`weft.InitLogging(path, fallback)` makes `log/slog` (and the stdlib
`log` package) write to `path`, or to `$WEFT_LOG_FILE` when `path` is
empty. With neither set it logs JSON lines to `fallback` (usually
`os.Stderr`), each with the ctx's trace ID as a `trace_id` attribute.
Name the file as a source in weftd's config.

`weft.Infof`, `Warnf` and `Errorf` log a formatted message through the
same logger with the ctx's trace ID: `slog` has no printf-style call and
the stdlib `log` has no ctx.

Each line carries a header with its timestamp, level, trace ID and span
fields, which weftd moves onto the row's own columns. The body is the
record as JSON. A service whose logs come this way needs no schema
extraction, and nothing in the message has to spell the trace ID.

The file belongs to the SDK; nothing else should write to it. To bring
stdout and stderr in too, call `CaptureStdio` on what `InitLogging`
returns (both are no-ops when no file is in use):

```go
lf := weft.InitLogging("", os.Stderr)
defer lf.Close()
lf.CaptureStdio() // stdout.log and stderr.log under the service
```

`fmt.Println` carries no context, so captured lines have no trace ID;
log through `slog` with a ctx where you want one. Under systemd, stop
redirecting the unit's stdout and stderr to a file (leave them on the
journal) or each line is written twice. Capture does nothing when
stdout is a terminal.

## Trace propagation over gRPC

`context.Value` does not cross a process boundary, so `weft.GRPC`
sends the ctx's trace and span IDs as call metadata and restores
them in the handler. Install the interceptors once on each side:

```go
conn, _ := grpc.NewClient(addr,
    grpc.WithChainUnaryInterceptor(weft.GRPC.UnaryClientInterceptor()),
    grpc.WithChainStreamInterceptor(weft.GRPC.StreamClientInterceptor()))

srv := grpc.NewServer(
    grpc.ChainUnaryInterceptor(weft.GRPC.UnaryServerInterceptor()),
    grpc.ChainStreamInterceptor(weft.GRPC.StreamServerInterceptor()))
```

A `StartSpan` in the handler then parents off the caller's span, and log
lines written with the handler's ctx carry the caller's trace ID. Without
a trace ID on the ctx nothing is sent.

## Spans

`StartSpan` times a unit of work within a trace. It logs `span start`
immediately and `span done` (with duration and status) on `End`, through
the same logger, so the span's name, IDs and duration land in the line
header. A nested `StartSpan` parents off the ctx's ambient span; with no
trace ID on the ctx it does nothing.

```go
ctx, span := weft.StartSpan(ctx, "db.query")
defer span.End()
if err := run(ctx); err != nil {
    span.RecordError(err)
}
```

`TraceMiddleware` and `TraceHeaders` carry the span ID in `traceparent`
too, so a remote caller's span becomes the parent of the handler's.

## Heartbeat and runtime fields

`c.HeartbeatWithFields` and `c.RuntimeStatsWithFields` merge extra fields
(an instance ID, say) into every record. The reserved keys (`type`,
`service`, `ts`) always win.

## Two transports

The records path (`Send` / `SendLine`) prefers the local weftd
Unix socket and falls back to HTTP `/api/ingest` when configured.
Selection happens once at construction:

| Configuration | Transport |
|---|---|
| `WEFT_SOCKET` set or default `/var/run/weftd.sock` resolves | local Unix socket |
| `weft.WithHTTP(endpoint, token)` | HTTP, regardless of socket |
| `WEFT_TOKEN` set, no socket | HTTP fallback |
| neither socket nor token | no-op (`Send` returns an error) |

Heartbeats ride this same records transport — each beat is an
ordinary record emitted to app `<service>/heartbeats` (the
pseudofile `/log/<namespace>/<service>/heartbeats.log`), so there
is no separate heartbeat socket. Liveness is judged by recency: a
service that stops emitting heartbeat rows is absent.

## Configuration

| Env | Default | Meaning |
|---|---|---|
| `WEFT_SOCKET` | `/var/run/weftd.sock` | records socket path; empty disables |
| `WEFT_ENDPOINT` | `https://ingest.w3ft.net` | HTTP fallback endpoint |
| `WEFT_TOKEN` | (none) | bearer for HTTP transport |

Programmatic equivalents: `weft.WithSocket`, `weft.WithHTTP`,
`weft.WithToken`. (`weft.WithHeartbeatSocket` and
`WEFT_HEARTBEAT_SOCKET` are deprecated no-ops — heartbeats now
share the records transport.)

## Runtime stats

`c.RuntimeStats(ctx, service, interval)` periodically emits a
record at app `<service>/runtime` with:

- `goroutines` — `runtime.NumGoroutine()`
- `fds_open` — `/proc/self/fd` count (Linux only; 0 elsewhere)
- `heap_alloc_mb`, `heap_sys_mb`, `heap_inuse_mb`, `stack_inuse_mb`
- `gc_pauses_p50_us` / `_p95_us` / `_p99_us` — pause histogram percentiles
- `gc_count`, `next_gc_mb`
- `uptime_s`

Default cadence is 30s (`weft.DefaultRuntimeStatsInterval`,
matches weftd's OS-metrics rhythm).

## Status

v0.3.0. API may change before v1.0. **Breaking change in v0.3.0:**
`Client.Send`/`Client.SendLine` now take `ctx context.Context` as
their first parameter (needed to carry the ambient trace ID — see
"Trace propagation" above).

## License

MIT. See [LICENSE](LICENSE).
