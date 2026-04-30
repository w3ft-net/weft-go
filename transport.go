package weft

import (
	"errors"
	"io"
)

// transport is the internal contract a record sink fulfils. The
// SDK uses a Unix-socket sink to the local weftd, or a no-op when
// no socket path is configured. Heartbeats have a separate,
// simpler transport — see heartbeat.go.
type transport interface {
	// sendLine writes one app-prefixed log line. The line should
	// not contain a newline; the transport adds framing.
	sendLine(app, line string) error
	// close releases resources. Called from Client.Close.
	close() error
}

// errTransportUnavailable signals that no records destination is
// configured (empty records-socket path). The Client returns this
// so callers can distinguish "couldn't deliver" from programmer
// errors.
var errTransportUnavailable = errors.New("weft: transport unavailable")

// noopTransport drops everything. Used in tests and as a defensive
// fallback when no socket path is configured — the Client
// constructs without error but Send returns
// errTransportUnavailable so the caller knows.
type noopTransport struct{}

func (noopTransport) sendLine(_, _ string) error { return errTransportUnavailable }
func (noopTransport) close() error               { return nil }

// Compile-time check.
var _ transport = (*noopTransport)(nil)
var _ io.Closer = (*Client)(nil)
