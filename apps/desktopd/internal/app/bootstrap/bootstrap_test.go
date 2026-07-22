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
	"neulsang/desktopd/internal/db"
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

// TestRunRecoversStaleLookupJobsFromPreviousProcess simulates the scenario RW-03
// (review R-03) fixes: a previous desktopd process left a lookup_job "running"
// (crash, force-kill, or anything that skipped graceful shutdown) — no goroutine
// in *this* process is ever going to finish it. Run() must recover it to
// "failed" at startup so the capture doesn't stay stuck "processing" forever.
func TestRunRecoversStaleLookupJobsFromPreviousProcess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data", "neulsang.db")

	func() {
		sqlDB, err := db.Open(dbPath)
		if err != nil {
			t.Fatalf("open database: %v", err)
		}
		defer func() {
			if err := sqlDB.Close(); err != nil {
				t.Fatalf("close database: %v", err)
			}
		}()
		if err := db.Migrate(context.Background(), sqlDB, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
			t.Fatalf("migrate database: %v", err)
		}
		now := time.Now().UTC()
		if _, err := sqlDB.Exec(
			`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at, inbox_status) VALUES (?, ?, ?, ?, ?, ?)`,
			"stale-capture", "hello", "manual", "stale-hash", now, "new",
		); err != nil {
			t.Fatalf("insert stale capture fixture: %v", err)
		}
		if _, err := sqlDB.Exec(
			`INSERT INTO lookup_jobs(id, capture_id, status, created_at, started_at) VALUES (?, ?, ?, ?, ?)`,
			"stale-job", "stale-capture", "running", now, now,
		); err != nil {
			t.Fatalf("insert stale lookup job fixture: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	app := New(config.Config{Addr: "127.0.0.1:0", DBPath: dbPath}, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

	body := getExplanationSnapshot(t, addr, "stale-capture")
	if body.Status != "failed" {
		t.Fatalf("stale job status = %q, want failed (recovered at startup)", body.Status)
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

	explanationBody := waitForExplanationFinished(t, addr, body.CaptureID)
	if explanationBody.CaptureID != body.CaptureID || explanationBody.Status != "done" || explanationBody.Explanation == nil || explanationBody.Explanation.BriefKo == "" || explanationBody.Explanation.DetailedKo == "" {
		t.Fatalf("explanation response = %#v", explanationBody)
	}

	inboxResponse, err := http.Get("http://" + addr + "/v1/inbox?status=new")
	if err != nil {
		t.Fatalf("GET /v1/inbox?status=new: %v", err)
	}
	defer func() {
		if err := inboxResponse.Body.Close(); err != nil {
			t.Fatalf("close inbox response body: %v", err)
		}
	}()
	if inboxResponse.StatusCode != http.StatusOK {
		responseBody, readErr := io.ReadAll(inboxResponse.Body)
		if readErr != nil {
			t.Fatalf("read inbox response body: %v", readErr)
		}
		t.Fatalf("status = %d, want %d, body=%s", inboxResponse.StatusCode, http.StatusOK, string(responseBody))
	}
	var inboxBody struct {
		Items []inboxTestItem `json:"items"`
	}
	if err := json.NewDecoder(inboxResponse.Body).Decode(&inboxBody); err != nil {
		t.Fatalf("decode inbox response: %v", err)
	}
	if !containsInboxItem(inboxBody.Items, body.CaptureID, "new") {
		t.Fatalf("inbox response = %#v, want capture_id %q with status new", inboxBody, body.CaptureID)
	}

	archiveRequest, err := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/inbox/"+body.CaptureID+"/archive", nil)
	if err != nil {
		t.Fatalf("build archive request: %v", err)
	}
	archiveResponse, err := http.DefaultClient.Do(archiveRequest)
	if err != nil {
		t.Fatalf("POST /v1/inbox/{id}/archive: %v", err)
	}
	defer func() {
		if err := archiveResponse.Body.Close(); err != nil {
			t.Fatalf("close archive response body: %v", err)
		}
	}()
	if archiveResponse.StatusCode != http.StatusOK {
		responseBody, readErr := io.ReadAll(archiveResponse.Body)
		if readErr != nil {
			t.Fatalf("read archive response body: %v", readErr)
		}
		t.Fatalf("status = %d, want %d, body=%s", archiveResponse.StatusCode, http.StatusOK, string(responseBody))
	}

	archivedInboxResponse, err := http.Get("http://" + addr + "/v1/inbox?status=archived")
	if err != nil {
		t.Fatalf("GET /v1/inbox?status=archived: %v", err)
	}
	defer func() {
		if err := archivedInboxResponse.Body.Close(); err != nil {
			t.Fatalf("close archived inbox response body: %v", err)
		}
	}()
	if archivedInboxResponse.StatusCode != http.StatusOK {
		responseBody, readErr := io.ReadAll(archivedInboxResponse.Body)
		if readErr != nil {
			t.Fatalf("read archived inbox response body: %v", readErr)
		}
		t.Fatalf("status = %d, want %d, body=%s", archivedInboxResponse.StatusCode, http.StatusOK, string(responseBody))
	}
	var archivedInboxBody struct {
		Items []inboxTestItem `json:"items"`
	}
	if err := json.NewDecoder(archivedInboxResponse.Body).Decode(&archivedInboxBody); err != nil {
		t.Fatalf("decode archived inbox response: %v", err)
	}
	if !containsInboxItem(archivedInboxBody.Items, body.CaptureID, "archived") {
		t.Fatalf("archived inbox response = %#v, want capture_id %q with status archived", archivedInboxBody, body.CaptureID)
	}
}

type inboxTestItem struct {
	CaptureID string `json:"capture_id"`
	Status    string `json:"status"`
}

type explanationTestResponse struct {
	CaptureID   string `json:"capture_id"`
	Status      string `json:"status"`
	Explanation *struct {
		BriefKo    string `json:"brief_ko"`
		DetailedKo string `json:"detailed_ko"`
	} `json:"explanation"`
}

func waitForExplanationFinished(t *testing.T, addr, captureID string) explanationTestResponse {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		body := getExplanationSnapshot(t, addr, captureID)
		if body.Status != "queued" && body.Status != "running" {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf("explanation did not finish within 2s: %#v", body)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func getExplanationSnapshot(t *testing.T, addr, captureID string) explanationTestResponse {
	t.Helper()
	response, err := http.Get("http://" + addr + "/v1/captures/" + captureID + "/explanation")
	if err != nil {
		t.Fatalf("GET /v1/captures/{id}/explanation: %v", err)
	}
	responseBody, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil {
		t.Fatalf("read explanation response body: %v", readErr)
	}
	if closeErr != nil {
		t.Fatalf("close explanation response body: %v", closeErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", response.StatusCode, http.StatusOK, string(responseBody))
	}
	var body explanationTestResponse
	if err := json.Unmarshal(responseBody, &body); err != nil {
		t.Fatalf("decode explanation response: %v", err)
	}
	return body
}

func containsInboxItem(items []inboxTestItem, captureID, status string) bool {
	for _, item := range items {
		if item.CaptureID == captureID && item.Status == status {
			return true
		}
	}
	return false
}
