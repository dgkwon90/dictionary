package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"neulsang/desktopd/internal/domain/capture"
)

type CaptureCreator interface {
	Create(ctx context.Context, input capture.CreateInput) (capture.CreateResult, error)
}

type Capture struct {
	svc CaptureCreator
	log *slog.Logger
}

func NewCapture(svc CaptureCreator, log *slog.Logger) *Capture {
	return &Capture{svc: svc, log: log}
}

func (h *Capture) Create(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Text       string `json:"text"`
		InputMode  string `json:"input_mode"`
		SourceApp  string `json:"source_app"`
		SourceType string `json:"source_type"`
	}
	if err := decodeJSONBody(w, r, &request, 1<<20, h.log); err != nil {
		writeJSONDecodeError(w, err)
		return
	}

	result, err := h.svc.Create(r.Context(), capture.CreateInput{
		Text:       request.Text,
		InputMode:  request.InputMode,
		SourceApp:  request.SourceApp,
		SourceType: request.SourceType,
	})
	if err != nil {
		if errors.Is(err, capture.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error("create capture", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, struct {
		CaptureID   string `json:"capture_id"`
		LookupJobID string `json:"lookup_job_id"`
		Status      string `json:"status"`
	}{
		CaptureID:   result.CaptureID,
		LookupJobID: result.LookupJobID,
		Status:      result.Status,
	})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}
