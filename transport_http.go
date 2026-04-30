package weft

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// httpTransport is the fallback when no local weftd is reachable.
// Each sendLine becomes one POST to the configured endpoint with
// the app + line wrapped as a single-event NDJSON body. Bearer
// auth via the configured token. No batching in v1 — a future
// optimization can buffer in-memory if hot-path latency matters.
//
// Depends on the http-ingest endpoint defined in weft's
// docs/plans/http-ingest.md, which is "Designed, not started" at
// the time this SDK ships. Until that lands, this transport
// returns errors; the local-socket path remains the working
// transport.
type httpTransport struct {
	endpoint string // base URL, e.g., https://ingest.w3ft.net
	token    string
	client   *http.Client
}

func newHTTPTransport(endpoint, token string) *httpTransport {
	return &httpTransport{
		endpoint: strings.TrimRight(endpoint, "/"),
		token:    token,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *httpTransport) sendLine(app, line string) error {
	if t.token == "" {
		return fmt.Errorf("weft: http transport: no token configured (set WEFT_TOKEN)")
	}
	// One-event NDJSON body with the message field set to the line.
	// The app is part of the path-binding via the token's namespace
	// scope; including it in the body lets the coordinator route
	// when a token covers multiple apps in the namespace.
	body := []byte(fmt.Sprintf(`{"app":%q,"message":%q}`, app, line))

	req, err := http.NewRequest(http.MethodPost, t.endpoint+"/api/ingest", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("Authorization", "Bearer "+t.token)

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("weft: http POST: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("weft: http POST: %d %s", resp.StatusCode, resp.Status)
	}
	return nil
}

func (t *httpTransport) close() error { return nil }

var _ transport = (*httpTransport)(nil)
