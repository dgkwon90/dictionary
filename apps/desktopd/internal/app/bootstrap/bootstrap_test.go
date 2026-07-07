package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"neulsang/desktopd/internal/config"
)

func TestRunServesHealthAndShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbPath := filepath.Join(t.TempDir(), "data", "neulsang.db")
	app := New(config.Config{Addr: "127.0.0.1:0", DBPath: dbPath}, slog.Default())
	runErr := make(chan error, 1)
	go func() {
		runErr <- app.Run(ctx)
	}()

	addr, err := app.Addr()
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}
	response, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		cancel()
		t.Fatalf("GET /healthz: %v", err)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		cancel()
		t.Fatalf("read response body: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		cancel()
		t.Fatalf("close response body: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		cancel()
		t.Errorf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file was not created: %v", err)
	}
}

func TestRunServesCaptureCreate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbPath := filepath.Join(t.TempDir(), "data", "neulsang.db")
	app := New(config.Config{Addr: "127.0.0.1:0", DBPath: dbPath}, slog.Default())
	runErr := make(chan error, 1)
	go func() {
		runErr <- app.Run(ctx)
	}()
	defer func() {
		cancel()
		select {
		case err := <-runErr:
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Run() did not return after context cancellation")
		}
	}()

	addr, err := app.Addr()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	response, err := http.Post(
		"http://"+addr+"/v1/captures",
		"application/json",
		bytes.NewBufferString(`{"text":"hello","input_mode":"manual","source_app":"desktopd","source_type":"manual"}`),
	)
	if err != nil {
		t.Fatalf("POST /v1/captures: %v", err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Fatalf("close response body: %v", err)
		}
	}()
	if response.StatusCode != http.StatusCreated {
		body, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			t.Fatalf("read response body: %v", readErr)
		}
		t.Fatalf("status = %d, want %d, body=%s", response.StatusCode, http.StatusCreated, string(body))
	}
	var body struct {
		CaptureID   string `json:"capture_id"`
		LookupJobID string `json:"lookup_job_id"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.CaptureID == "" || body.LookupJobID == "" || body.Status != "queued" {
		t.Fatalf("response = %#v", body)
	}

	getResponse, err := http.Get("http://" + addr + "/v1/captures/" + body.CaptureID + "/explanation")
	if err != nil {
		t.Fatalf("GET /v1/captures/{id}/explanation: %v", err)
	}
	defer func() {
		if err := getResponse.Body.Close(); err != nil {
			t.Fatalf("close explanation response body: %v", err)
		}
	}()
	if getResponse.StatusCode != http.StatusOK {
		responseBody, readErr := io.ReadAll(getResponse.Body)
		if readErr != nil {
			t.Fatalf("read explanation response body: %v", readErr)
		}
		t.Fatalf("status = %d, want %d, body=%s", getResponse.StatusCode, http.StatusOK, string(responseBody))
	}
	var explanationBody struct {
		CaptureID   string `json:"capture_id"`
		Status      string `json:"status"`
		Explanation struct {
			BriefKo    string `json:"brief_ko"`
			DetailedKo string `json:"detailed_ko"`
		} `json:"explanation"`
	}
	if err := json.NewDecoder(getResponse.Body).Decode(&explanationBody); err != nil {
		t.Fatalf("decode explanation response: %v", err)
	}
	if explanationBody.CaptureID != body.CaptureID || explanationBody.Status != "done" || explanationBody.Explanation.BriefKo == "" || explanationBody.Explanation.DetailedKo == "" {
		t.Fatalf("explanation response = %#v", explanationBody)
	}
}
