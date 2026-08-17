package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"neulsang/desktopd/internal/domain/explain"
	"neulsang/desktopd/internal/domain/review"
	"neulsang/desktopd/internal/domain/settings"
)

type SettingsService interface {
	Get(ctx context.Context) (settings.Preferences, error)
	Update(ctx context.Context, prefs settings.Preferences) (settings.Preferences, error)
}

// EffectiveConfig is the read-only infra config reflected to the Settings screen
// (PRD §10.7). It mirrors env/bootstrap values so the UI can show what the running
// process actually uses; APIKeyConfigured is a presence flag only — the key value is
// never exposed (ADR-0004 부록, #17).
type EffectiveConfig struct {
	Addr             string
	DBPath           string
	AIProvider       string
	GeminiModel      string
	APIKeyConfigured bool
}

// SchemaSampler renders the JSON schema the AI provider is asked to fill, for the
// read-only sample on the Settings screen (D7). It is a function so the transport layer
// does not import a provider package; bootstrap passes the real one in.
type SchemaSampler func(explain.Format) map[string]any

type Settings struct {
	svc       SettingsService
	effective EffectiveConfig
	schema    SchemaSampler
	log       *slog.Logger
}

func NewSettings(svc SettingsService, effective EffectiveConfig, schema SchemaSampler, log *slog.Logger) *Settings {
	return &Settings{svc: svc, effective: effective, schema: schema, log: log}
}

func (h *Settings) Get(w http.ResponseWriter, r *http.Request) {
	prefs, err := h.svc.Get(r.Context())
	if err != nil {
		h.log.Error("get settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, h.response(prefs))
}

func (h *Settings) Update(w http.ResponseWriter, r *http.Request) {
	var request preferencesPayload
	if err := decodeJSONBody(w, r, &request, 1<<20, h.log); err != nil {
		writeJSONDecodeError(w, err)
		return
	}

	prefs, err := h.svc.Update(r.Context(), settings.Preferences{
		NotificationsEnabled: request.NotificationsEnabled,
		MorningReviewTime:    request.MorningReviewTime,
		EveningReviewTime:    request.EveningReviewTime,
		ReviewIntervals:      request.ReviewIntervals.toDomain(),
		AIFormat: explain.Format{
			PromptStyle:   request.AIFormat.PromptStyle,
			ExamplesCount: request.AIFormat.ExamplesCount,
		},
	})
	if err != nil {
		if errors.Is(err, settings.ErrInvalidPreferences) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error("update settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, h.response(prefs))
}

// AIFormatSample answers the read-only schema sample the Settings screen shows next to
// the style box (D7). It is rendered against the user's saved format, not the default,
// because the example count they chose changes the shape they are looking at.
//
// The sample is deliberately display-only: the schema drives struct parsing, which
// drives knowledge extraction, so an editable one would let a renamed field fail every
// save with no way for the user to see what they broke.
func (h *Settings) AIFormatSample(w http.ResponseWriter, r *http.Request) {
	if h.schema == nil {
		writeError(w, http.StatusNotFound, "schema sample unavailable")
		return
	}
	prefs, err := h.svc.Get(r.Context())
	if err != nil {
		h.log.Error("get settings for schema sample", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"response_schema": h.schema(prefs.AIFormat),
	})
}

func (h *Settings) response(prefs settings.Preferences) settingsResponse {
	return settingsResponse{
		Preferences: preferencesPayload{
			NotificationsEnabled: prefs.NotificationsEnabled,
			MorningReviewTime:    prefs.MorningReviewTime,
			EveningReviewTime:    prefs.EveningReviewTime,
			ReviewIntervals:      intervalsPayloadOf(prefs.ReviewIntervals),
			AIFormat: aiFormatPayload{
				PromptStyle:   prefs.AIFormat.PromptStyle,
				ExamplesCount: prefs.AIFormat.ExamplesCount,
			},
		},
		Effective: effectiveConfigResponse{
			Addr:             h.effective.Addr,
			DBPath:           h.effective.DBPath,
			AIProvider:       h.effective.AIProvider,
			GeminiModel:      h.effective.GeminiModel,
			APIKeyConfigured: h.effective.APIKeyConfigured,
		},
	}
}

// preferencesPayload is both the PUT request body and the preferences half of the
// response. PUT is a full replace: every field is sent, and an omitted one is read as
// zero and refused by validation rather than left at its previous value.
type preferencesPayload struct {
	NotificationsEnabled bool             `json:"notifications_enabled"`
	MorningReviewTime    string           `json:"morning_review_time"`
	EveningReviewTime    string           `json:"evening_review_time"`
	ReviewIntervals      intervalsPayload `json:"review_intervals"`
	AIFormat             aiFormatPayload  `json:"ai_format"`
}

// intervalsPayload names each interval with its unit, because that is the number the
// user types: a missed card comes back in minutes, a remembered one in days.
type intervalsPayload struct {
	AgainMinutes   float64 `json:"again_minutes"`
	FirstHardDays  float64 `json:"first_hard_days"`
	FirstGoodDays  float64 `json:"first_good_days"`
	FirstEasyDays  float64 `json:"first_easy_days"`
	HardMultiplier float64 `json:"hard_multiplier"`
	GoodMultiplier float64 `json:"good_multiplier"`
	EasyMultiplier float64 `json:"easy_multiplier"`
}

func (p intervalsPayload) toDomain() review.Intervals {
	return review.Intervals{
		AgainMinutes:   p.AgainMinutes,
		FirstHardDays:  p.FirstHardDays,
		FirstGoodDays:  p.FirstGoodDays,
		FirstEasyDays:  p.FirstEasyDays,
		HardMultiplier: p.HardMultiplier,
		GoodMultiplier: p.GoodMultiplier,
		EasyMultiplier: p.EasyMultiplier,
	}
}

func intervalsPayloadOf(intervals review.Intervals) intervalsPayload {
	return intervalsPayload{
		AgainMinutes:   intervals.AgainMinutes,
		FirstHardDays:  intervals.FirstHardDays,
		FirstGoodDays:  intervals.FirstGoodDays,
		FirstEasyDays:  intervals.FirstEasyDays,
		HardMultiplier: intervals.HardMultiplier,
		GoodMultiplier: intervals.GoodMultiplier,
		EasyMultiplier: intervals.EasyMultiplier,
	}
}

// aiFormatPayload carries style only. The schema itself is read-only (D7) and is served
// separately by AIFormatSample.
type aiFormatPayload struct {
	PromptStyle   string `json:"prompt_style"`
	ExamplesCount int    `json:"examples_count"`
}

type effectiveConfigResponse struct {
	Addr             string `json:"addr"`
	DBPath           string `json:"db_path"`
	AIProvider       string `json:"ai_provider"`
	GeminiModel      string `json:"gemini_model"`
	APIKeyConfigured bool   `json:"api_key_configured"`
}

type settingsResponse struct {
	Preferences preferencesPayload      `json:"preferences"`
	Effective   effectiveConfigResponse `json:"effective"`
}
