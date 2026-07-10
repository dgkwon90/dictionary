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
	"neulsang/desktopd/internal/domain/inbox"
	"neulsang/desktopd/internal/domain/knowledge"
	"neulsang/desktopd/internal/domain/review"
	"neulsang/desktopd/internal/domain/stats"
	"neulsang/desktopd/internal/domain/suggest"
	"neulsang/desktopd/internal/transport/http/handlers"
)

func TestHealthz(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/healthz", nil)

	NewRouter(slog.Default(), nil, nil, nil, nil, nil, nil, nil).ServeHTTP(recorder, request)

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

	NewRouter(slog.Default(), nil, nil, nil, nil, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusNotFound {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusNotFound)
	}
}

func TestHealthzMethodNotAllowed(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/healthz", nil)

	NewRouter(slog.Default(), nil, nil, nil, nil, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestCapturesRoute(t *testing.T) {
	handler := handlers.NewCapture(routerFakeCaptureCreator{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/captures", strings.NewReader(`{"text":"hello","input_mode":"manual"}`))

	NewRouter(slog.Default(), handler, nil, nil, nil, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusCreated {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusCreated)
	}
}

func TestCapturesGetMethodNotAllowed(t *testing.T) {
	handler := handlers.NewCapture(routerFakeCaptureCreator{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/captures", nil)

	NewRouter(slog.Default(), handler, nil, nil, nil, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestExplanationRoute(t *testing.T) {
	handler := handlers.NewExplanation(routerFakeExplanationReader{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/captures/capture-id/explanation", nil)

	NewRouter(slog.Default(), nil, handler, nil, nil, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestExplanationPostMethodNotAllowed(t *testing.T) {
	handler := handlers.NewExplanation(routerFakeExplanationReader{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/captures/capture-id/explanation", nil)

	NewRouter(slog.Default(), nil, handler, nil, nil, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestInboxListRoute(t *testing.T) {
	handler := handlers.NewInbox(routerFakeInboxService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/inbox?status=new", nil)

	NewRouter(slog.Default(), nil, nil, handler, nil, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestInboxSaveRoute(t *testing.T) {
	handler := handlers.NewInbox(routerFakeInboxService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/inbox/capture-id/save", nil)

	NewRouter(slog.Default(), nil, nil, handler, nil, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestInboxArchiveRoute(t *testing.T) {
	handler := handlers.NewInbox(routerFakeInboxService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/inbox/capture-id/archive", nil)

	NewRouter(slog.Default(), nil, nil, handler, nil, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestInboxSaveGetMethodNotAllowed(t *testing.T) {
	handler := handlers.NewInbox(routerFakeInboxService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/inbox/capture-id/save", nil)

	NewRouter(slog.Default(), nil, nil, handler, nil, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestInboxArchiveGetMethodNotAllowed(t *testing.T) {
	handler := handlers.NewInbox(routerFakeInboxService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/inbox/capture-id/archive", nil)

	NewRouter(slog.Default(), nil, nil, handler, nil, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestKnowledgeMarkUnknownRoute(t *testing.T) {
	handler := handlers.NewKnowledge(routerFakeKnowledgeService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/knowledge/item-id/mark-unknown", nil)

	NewRouter(slog.Default(), nil, nil, nil, handler, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestKnowledgeMarkKnownRoute(t *testing.T) {
	handler := handlers.NewKnowledge(routerFakeKnowledgeService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/knowledge/item-id/mark-known", nil)

	NewRouter(slog.Default(), nil, nil, nil, handler, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestKnowledgeMarkUnknownGetMethodNotAllowed(t *testing.T) {
	handler := handlers.NewKnowledge(routerFakeKnowledgeService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/knowledge/item-id/mark-unknown", nil)

	NewRouter(slog.Default(), nil, nil, nil, handler, nil, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestReviewDueRoute(t *testing.T) {
	handler := handlers.NewReview(routerFakeReviewService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/reviews/due", nil)

	NewRouter(slog.Default(), nil, nil, nil, nil, handler, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestReviewDuePostMethodNotAllowed(t *testing.T) {
	handler := handlers.NewReview(routerFakeReviewService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/reviews/due", nil)

	NewRouter(slog.Default(), nil, nil, nil, nil, handler, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestReviewSessionStartRoute(t *testing.T) {
	handler := handlers.NewReview(routerFakeReviewService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/reviews/session/start", nil)

	NewRouter(slog.Default(), nil, nil, nil, nil, handler, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestReviewGradeRoute(t *testing.T) {
	handler := handlers.NewReview(routerFakeReviewService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/reviews/card-1/grade", strings.NewReader(`{"rating":"good","elapsed_ms":100}`))

	NewRouter(slog.Default(), nil, nil, nil, nil, handler, nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestDashboardSummaryRoute(t *testing.T) {
	handler := handlers.NewDashboard(routerFakeDashboardService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/dashboard/summary", nil)

	NewRouter(slog.Default(), nil, nil, nil, nil, nil, handler, nil).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestSuggestRoute(t *testing.T) {
	handler := handlers.NewSuggest(routerFakeSuggestService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/suggest?q=스테일", nil)

	NewRouter(slog.Default(), nil, nil, nil, nil, nil, nil, handler).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
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

type routerFakeInboxService struct{}

func (routerFakeInboxService) List(context.Context, inbox.ListInput) ([]inbox.Item, error) {
	return []inbox.Item{{CaptureID: "capture-id", SelectedText: "hello", InputMode: "manual", Status: "new", JobStatus: "done"}}, nil
}

func (routerFakeInboxService) SetStatus(_ context.Context, captureID, status string) error {
	if captureID == "" || status == "" {
		return inbox.ErrInvalidInput
	}
	return nil
}

type routerFakeKnowledgeService struct{}

func (routerFakeKnowledgeService) MarkUnknown(_ context.Context, knowledgeItemID string) (knowledge.MarkResult, error) {
	return knowledge.MarkResult{KnowledgeItemID: knowledgeItemID, Status: knowledge.StatusActive, WrongCount: 1}, nil
}

func (routerFakeKnowledgeService) MarkKnown(_ context.Context, knowledgeItemID string) (knowledge.MarkResult, error) {
	return knowledge.MarkResult{KnowledgeItemID: knowledgeItemID, Status: knowledge.StatusKnown}, nil
}

func (routerFakeKnowledgeService) ListByCapture(_ context.Context, _ string) ([]knowledge.CaptureItem, error) {
	return []knowledge.CaptureItem{{KnowledgeItemID: "know-id", SurfaceText: "stale", Status: knowledge.StatusActive}}, nil
}

type routerFakeReviewService struct{}

func (routerFakeReviewService) Due(_ context.Context, _ review.DueInput) ([]review.Card, error) {
	return []review.Card{{CardID: "card-id", KnowledgeItemID: "know-id", CardType: "meaning", Question: "q", State: review.CardStateNew}}, nil
}

func (routerFakeReviewService) StartSession(_ context.Context, _ review.DueInput) ([]review.Card, error) {
	return []review.Card{{CardID: "card-id", CardType: "meaning", Question: "q", State: review.CardStateNew}}, nil
}

func (routerFakeReviewService) Grade(_ context.Context, input review.GradeInput) (review.GradeResult, error) {
	return review.GradeResult{CardID: input.CardID, Rating: input.Rating, State: review.CardStateReview, Reps: 1}, nil
}

type routerFakeDashboardService struct{}

func (routerFakeDashboardService) Summary(_ context.Context) (stats.Summary, error) {
	return stats.Summary{TodaySearchCount: 1, DueCardCount: 2}, nil
}

type routerFakeSuggestService struct{}

func (routerFakeSuggestService) Suggest(_ context.Context, _ string) ([]suggest.Candidate, error) {
	return []suggest.Candidate{{English: "stale", Confidence: 0.9, GlossKo: "오래된", Source: suggest.SourceAI}}, nil
}

func (routerFakeSuggestService) ConfirmPick(_ context.Context, _, _, _ string) error {
	return nil
}
