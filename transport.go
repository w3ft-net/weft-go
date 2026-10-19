package weft

import (
	"context"
	"errors"
	"io"
)

// transport is the internal contract a record sink fulfils. The
// SDK selects a concrete transport at construction (Unix socket
// preferred; HTTP fallback) and routes Send / SendLine through
// it. Heartbeats ride this same records transport (see
// Client.Heartbeat) — there is no separate heartbeat channel.
type transport interface {
	// sendLine writes one app-prefixed log line. The line should
	// not contain a newline; the transport adds framing. ctx's
	// ambient trace ID (see TraceIDFromContext), if any, is
	// attached by the concrete transport in whatever way its wire
	// format supports.
	sendLine(ctx context.Context, app, line string) error
	// close releases resources. Called from Client.Close.
	close() error
}

// errTransportUnavailable signals that the transport's underlying
// resource (socket file, HTTP endpoint) isn't usable. The Client
// returns this so callers can distinguish "couldn't deliver" from
// programmer errors.
var errTransportUnavailable = errors.New("weft: transport unavailable")

// noopTransport drops everything. Used in tests and as a defensive
// fallback when neither a socket nor HTTP credentials are
// available — the Client constructs without error but Send returns
// errTransportUnavailable so the caller knows.
type noopTransport struct{}

func (noopTransport) sendLine(_ context.Context, _, _ string) error { return errTransportUnavailable }
func (noopTransport) close() error                                 { return nil }

// Compile-time check.
var _ transport = (*noopTransport)(nil)
var _ io.Closer = (*Client)(nil)
