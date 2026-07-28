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
	"neulsang/desktopd/internal/db"
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

	noTokenResponse, err := http.Get("http://" + addr + "/v1/searches?view=unresolved")
	if err != nil {
		t.Fatalf("GET /v1/searches (no token): %v", err)
	}
	if err := noTokenResponse.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
	if noTokenResponse.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /v1/searches (no token) status = %d, want %d", noTokenResponse.StatusCode, http.StatusUnauthorized)
	}

	wrongTokenResponse, err := doAuthedRequest(t, "wrong-token", http.MethodGet, "http://"+addr+"/v1/searches?view=unresolved", "", nil)
	if err != nil {
		t.Fatalf("GET /v1/searches (wrong token): %v", err)
	}
	if err := wrongTokenResponse.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
	if wrongTokenResponse.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /v1/searches (wrong token) status = %d, want %d", wrongTokenResponse.StatusCode, http.StatusUnauthorized)
	}

	// review R-01's DNS-rebinding scenario: a valid token alone must not be enough
	// if the Host header doesn't match the loopback address actually being served.
	spoofedHostRequest, err := http.NewRequest(http.MethodGet, "http://"+addr+"/v1/searches?view=unresolved", nil)
	if err != nil {
		t.Fatalf("build spoofed-host request: %v", err)
	}
	spoofedHostRequest.Host = "rebound.attacker.example"
	spoofedHostRequest.Header.Set("Authorization", "Bearer "+token)
	spoofedHostResponse, err := http.DefaultClient.Do(spoofedHostRequest)
	if err != nil {
		t.Fatalf("GET /v1/searches (spoofed Host, valid token): %v", err)
	}
	if err := spoofedHostResponse.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
	if spoofedHostResponse.StatusCode != http.StatusForbidden {
		t.Errorf("GET /v1/searches (spoofed Host, valid token) status = %d, want %d", spoofedHostResponse.StatusCode, http.StatusForbidden)
	}

	validResponse, err := doAuthedRequest(t, token, http.MethodGet, "http://"+addr+"/v1/searches?view=unresolved", "", nil)
	if err != nil {
		t.Fatalf("GET /v1/searches (valid token): %v", err)
	}
	if err := validResponse.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
	if validResponse.StatusCode != http.StatusOK {
		t.Errorf("GET /v1/searches (valid token) status = %d, want %d", validResponse.StatusCode, http.StatusOK)
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
			`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at, updated_at, triage_state) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"stale-capture", "hello", "manual", "stale-hash", now, now, "unseen",
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
	token, err := app.APIToken()
	if err != nil {
		t.Fatalf("APIToken: %v", err)
	}

	body := getExplanationSnapshot(t, token, addr, "stale-capture")
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
	// The capture starts unresolved: it is waiting for the user to decide something.
	searches := decodeSearchList(t, token, addr, "view=unresolved")
	if !containsSearchItem(searches, body.CaptureID, "unseen") {
		t.Fatalf("unresolved searches = %#v, want capture_id %q as unseen", searches, body.CaptureID)
	}

	// "학습할래요" on a word capture commits it to the learning list in one step.
	learnResponse, err := doAuthedRequest(t, token, http.MethodPost, "http://"+addr+"/v1/searches/"+body.CaptureID+"/learn", "", nil)
	if err != nil {
		t.Fatalf("POST /v1/searches/{id}/learn: %v", err)
	}
	defer func() {
		if err := learnResponse.Body.Close(); err != nil {
			t.Fatalf("close learn response body: %v", err)
		}
	}()
	if learnResponse.StatusCode != http.StatusOK {
		responseBody, readErr := io.ReadAll(learnResponse.Body)
		if readErr != nil {
			t.Fatalf("read learn response body: %v", readErr)
		}
		t.Fatalf("status = %d, want %d, body=%s", learnResponse.StatusCode, http.StatusOK, string(responseBody))
	}
	var learnBody struct {
		TriageState  string `json:"triage_state"`
		CardsCreated int    `json:"cards_created"`
	}
	if err := json.NewDecoder(learnResponse.Body).Decode(&learnBody); err != nil {
		t.Fatalf("decode learn response: %v", err)
	}
	if learnBody.TriageState != "learning" {
		t.Fatalf("triage_state = %q, want learning", learnBody.TriageState)
	}

	// Having decided, it drops out of the unresolved list but stays in the full history.
	if containsSearchItem(t2Items(t, token, addr, "view=unresolved"), body.CaptureID, "learning") {
		t.Fatalf("capture %q still listed as unresolved after learning", body.CaptureID)
	}
	if !containsSearchItem(t2Items(t, token, addr, "view=all"), body.CaptureID, "learning") {
		t.Fatalf("capture %q missing from the full history", body.CaptureID)
	}
}

func t2Items(t *testing.T, token, addr, query string) []searchTestItem {
	t.Helper()
	return decodeSearchList(t, token, addr, query)
}

func decodeSearchList(t *testing.T, token, addr, query string) []searchTestItem {
	t.Helper()
	response, err := doAuthedRequest(t, token, http.MethodGet, "http://"+addr+"/v1/searches?"+query, "", nil)
	if err != nil {
		t.Fatalf("GET /v1/searches?%s: %v", query, err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Fatalf("close searches response body: %v", err)
		}
	}()
	if response.StatusCode != http.StatusOK {
		responseBody, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			t.Fatalf("read searches response body: %v", readErr)
		}
		t.Fatalf("GET /v1/searches?%s status = %d, want %d, body=%s", query, response.StatusCode, http.StatusOK, string(responseBody))
	}
	var listBody struct {
		Items []searchTestItem `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode searches response: %v", err)
	}
	return listBody.Items
}

type searchTestItem struct {
	CaptureID   string `json:"capture_id"`
	TriageState string `json:"triage_state"`
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

func containsSearchItem(items []searchTestItem, captureID, triageState string) bool {
	for _, item := range items {
		if item.CaptureID == captureID && item.TriageState == triageState {
			return true
		}
	}
	return false
}

func TestResolveAIProvider(t *testing.T) {
	tests := []struct {
		name       string
		aiProvider string
		apiKey     string
		want       string
	}{
		{"explicit mock", "mock", "some-key", "mock"},
		{"explicit gemini with key", "gemini", "some-key", "gemini"},
		{"explicit gemini without key degrades to mock", "gemini", "", "mock"},
		{"auto with key selects gemini", "", "some-key", "gemini"},
		{"auto without key selects mock", "", "", "mock"},
		{"unknown value degrades to mock", "openai", "some-key", "mock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := New(config.Config{
				AIProvider:   tt.aiProvider,
				GeminiAPIKey: tt.apiKey,
			}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if got := app.resolveAIProvider(); got != tt.want {
				t.Errorf("resolveAIProvider() = %q, want %q", got, tt.want)
			}
			// newExplainer, newSuggester, and resolvedProvider must all agree with
			// resolveAIProvider — that agreement is the point of RW-06.
			if got := app.resolvedProvider(); got != tt.want {
				t.Errorf("resolvedProvider() = %q, want %q", got, tt.want)
			}
			_, explainerIsMock := app.newExplainer().(*explain.MockExplainer)
			if explainerIsMock != (tt.want == "mock") {
				t.Errorf("newExplainer() mock = %v, want %v", explainerIsMock, tt.want == "mock")
			}
			suggester := app.newSuggester()
			if tt.want == "mock" && suggester != nil {
				t.Errorf("newSuggester() = %v, want nil (mock/no-key should disable AI suggest)", suggester)
			}
			if tt.want == "gemini" && suggester == nil {
				t.Errorf("newSuggester() = nil, want a Gemini suggester")
			}
		})
	}
}

// slowExplainer tracks how many Explain calls are in flight at once, so the test
// can assert the semaphore in explainingCaptureCreator actually bounds concurrency
// (RW-02/review R-01,R-08).
type slowExplainer struct {
	inFlight    atomic.Int64
	maxInFlight atomic.Int64
	delay       time.Duration
}

func (e *slowExplainer) Explain(ctx context.Context, text string, _ explain.Format) (explain.ExplainResult, string, error) {
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
		explainService: explain.NewService(explainer, noopExplainRepo{}, nil),
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
