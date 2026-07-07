package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"neulsang/desktopd/internal/domain/explain"
)

type ExplanationReader interface {
	GetSnapshot(ctx context.Context, captureID string) (explain.Snapshot, error)
}

type Explanation struct {
	reader ExplanationReader
	log    *slog.Logger
}

func NewExplanation(reader ExplanationReader, log *slog.Logger) *Explanation {
	return &Explanation{reader: reader, log: log}
}

func (h *Explanation) Get(w http.ResponseWriter, r *http.Request) {
	captureID := r.PathValue("id")
	snapshot, err := h.reader.GetSnapshot(r.Context(), captureID)
	if err != nil {
		if errors.Is(err, explain.ErrCaptureNotFound) {
			writeError(w, http.StatusNotFound, "capture not found")
			return
		}
		h.log.Error("get explanation snapshot", "capture_id", captureID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	response := explanationResponse{
		CaptureID: captureID,
		Status:    snapshot.Status,
	}
	if snapshot.Status == "failed" {
		response.ErrorMessage = snapshot.ErrorMessage
	}
	if snapshot.Result != nil {
		response.Explanation = &explanationBody{
			BriefKo:         snapshot.Result.BriefKo,
			DetailedKo:      snapshot.Result.DetailedKo,
			PronunciationKo: snapshot.Result.PronunciationKo,
			DomainCategory:  snapshot.Result.DomainCategory,
			Difficulty:      snapshot.Result.Difficulty,
			Examples:        snapshot.Result.Examples,
			SubItems:        snapshot.Result.SubItems,
		}
	}
	writeJSON(w, http.StatusOK, response)
}

type explanationResponse struct {
	CaptureID    string           `json:"capture_id"`
	Status       string           `json:"status"`
	ErrorMessage string           `json:"error_message,omitempty"`
	Explanation  *explanationBody `json:"explanation,omitempty"`
}

type explanationBody struct {
	BriefKo         string            `json:"brief_ko"`
	DetailedKo      string            `json:"detailed_ko"`
	PronunciationKo string            `json:"pronunciation_ko"`
	DomainCategory  string            `json:"domain_category"`
	Difficulty      float64           `json:"difficulty"`
	Examples        []explain.Example `json:"examples"`
	SubItems        []explain.SubItem `json:"sub_items"`
}
