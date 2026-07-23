package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neulsang/desktopd/internal/domain/capture"
)

type fakeCaptureCreator struct {
	create func(context.Context, capture.CreateInput) (capture.CreateResult, error)
}

func (f fakeCaptureCreator) Create(ctx context.Context, input capture.CreateInput) (capture.CreateResult, error) {
	return f.create(ctx, input)
}

func TestCaptureCreateCreated(t *testing.T) {
	handler := NewCapture(fakeCaptureCreator{create: func(_ context.Context, input capture.CreateInput) (capture.CreateResult, error) {
		if input.Text != "hello" || input.InputMode != "manual" || input.SourceApp != "app" || input.SourceType != "manual" {
			t.Fatalf("input = %#v", input)
		}
		return capture.CreateResult{CaptureID: "capture-id", LookupJobID: "job-id", Status: "queued"}, nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/captures", bytes.NewBufferString(`{"text":"hello","input_mode":"manual","source_app":"app","source_type":"manual"}`))

	handler.Create(recorder, request)

	result := recorder.Result()
	if result.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", result.StatusCode, http.StatusCreated)
	}
	defer func() {
		if err := result.Body.Close(); err != nil {
			t.Fatalf("close response body: %v", err)
		}
	}()
	var body map[string]string
	if err := json.NewDecoder(result.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["capture_id"] != "capture-id" || body["lookup_job_id"] != "job-id" || body["status"] != "queued" {
		t.Fatalf("body = %#v", body)
	}
}

func TestCaptureCreateBadRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "bad json", body: `{"text":`},
		{name: "unknown field", body: `{"text":"hello","input_mode":"manual","unexpected":true}`},
		{name: "trailing data after JSON value", body: `{"text":"hello","input_mode":"manual"}{"extra":"payload"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewCapture(fakeCaptureCreator{create: func(context.Context, capture.CreateInput) (capture.CreateResult, error) {
				t.Fatal("Create should not be called")
				return capture.CreateResult{}, nil
			}}, slog.Default())
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/captures", bytes.NewBufferString(tt.body))

			handler.Create(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestCaptureCreateInvalidInput(t *testing.T) {
	handler := NewCapture(fakeCaptureCreator{create: func(context.Context, capture.CreateInput) (capture.CreateResult, error) {
		return capture.CreateResult{}, capture.ErrInvalidInput
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/captures", bytes.NewBufferString(`{"text":"","input_mode":"manual"}`))

	handler.Create(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestCaptureCreateInternalError(t *testing.T) {
	handler := NewCapture(fakeCaptureCreator{create: func(context.Context, capture.CreateInput) (capture.CreateResult, error) {
		return capture.CreateResult{}, errors.New("secret database detail")
	}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/captures", bytes.NewBufferString(`{"text":"hello","input_mode":"manual"}`))

	handler.Create(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("body exposes internal detail: %s", recorder.Body.String())
	}
}

func TestCaptureCreateRequestEntityTooLarge(t *testing.T) {
	handler := NewCapture(fakeCaptureCreator{create: func(context.Context, capture.CreateInput) (capture.CreateResult, error) {
		t.Fatal("Create should not be called")
		return capture.CreateResult{}, nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	oversizedText := strings.Repeat("a", 2<<20) // 2MiB, over the handler's 1MiB cap
	body, err := json.Marshal(map[string]string{"text": oversizedText, "input_mode": "manual"})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/captures", bytes.NewReader(body))

	handler.Create(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}
