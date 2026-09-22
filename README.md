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
automatically correlated. `weft.NewSlogHandler` stamps the same ID onto
regular `log/slog` output; `weft.TraceHeaders` propagates it to a
downstream service.

```go
import (
    "log/slog"
    "net/http"
    "os"

    "github.com/w3ft-net/weft-go"
)

slog.SetDefault(slog.New(weft.NewSlogHandler(slog.NewJSONHandler(os.Stdout, nil))))

mux := http.NewServeMux()
mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    c.Send(ctx, "orders", map[string]any{"event": "created"}) // auto-tagged
    slog.InfoContext(ctx, "processing order")                 // trace_id attribute

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
"Trace propagation" above). The HTTP fallback transport depends on
weft's `http-ingest` endpoint, which is "Designed, not started"
upstream — until that lands, the local-socket transport is the only
working path.

## License

MIT. See [LICENSE](LICENSE).
