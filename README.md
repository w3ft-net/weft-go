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

// 1. Generic event emission.
c.Send("billing", map[string]any{"event": "charge", "amount": 4200})

// 2. Service heartbeats — long-running goroutine, cancelled via ctx.
go c.Heartbeat(ctx, "my-service", weft.DefaultHeartbeatInterval)

// 3. Go runtime telemetry — same shape as Heartbeat.
go c.RuntimeStats(ctx, "my-service", weft.DefaultRuntimeStatsInterval)
```

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

v0.1.0. API may change before v1.0. The HTTP fallback transport
depends on weft's `http-ingest` endpoint, which is "Designed, not
started" upstream — until that lands, the local-socket transport
is the only working path.

## License

MIT. See [LICENSE](LICENSE).
