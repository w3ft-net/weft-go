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
// All three share one record path. Two transports back it: a local
// weftd Unix socket (default; set via WEFT_SOCKET) and an HTTP
// fallback to /api/ingest (set via WEFT_ENDPOINT and WEFT_TOKEN).
// The transport is selected once at construction; callers that need
// to override pass weft.WithSocket / weft.WithHTTP explicitly.
//
// Heartbeats are ordinary records emitted to app
// "<service>/heartbeats"; they travel the same transport as Send,
// not a separate socket. Liveness is judged by recency — a service
// that stops emitting heartbeat rows is absent.
package weft
