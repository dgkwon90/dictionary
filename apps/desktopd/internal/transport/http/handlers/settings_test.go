package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neulsang/desktopd/internal/domain/settings"
)

type fakeSettingsService struct {
	get     func(context.Context) (settings.Preferences, error)
	update  func(context.Context, settings.Preferences) (settings.Preferences, error)
	updated *settings.Preferences
}

func (f *fakeSettingsService) Get(ctx context.Context) (settings.Preferences, error) {
	return f.get(ctx)
}

func (f *fakeSettingsService) Update(ctx context.Context, prefs settings.Preferences) (settings.Preferences, error) {
	f.updated = &prefs
	return f.update(ctx, prefs)
}

func testEffective() EffectiveConfig {
	return EffectiveConfig{
		Addr:             "127.0.0.1:48989",
		DBPath:           "/tmp/neulsang.db",
		AIProvider:       "gemini",
		GeminiModel:      "gemini-flash-lite-latest",
		APIKeyConfigured: true,
	}
}

func TestSettingsGetReturnsPreferencesAndEffective(t *testing.T) {
	svc := &fakeSettingsService{get: func(context.Context) (settings.Preferences, error) {
		return settings.Preferences{NotificationsEnabled: true, MorningReviewTime: "09:00", EveningReviewTime: "21:00"}, nil
	}}
	handler := NewSettings(svc, testEffective(), slog.Default())
	recorder := httptest.NewRecorder()
	handler.Get(recorder, httptest.NewRequest(http.MethodGet, "/v1/settings", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body struct {
		Preferences struct {
			NotificationsEnabled bool   `json:"notifications_enabled"`
			MorningReviewTime    string `json:"morning_review_time"`
		} `json:"preferences"`
		Effective struct {
			AIProvider       string `json:"ai_provider"`
			APIKeyConfigured bool   `json:"api_key_configured"`
			DBPath           string `json:"db_path"`
		} `json:"effective"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Preferences.NotificationsEnabled || body.Preferences.MorningReviewTime != "09:00" {
		t.Fatalf("preferences = %#v", body.Preferences)
	}
	if body.Effective.AIProvider != "gemini" || !body.Effective.APIKeyConfigured || body.Effective.DBPath != "/tmp/neulsang.db" {
		t.Fatalf("effective = %#v", body.Effective)
	}
}

func TestSettingsGetNeverLeaksAPIKey(t *testing.T) {
	svc := &fakeSettingsService{get: func(context.Context) (settings.Preferences, error) {
		return settings.Defaults(), nil
	}}
	handler := NewSettings(svc, testEffective(), slog.Default())
	recorder := httptest.NewRecorder()
	handler.Get(recorder, httptest.NewRequest(http.MethodGet, "/v1/settings", nil))

	// The response exposes only a presence flag; no field should carry a key value.
	if strings.Contains(recorder.Body.String(), "api_key\"") && !strings.Contains(recorder.Body.String(), "api_key_configured") {
		t.Fatalf("response leaks api key field: %s", recorder.Body.String())
	}
}

func TestSettingsUpdateValid(t *testing.T) {
	svc := &fakeSettingsService{update: func(_ context.Context, p settings.Preferences) (settings.Preferences, error) {
		return p, nil
	}}
	handler := NewSettings(svc, testEffective(), slog.Default())
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings",
		strings.NewReader(`{"notifications_enabled":false,"morning_review_time":"07:30","evening_review_time":"22:15"}`))
	handler.Update(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if svc.updated == nil || svc.updated.MorningReviewTime != "07:30" || svc.updated.NotificationsEnabled {
		t.Fatalf("service received = %#v", svc.updated)
	}
}

func TestSettingsUpdateInvalidTimeReturns400(t *testing.T) {
	// Use the real service so validation runs end to end and nothing is persisted.
	repo := &recordingRepo{}
	handler := NewSettings(settings.NewService(repo), testEffective(), slog.Default())
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings",
		strings.NewReader(`{"notifications_enabled":true,"morning_review_time":"25:00","evening_review_time":"21:00"}`))
	handler.Update(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if repo.saved != nil {
		t.Fatalf("invalid update persisted: %#v", repo.saved)
	}
}

func TestSettingsUpdateBadJSONReturns400(t *testing.T) {
	svc := &fakeSettingsService{}
	handler := NewSettings(svc, testEffective(), slog.Default())
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings", strings.NewReader(`{not json`))
	handler.Update(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

type recordingRepo struct{ saved *settings.Preferences }

func (r *recordingRepo) Load(context.Context) (settings.Preferences, error) {
	return settings.Defaults(), nil
}

func (r *recordingRepo) Save(_ context.Context, p settings.Preferences) error {
	r.saved = &p
	return nil
}
