package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"neulsang/desktopd/internal/config"
	"neulsang/desktopd/internal/domain/capture"
	"neulsang/desktopd/internal/domain/explain"
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

// TestRunRejectsUnauthenticatedRequests is the end-to-end counterpart to
// internal/transport/http.Secure's unit tests: it proves the real, fully wired
// App enforces the token on a live listener, while leaving /healthz open
// (review R-01).
func TestRunRejectsUnauthenticatedRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbPath := filepath.Join(t.TempDir(), "data", "neulsang.db")
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
	token, err := app.APIToken()
	if err != nil {
		t.Fatalf("APIToken: %v", err)
	}
	if token == "" {
		t.Fatal("APIToken() = empty, want a generated token")
	}

	healthzResponse, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	if err := healthzResponse.Body.Close(); err != nil {
		t.Fatalf("close healthz response body: %v", err)
	}
	if healthzResponse.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz (no token) status = %d, want %d", healthzResponse.StatusCode, http.StatusOK)
	}

	noTokenResponse, err := http.Get("http://" + addr + "/v1/inbox?status=new")
	if err != nil {
		t.Fatalf("GET /v1/inbox (no token): %v", err)
	}
	if err := noTokenResponse.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
	if noTokenResponse.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /v1/inbox (no token) status = %d, want %d", noTokenResponse.StatusCode, http.StatusUnauthorized)
	}

	wrongTokenResponse, err := doAuthedRequest(t, "wrong-token", http.MethodGet, "http://"+addr+"/v1/inbox?status=new", "", nil)
	if err != nil {
		t.Fatalf("GET /v1/inbox (wrong token): %v", err)
	}
	if err := wrongTokenResponse.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
	if wrongTokenResponse.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /v1/inbox (wrong token) status = %d, want %d", wrongTokenResponse.StatusCode, http.StatusUnauthorized)
	}

	// review R-01's DNS-rebinding scenario: a valid token alone must not be enough
	// if the Host header doesn't match the loopback address actually being served.
	spoofedHostRequest, err := http.NewRequest(http.MethodGet, "http://"+addr+"/v1/inbox?status=new", nil)
	if err != nil {
		t.Fatalf("build spoofed-host request: %v", err)
	}
	spoofedHostRequest.Host = "rebound.attacker.example"
	spoofedHostRequest.Header.Set("Authorization", "Bearer "+token)
	spoofedHostResponse, err := http.DefaultClient.Do(spoofedHostRequest)
	if err != nil {
		t.Fatalf("GET /v1/inbox (spoofed Host, valid token): %v", err)
	}
	if err := spoofedHostResponse.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
	if spoofedHostResponse.StatusCode != http.StatusForbidden {
		t.Errorf("GET /v1/inbox (spoofed Host, valid token) status = %d, want %d", spoofedHostResponse.StatusCode, http.StatusForbidden)
	}

	validResponse, err := doAuthedRequest(t, token, http.MethodGet, "http://"+addr+"/v1/inbox?status=new", "", nil)
	if err != nil {
		t.Fatalf("GET /v1/inbox (valid token): %v", err)
	}
	if err := validResponse.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
	if validResponse.StatusCode != http.StatusOK {
		t.Errorf("GET /v1/inbox (valid token) status = %d, want %d", validResponse.StatusCode, http.StatusOK)
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
	token, err := app.APIToken()
	if err != nil {
		t.Fatalf("APIToken: %v", err)
	}
	response, err := doAuthedRequest(t, token, http.MethodPost, "http://"+addr+"/v1/captures", "application/json",
		bytes.NewBufferString(`{"text":"hello","input_mode":"manual","source_app":"desktopd","source_type":"manual"}`))
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

	explanationBody := waitForExplanationFinished(t, token, addr, body.CaptureID)
	if explanationBody.CaptureID != body.CaptureID || explanationBody.Status != "done" || explanationBody.Explanation == nil || explanationBody.Explanation.BriefKo == "" || explanationBody.Explanation.DetailedKo == "" {
		t.Fatalf("explanation response = %#v", explanationBody)
	}

	inboxResponse, err := doAuthedRequest(t, token, http.MethodGet, "http://"+addr+"/v1/inbox?status=new", "", nil)
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

	archiveResponse, err := doAuthedRequest(t, token, http.MethodPost, "http://"+addr+"/v1/inbox/"+body.CaptureID+"/archive", "", nil)
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

	archivedInboxResponse, err := doAuthedRequest(t, token, http.MethodGet, "http://"+addr+"/v1/inbox?status=archived", "", nil)
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

func waitForExplanationFinished(t *testing.T, token, addr, captureID string) explanationTestResponse {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		body := getExplanationSnapshot(t, token, addr, captureID)
		if body.Status != "queued" && body.Status != "running" {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf("explanation did not finish within 2s: %#v", body)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// doAuthedRequest builds and sends a request carrying the API token every
// /v1/* route now requires (review R-01); contentType is skipped (no header
// set) when empty, matching plain GET/no-body POST routes in this test.
func doAuthedRequest(t *testing.T, token, method, url, contentType string, body io.Reader) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return http.DefaultClient.Do(req)
}

func getExplanationSnapshot(t *testing.T, token, addr, captureID string) explanationTestResponse {
	t.Helper()
	response, err := doAuthedRequest(t, token, http.MethodGet, "http://"+addr+"/v1/captures/"+captureID+"/explanation", "", nil)
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

// slowExplainer tracks how many Explain calls are in flight at once, so the test
// can assert the semaphore in explainingCaptureCreator actually bounds concurrency
// (RW-02/review R-01,R-08).
type slowExplainer struct {
	inFlight    atomic.Int64
	maxInFlight atomic.Int64
	delay       time.Duration
}

func (e *slowExplainer) Explain(ctx context.Context, text string) (explain.ExplainResult, string, error) {
	n := e.inFlight.Add(1)
	defer e.inFlight.Add(-1)
	for {
		max := e.maxInFlight.Load()
		if n <= max || e.maxInFlight.CompareAndSwap(max, n) {
			break
		}
	}
	select {
	case <-time.After(e.delay):
	case <-ctx.Done():
	}
	return explain.ExplainResult{}, "", errors.New("intentional test failure")
}

type noopExplainRepo struct{}

func (noopExplainRepo) MarkRunning(context.Context, string, time.Time) error { return nil }
func (noopExplainRepo) SaveSuccess(context.Context, string, string, explain.ExplainResult, string, time.Time) error {
	return nil
}
func (noopExplainRepo) SaveFailure(context.Context, string, string, time.Time) error { return nil }

type noopCaptureRepo struct{}

func (noopCaptureRepo) SaveNew(context.Context, capture.Capture, capture.LookupJob, capture.OutboxEvent) error {
	return nil
}

func TestExplainingCaptureCreatorBoundsConcurrency(t *testing.T) {
	const (
		semSize      = 2
		captureCount = 6
	)
	explainer := &slowExplainer{delay: 50 * time.Millisecond}
	creator := explainingCaptureCreator{
		captureService: capture.NewService(noopCaptureRepo{}),
		explainService: explain.NewService(explainer, noopExplainRepo{}),
		log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		baseCtx:        context.Background(),
		wg:             &sync.WaitGroup{},
		sem:            make(chan struct{}, semSize),
	}

	for i := 0; i < captureCount; i++ {
		if _, err := creator.Create(context.Background(), capture.CreateInput{
			Text:      "hello",
			InputMode: "manual",
		}); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}
	creator.wg.Wait()

	if max := explainer.maxInFlight.Load(); max > semSize {
		t.Errorf("max concurrent Explain() calls = %d, want <= %d", max, semSize)
	} else if max < semSize {
		// Not a hard failure, but a sign the test isn't actually exercising
		// contention — semSize concurrent explains should have overlapped given
		// captureCount is 3x semSize and each call sleeps for delay.
		t.Logf("max concurrent Explain() calls = %d, want == %d for a meaningful test", max, semSize)
	}
}
