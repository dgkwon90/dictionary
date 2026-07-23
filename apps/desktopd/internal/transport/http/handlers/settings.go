package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

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

type Settings struct {
	svc       SettingsService
	effective EffectiveConfig
	log       *slog.Logger
}

func NewSettings(svc SettingsService, effective EffectiveConfig, log *slog.Logger) *Settings {
	return &Settings{svc: svc, effective: effective, log: log}
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

func (h *Settings) response(prefs settings.Preferences) settingsResponse {
	return settingsResponse{
		Preferences: preferencesPayload{
			NotificationsEnabled: prefs.NotificationsEnabled,
			MorningReviewTime:    prefs.MorningReviewTime,
			EveningReviewTime:    prefs.EveningReviewTime,
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
// response. PUT is a full replace: every field is sent.
type preferencesPayload struct {
	NotificationsEnabled bool   `json:"notifications_enabled"`
	MorningReviewTime    string `json:"morning_review_time"`
	EveningReviewTime    string `json:"evening_review_time"`
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
