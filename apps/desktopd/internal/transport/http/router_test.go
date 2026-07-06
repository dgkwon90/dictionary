package http

import (
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/healthz", nil)

	NewRouter(slog.Default()).ServeHTTP(recorder, request)

	result := recorder.Result()
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if err := result.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
	if result.StatusCode != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", result.StatusCode, nethttp.StatusOK)
	}
	if got := result.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if got, want := string(body), `{"status":"ok"}`; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestUnknownPath(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/unknown", nil)

	NewRouter(slog.Default()).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusNotFound {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusNotFound)
	}
}

func TestHealthzMethodNotAllowed(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/healthz", nil)

	NewRouter(slog.Default()).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}
