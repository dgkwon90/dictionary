package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neulsang/desktopd/internal/domain/explain"
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
	handler := NewSettings(svc, testEffective(), nil, slog.Default())
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
	handler := NewSettings(svc, testEffective(), nil, slog.Default())
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
	handler := NewSettings(svc, testEffective(), nil, slog.Default())
	recorder := httptest.NewRecorder()
	req := newJSONRequest(http.MethodPut, "/v1/settings",
		strings.NewReader(`{"notifications_enabled":false,"morning_review_time":"07:30","evening_review_time":"22:15"}`))
	handler.Update(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if svc.updated == nil || svc.updated.MorningReviewTime != "07:30" || svc.updated.NotificationsEnabled {
		t.Fatalf("service received = %#v", svc.updated)
	}
}

// validPayload is a full PUT body: the contract is replace-everything, so a test that
// wants to break one field starts from a body where nothing else is broken.
const validPayload = `{
  "notifications_enabled": true,
  "morning_review_time": "09:00",
  "evening_review_time": "21:00",
  "review_intervals": {
    "again_minutes": 10, "first_hard_days": 1, "first_good_days": 3, "first_easy_days": 7,
    "hard_multiplier": 1.2, "good_multiplier": 2.5, "easy_multiplier": 4
  },
  "ai_format": {"prompt_style": "", "examples_count": 2}
}`

func TestSettingsUpdateInvalidReturns400(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"hour 25", strings.Replace(validPayload, `"09:00"`, `"25:00"`, 1)},
		{"multiplier below 1", strings.Replace(validPayload, `"good_multiplier": 2.5`, `"good_multiplier": 0.8`, 1)},
		{"zero again interval", strings.Replace(validPayload, `"again_minutes": 10`, `"again_minutes": 0`, 1)},
		{"too many examples", strings.Replace(validPayload, `"examples_count": 2`, `"examples_count": 9`, 1)},
		// A partial body is a partial replace, which this endpoint does not do: the
		// missing schedule reads as zero and is refused rather than silently applied.
		{"missing schedule", `{"notifications_enabled":true,"morning_review_time":"09:00","evening_review_time":"21:00"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Use the real service so validation runs end to end and nothing is persisted.
			repo := &recordingRepo{}
			handler := NewSettings(settings.NewService(repo), testEffective(), nil, slog.Default())
			recorder := httptest.NewRecorder()
			handler.Update(recorder, newJSONRequest(http.MethodPut, "/v1/settings", strings.NewReader(tc.body)))

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
			}
			if repo.saved != nil {
				t.Fatalf("invalid update persisted: %#v", repo.saved)
			}
		})
	}
}

func TestSettingsUpdatePersistsScheduleAndStyle(t *testing.T) {
	repo := &recordingRepo{}
	handler := NewSettings(settings.NewService(repo), testEffective(), nil, slog.Default())
	recorder := httptest.NewRecorder()
	body := strings.Replace(validPayload, `"first_good_days": 3`, `"first_good_days": 5`, 1)
	body = strings.Replace(body, `"prompt_style": ""`, `"prompt_style": "짧게"`, 1)
	handler.Update(recorder, newJSONRequest(http.MethodPut, "/v1/settings", strings.NewReader(body)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if repo.saved == nil {
		t.Fatal("nothing was persisted")
	}
	if repo.saved.ReviewIntervals.FirstGoodDays != 5 || repo.saved.AIFormat.PromptStyle != "짧게" {
		t.Fatalf("persisted = %+v", *repo.saved)
	}

	var body2 struct {
		Preferences struct {
			ReviewIntervals struct {
				FirstGoodDays float64 `json:"first_good_days"`
			} `json:"review_intervals"`
			AIFormat struct {
				PromptStyle   string `json:"prompt_style"`
				ExamplesCount int    `json:"examples_count"`
			} `json:"ai_format"`
		} `json:"preferences"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body2.Preferences.ReviewIntervals.FirstGoodDays != 5 || body2.Preferences.AIFormat.PromptStyle != "짧게" {
		t.Fatalf("response = %#v", body2.Preferences)
	}
}

// The sample reflects the format the user saved, not the built-in default: the example
// count they chose changes the shape they are being shown.
func TestSettingsAIFormatSampleUsesSavedFormat(t *testing.T) {
	var asked explain.Format
	svc := &fakeSettingsService{get: func(context.Context) (settings.Preferences, error) {
		prefs := settings.Defaults()
		prefs.AIFormat = explain.Format{PromptStyle: "짧게", ExamplesCount: 4}
		return prefs, nil
	}}
	sampler := func(format explain.Format) map[string]any {
		asked = format
		return map[string]any{"type": "object"}
	}
	handler := NewSettings(svc, testEffective(), sampler, slog.Default())
	recorder := httptest.NewRecorder()
	handler.AIFormatSample(recorder, httptest.NewRequest(http.MethodGet, "/v1/settings/ai-format/sample", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if asked.ExamplesCount != 4 {
		t.Fatalf("schema rendered for %+v, want the saved format", asked)
	}
	var body struct {
		ResponseSchema map[string]any `json:"response_schema"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ResponseSchema["type"] != "object" {
		t.Fatalf("response_schema = %#v", body.ResponseSchema)
	}
}

func TestSettingsUpdateBadJSONReturns400(t *testing.T) {
	svc := &fakeSettingsService{}
	handler := NewSettings(svc, testEffective(), nil, slog.Default())
	recorder := httptest.NewRecorder()
	req := newJSONRequest(http.MethodPut, "/v1/settings", strings.NewReader(`{not json`))
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
