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
	"neulsang/desktopd/internal/domain/knowledge"
	"neulsang/desktopd/internal/domain/learning"
	"neulsang/desktopd/internal/domain/notification"
	"neulsang/desktopd/internal/domain/outbox"
	"neulsang/desktopd/internal/domain/review"
	"neulsang/desktopd/internal/domain/search"
	"neulsang/desktopd/internal/domain/settings"
	"neulsang/desktopd/internal/domain/stats"
	"neulsang/desktopd/internal/domain/suggest"
	"neulsang/desktopd/internal/transport/http/handlers"
)

func TestHealthz(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/healthz", nil)

	NewRouter(slog.Default(), Set{}).ServeHTTP(recorder, request)

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

	NewRouter(slog.Default(), Set{}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusNotFound {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusNotFound)
	}
}

func TestHealthzMethodNotAllowed(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/healthz", nil)

	NewRouter(slog.Default(), Set{}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestCapturesRoute(t *testing.T) {
	handler := handlers.NewCapture(routerFakeCaptureCreator{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/captures", strings.NewReader(`{"text":"hello","input_mode":"manual"}`))
	request.Header.Set("Content-Type", "application/json")

	NewRouter(slog.Default(), Set{Capture: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusCreated {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusCreated)
	}
}

func TestCapturesGetMethodNotAllowed(t *testing.T) {
	handler := handlers.NewCapture(routerFakeCaptureCreator{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/captures", nil)

	NewRouter(slog.Default(), Set{Capture: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestExplanationRoute(t *testing.T) {
	handler := handlers.NewExplanation(routerFakeExplanationReader{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/captures/capture-id/explanation", nil)

	NewRouter(slog.Default(), Set{Explanation: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestExplanationPostMethodNotAllowed(t *testing.T) {
	handler := handlers.NewExplanation(routerFakeExplanationReader{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/captures/capture-id/explanation", nil)

	NewRouter(slog.Default(), Set{Explanation: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestSearchListRoute(t *testing.T) {
	handler := handlers.NewSearch(routerFakeSearchService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/searches?view=all", nil)

	NewRouter(slog.Default(), Set{Search: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestSearchTriageRoutes(t *testing.T) {
	for _, action := range []string{"open", "learn", "discard"} {
		t.Run(action, func(t *testing.T) {
			handler := handlers.NewSearch(routerFakeSearchService{}, slog.Default())
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(nethttp.MethodPost, "/v1/searches/capture-id/"+action, nil)

			NewRouter(slog.Default(), Set{Search: handler}).ServeHTTP(recorder, request)

			if recorder.Code != nethttp.StatusOK {
				t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
			}
		})
	}
}

func TestSearchTriageGetMethodNotAllowed(t *testing.T) {
	handler := handlers.NewSearch(routerFakeSearchService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/searches/capture-id/learn", nil)

	NewRouter(slog.Default(), Set{Search: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestReviewDueRoute(t *testing.T) {
	handler := handlers.NewReview(routerFakeReviewService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/reviews/due", nil)

	NewRouter(slog.Default(), Set{Review: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestReviewDuePostMethodNotAllowed(t *testing.T) {
	handler := handlers.NewReview(routerFakeReviewService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/reviews/due", nil)

	NewRouter(slog.Default(), Set{Review: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestReviewPracticeCardsRoute(t *testing.T) {
	handler := handlers.NewReview(routerFakeReviewService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/practice/cards", nil)

	NewRouter(slog.Default(), Set{Review: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestReviewSessionStartRoute(t *testing.T) {
	handler := handlers.NewReview(routerFakeReviewService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/reviews/session/start", nil)

	NewRouter(slog.Default(), Set{Review: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestPracticeGradeRoute(t *testing.T) {
	handler := handlers.NewReview(routerFakeReviewService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/practice/card-1/grade", strings.NewReader(`{"rating":"good","elapsed_ms":100}`))
	request.Header.Set("Content-Type", "application/json")

	NewRouter(slog.Default(), Set{Review: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestReviewGradeRoute(t *testing.T) {
	handler := handlers.NewReview(routerFakeReviewService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/reviews/card-1/grade", strings.NewReader(`{"rating":"good","elapsed_ms":100}`))
	request.Header.Set("Content-Type", "application/json")

	NewRouter(slog.Default(), Set{Review: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestDashboardSummaryRoute(t *testing.T) {
	handler := handlers.NewDashboard(routerFakeDashboardService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/dashboard/summary", nil)

	NewRouter(slog.Default(), Set{Dashboard: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestSuggestRoute(t *testing.T) {
	handler := handlers.NewSuggest(routerFakeSuggestService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/suggest?q=스테일", nil)

	NewRouter(slog.Default(), Set{Suggest: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestSettingsGetRoute(t *testing.T) {
	handler := handlers.NewSettings(routerFakeSettingsService{}, handlers.EffectiveConfig{AIProvider: "mock"}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/settings", nil)

	NewRouter(slog.Default(), Set{Settings: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestSettingsPutRoute(t *testing.T) {
	handler := handlers.NewSettings(routerFakeSettingsService{}, handlers.EffectiveConfig{AIProvider: "mock"}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPut, "/v1/settings",
		strings.NewReader(`{"notifications_enabled":true,"morning_review_time":"09:00","evening_review_time":"21:00"}`))
	request.Header.Set("Content-Type", "application/json")

	NewRouter(slog.Default(), Set{Settings: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestNotificationsListRoute(t *testing.T) {
	handler := handlers.NewNotification(routerFakeNotificationService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/notifications", nil)

	NewRouter(slog.Default(), Set{Notification: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestNotificationsAckRoute(t *testing.T) {
	handler := handlers.NewNotification(routerFakeNotificationService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/notifications/notif-1/ack", nil)

	NewRouter(slog.Default(), Set{Notification: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusOK)
	}
}

func TestSyncStatusRoute(t *testing.T) {
	handler := handlers.NewSync(routerFakeSyncService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/sync/status", nil)

	NewRouter(slog.Default(), Set{Sync: handler}).ServeHTTP(recorder, request)

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

type routerFakeSearchService struct{}

func (routerFakeSearchService) List(context.Context, search.ListInput) ([]search.Item, error) {
	return []search.Item{{CaptureID: "capture-id", SelectedText: "hello", InputMode: "manual", LearnKind: "word", TriageState: "unseen", JobStatus: "done"}}, nil
}

func (routerFakeSearchService) Triage(_ context.Context, captureID string, _ capture.Transition) (search.TriageResult, error) {
	if captureID == "" {
		return search.TriageResult{}, search.ErrInvalidInput
	}
	return search.TriageResult{CaptureID: captureID, TriageState: "learning"}, nil
}

func (routerFakeSearchService) Get(_ context.Context, captureID string) (search.Detail, error) {
	return search.Detail{Item: search.Item{CaptureID: captureID, SelectedText: "hello", LearnKind: "sentence", TriageState: "needs_selection"}}, nil
}

func (routerFakeSearchService) Select(context.Context, string, string, bool) error {
	return nil
}

func (routerFakeSearchService) CompleteSelection(_ context.Context, input search.CompleteInput) (search.TriageResult, error) {
	return search.TriageResult{CaptureID: input.CaptureID, TriageState: "learning"}, nil
}

func TestCaptureKnowledgeListRoute(t *testing.T) {
	handler := handlers.NewKnowledge(routerFakeKnowledgeService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/captures/cap-id/knowledge", nil)

	NewRouter(slog.Default(), Set{Knowledge: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK || !strings.Contains(recorder.Body.String(), `"knowledge_item_id":"know-id"`) {
		t.Errorf("status = %d body = %q", recorder.Code, recorder.Body.String())
	}
}

type routerFakeKnowledgeService struct{}

func (routerFakeKnowledgeService) ListByCapture(_ context.Context, _ string) ([]knowledge.CaptureItem, error) {
	return []knowledge.CaptureItem{{KnowledgeItemID: "know-id", SurfaceText: "stale", Status: learning.StatusActive}}, nil
}

type routerFakeReviewService struct{}

func (routerFakeReviewService) Due(_ context.Context, _ review.DueInput) ([]review.Card, error) {
	return []review.Card{{CardID: "card-id", KnowledgeItemID: "know-id", CardType: "meaning", Question: "q", State: review.CardStateNew}}, nil
}

func (routerFakeReviewService) StartSession(_ context.Context, _ review.DueInput) ([]review.Card, error) {
	return []review.Card{{CardID: "card-id", CardType: "meaning", Question: "q", State: review.CardStateNew}}, nil
}

func (routerFakeReviewService) Practice(_ context.Context, _ review.PracticeInput) ([]review.Card, error) {
	return []review.Card{{CardID: "card-id", CardType: "meaning", Question: "q", State: review.CardStateReview}}, nil
}

func (routerFakeReviewService) Grade(_ context.Context, input review.GradeInput) (review.GradeResult, error) {
	return review.GradeResult{CardID: input.CardID, Rating: input.Rating, State: review.CardStateReview, Reps: 1}, nil
}

func (routerFakeReviewService) GradePractice(_ context.Context, input review.GradeInput) (review.PracticeResult, error) {
	return review.PracticeResult{CardID: input.CardID, Rating: input.Rating, AttemptCount: 1, CorrectCount: 1, Accuracy: 1}, nil
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

type routerFakeSettingsService struct{}

func (routerFakeSettingsService) Get(context.Context) (settings.Preferences, error) {
	return settings.Defaults(), nil
}

func (routerFakeSettingsService) Update(_ context.Context, p settings.Preferences) (settings.Preferences, error) {
	return p, nil
}

type routerFakeNotificationService struct{}

func (routerFakeNotificationService) Pending(context.Context) (notification.Pending, error) {
	return notification.Pending{}, nil
}

func (routerFakeNotificationService) Recent(context.Context, int) ([]notification.Notification, error) {
	return nil, nil
}

func (routerFakeNotificationService) Ack(context.Context, string) error {
	return nil
}

func (routerFakeNotificationService) AckCapture(context.Context, string) error {
	return nil
}

type routerFakeSyncService struct{}

func (routerFakeSyncService) Status(context.Context) (outbox.Status, error) {
	return outbox.Status{Enabled: true, Pending: 1}, nil
}

type routerFakeLearningService struct{}

func (routerFakeLearningService) List(_ context.Context, _ learning.ListInput) ([]learning.Item, error) {
	return []learning.Item{{KnowledgeItemID: "know-id", SurfaceText: "stale", LearnKind: "word", Status: learning.StatusActive}}, nil
}

func (routerFakeLearningService) Retire(_ context.Context, knowledgeItemID string) (learning.Item, error) {
	return learning.Item{KnowledgeItemID: knowledgeItemID, Status: learning.StatusKnown}, nil
}

func (routerFakeLearningService) Remove(_ context.Context, knowledgeItemID string) (learning.Item, error) {
	return learning.Item{KnowledgeItemID: knowledgeItemID, Status: learning.StatusRemoved}, nil
}

func TestLearningListRoute(t *testing.T) {
	handler := handlers.NewLearning(routerFakeLearningService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/learning?scope=today&kind=word", nil)

	NewRouter(slog.Default(), Set{Learning: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK || !strings.Contains(recorder.Body.String(), `"surface_text":"stale"`) {
		t.Errorf("status = %d body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestLearningRetireRoute(t *testing.T) {
	handler := handlers.NewLearning(routerFakeLearningService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodPost, "/v1/learning/know-id/retire", nil)

	NewRouter(slog.Default(), Set{Learning: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"known"`) {
		t.Errorf("status = %d body = %q", recorder.Code, recorder.Body.String())
	}
}

// DELETE and POST .../retire share the {id} segment, so the router has to keep them
// apart by method rather than by path.
func TestLearningRemoveRoute(t *testing.T) {
	handler := handlers.NewLearning(routerFakeLearningService{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodDelete, "/v1/learning/know-id", nil)

	NewRouter(slog.Default(), Set{Learning: handler}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"removed"`) {
		t.Errorf("status = %d body = %q", recorder.Code, recorder.Body.String())
	}
}

// A handler-less Set must 404 rather than panic: bootstrap serves a healthz-only
// router before its dependencies exist.
func TestLearningRouteAbsentWithoutHandler(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(nethttp.MethodGet, "/v1/learning", nil)

	NewRouter(slog.Default(), Set{}).ServeHTTP(recorder, request)

	if recorder.Code != nethttp.StatusNotFound {
		t.Errorf("status = %d, want %d", recorder.Code, nethttp.StatusNotFound)
	}
}
