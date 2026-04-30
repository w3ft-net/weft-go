package weft

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// heartbeatClient holds an http.Client preconfigured to dial the
// weftd heartbeat Unix socket. Environments without local weftd
// can't heartbeat — the empty-socket path turns Heartbeat into a
// no-op.
type heartbeatClient struct {
	socket string
	client *http.Client
}

func newHeartbeatClient(socket string) *heartbeatClient {
	if socket == "" {
		return nil
	}
	return &heartbeatClient{
		socket: socket,
		client: &http.Client{
			Timeout: 2 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", socket)
				},
			},
		},
	}
}

func (h *heartbeatClient) hit(service string) error {
	if !validService(service) {
		return fmt.Errorf("weft: invalid service name %q", service)
	}
	// The host part is irrelevant when DialContext is overridden
	// to dial a Unix socket; "localhost" is a placeholder.
	target := "http://localhost/h/" + url.PathEscape(service)
	req, err := http.NewRequest(http.MethodPost, target, nil)
	if err != nil {
		return err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("weft: heartbeat dial: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("weft: heartbeat: %d %s", resp.StatusCode, resp.Status)
	}
	return nil
}

func (h *heartbeatClient) close() {
	// http.Transport closes idle conns when garbage-collected; no
	// explicit shutdown needed for v1.
}

// validService mirrors the server-side rule in
// weft/weftd/heartbeat: [a-zA-Z0-9._-]+, 1-128 chars. Validated
// client-side too so callers get a clear error before a 400 trip.
func validService(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' ||
			r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' ||
			r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return !strings.ContainsRune(s, ' ')
}
