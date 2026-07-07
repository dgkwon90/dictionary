package http

import (
	"context"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neulsang/desktopd/internal/domain/capture"
	"neulsang/desktopd/internal/domain/explain"
	"neulsang/desktopd/internal/transport/http/handlers"
)

func TestHealthz(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/healthz", nil)

	NewRouter(slog.Default(), nil, nil).ServeHTTP(recorder, request)

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

	NewRouter(slog.Default(), nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusNotFound {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusNotFound)
	}
}

func TestHealthzMethodNotAllowed(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/healthz", nil)

	NewRouter(slog.Default(), nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestCapturesRoute(t *testing.T) {
	handler := handlers.NewCapture(routerFakeCaptureCreator{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/captures", strings.NewReader(`{"text":"hello","input_mode":"manual"}`))

	NewRouter(slog.Default(), handler, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusCreated {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusCreated)
	}
}

func TestCapturesGetMethodNotAllowed(t *testing.T) {
	handler := handlers.NewCapture(routerFakeCaptureCreator{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/captures", nil)

	NewRouter(slog.Default(), handler, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestExplanationRoute(t *testing.T) {
	handler := handlers.NewExplanation(routerFakeExplanationReader{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/captures/capture-id/explanation", nil)

	NewRouter(slog.Default(), nil, handler).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestExplanationPostMethodNotAllowed(t *testing.T) {
	handler := handlers.NewExplanation(routerFakeExplanationReader{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/captures/capture-id/explanation", nil)

	NewRouter(slog.Default(), nil, handler).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

type routerFakeCaptureCreator struct{}

func (routerFakeCaptureCreator) Create(context.Context, capture.CreateInput) (capture.CreateResult, error) {
	return capture.CreateResult{CaptureID: "capture-id", LookupJobID: "job-id", Status: "queued"}, nil
}

type routerFakeExplanationReader struct{}

func (routerFakeExplanationReader) GetSnapshot(context.Context, string) (explain.Snapshot, error) {
	return explain.Snapshot{Status: "queued"}, nil
}
