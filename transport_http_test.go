package weft

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPTransport_PostsBearerJSON(t *testing.T) {
	var (
		gotAuth string
		gotPath string
		gotCT   string
		gotBody []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	tr := newHTTPTransport(srv.URL, "wft_secret123")
	if err := tr.sendLine("billing", `{"event":"x"}`); err != nil {
		t.Fatal(err)
	}

	if gotPath != "/api/ingest" {
		t.Errorf("path = %q, want /api/ingest", gotPath)
	}
	if gotAuth != "Bearer wft_secret123" {
		t.Errorf("Authorization = %q, want bearer", gotAuth)
	}
	if !strings.Contains(gotCT, "ndjson") {
		t.Errorf("Content-Type = %q, want ndjson flavor", gotCT)
	}

	var body map[string]any
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatalf("body not JSON: %v (raw %q)", err, gotBody)
	}
	if body["app"] != "billing" {
		t.Errorf("body.app = %v, want billing", body["app"])
	}
	if body["message"] != `{"event":"x"}` {
		t.Errorf("body.message = %v, want raw line", body["message"])
	}
}

func TestHTTPTransport_RejectsMissingToken(t *testing.T) {
	tr := newHTTPTransport("http://localhost:1", "")
	if err := tr.sendLine("a", "b"); err == nil {
		t.Error("expected error when no token configured")
	}
}

func TestHTTPTransport_NonOKReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	tr := newHTTPTransport(srv.URL, "tok")
	if err := tr.sendLine("a", "b"); err == nil {
		t.Error("expected error on 500")
	}
}
