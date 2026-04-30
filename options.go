package weft

import "os"

// Defaults for env-driven configuration.
const (
	DefaultRecordsSocket   = "/var/run/weftd.sock"
	DefaultHeartbeatSocket = "/var/run/weftd-heartbeat.sock"
)

// Option configures a Client at construction time. Programmatic
// equivalents to the WEFT_* env vars; useful in tests and embedded
// scenarios.
type Option func(*config)

type config struct {
	recordsSocket   string
	heartbeatSocket string
}

func defaultConfig() *config {
	c := &config{
		recordsSocket:   firstNonEmpty(os.Getenv("WEFT_SOCKET"), DefaultRecordsSocket),
		heartbeatSocket: firstNonEmpty(os.Getenv("WEFT_HEARTBEAT_SOCKET"), DefaultHeartbeatSocket),
	}
	return c
}

// WithSocket overrides the records-socket path. Empty string
// disables the records transport entirely; Send then returns
// errTransportUnavailable so the caller knows nothing is being
// delivered.
func WithSocket(path string) Option {
	return func(c *config) { c.recordsSocket = path }
}

// WithHeartbeatSocket overrides the heartbeat-socket path. Empty
// string disables heartbeats entirely.
func WithHeartbeatSocket(path string) Option {
	return func(c *config) { c.heartbeatSocket = path }
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
