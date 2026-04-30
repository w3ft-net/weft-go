// Package weft is the Go SDK for weft (https://w3ft.net), a
// multi-tenant log analysis service.
//
// The SDK has three v1 features, all hung off *Client:
//
//  1. Generic event emission (Send / SendLine) for application
//     events.
//  2. Service-liveness heartbeats (Heartbeat) — long-running
//     goroutine launched once in main().
//  3. Go runtime telemetry (RuntimeStats) — periodic heap, GC,
//     goroutine, and FD-count records emitted alongside the
//     service's regular events.
//
// The record path writes to a local weftd Unix socket (default
// /var/run/weftd.sock; override via WEFT_SOCKET or
// weft.WithSocket). Heartbeats use a separate Unix socket
// (default /var/run/weftd-heartbeat.sock) per the wire protocol
// defined in the weft project's docs/plans/app-heartbeats.md.
package weft
