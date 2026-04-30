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

## Transport

Records (`Send` / `SendLine`) frame to the local weftd Unix socket
as `<app>:<message>\n` lines. Heartbeats use a separate Unix
socket (`/var/run/weftd-heartbeat.sock` by default) per the wire
protocol in the upstream weft repo's
`docs/plans/app-heartbeats.md`. Without a reachable weftd, the SDK
has no path to ship records — `Send` returns
`errTransportUnavailable` so the caller knows nothing's being
delivered.

## Configuration

| Env | Default | Meaning |
|---|---|---|
| `WEFT_SOCKET` | `/var/run/weftd.sock` | records socket path; empty disables |
| `WEFT_HEARTBEAT_SOCKET` | `/var/run/weftd-heartbeat.sock` | heartbeat socket path; empty disables |

Programmatic equivalents: `weft.WithSocket`,
`weft.WithHeartbeatSocket`.

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

v0.1.0. API may change before v1.0.

## License

MIT. See [LICENSE](LICENSE).
