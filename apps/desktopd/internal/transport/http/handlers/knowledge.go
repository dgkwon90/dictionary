package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"neulsang/desktopd/internal/domain/knowledge"
)

type KnowledgeService interface {
	ListByCapture(ctx context.Context, captureID string) ([]knowledge.CaptureItem, error)
}

type Knowledge struct {
	svc KnowledgeService
	log *slog.Logger
}

func NewKnowledge(svc KnowledgeService, log *slog.Logger) *Knowledge {
	return &Knowledge{svc: svc, log: log}
}

// ListByCapture serves GET /v1/captures/{id}/knowledge — the capture's extracted
// words with learner state, so the detail panel can render each one.
func (h *Knowledge) ListByCapture(w http.ResponseWriter, r *http.Request) {
	captureID := r.PathValue("id")
	items, err := h.svc.ListByCapture(r.Context(), captureID)
	if err != nil {
		switch {
		case errors.Is(err, knowledge.ErrCaptureNotFound):
			writeError(w, http.StatusNotFound, "capture not found")
		case errors.Is(err, knowledge.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.log.Error("list capture knowledge", "capture_id", captureID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	responseItems := make([]captureKnowledgeItem, 0, len(items))
	for _, item := range items {
		responseItems = append(responseItems, captureKnowledgeItem{
			KnowledgeItemID: item.KnowledgeItemID,
			SurfaceText:     item.SurfaceText,
			ItemType:        item.ItemType,
			PronunciationKo: item.PronunciationKo,
			MeaningKo:       item.MeaningKo,
			Role:            item.Role,
			Confidence:      item.Confidence,
			Status:          item.Status,
			AskCount:        item.AskCount,
			UnknownCount:    item.UnknownCount,
		})
	}
	writeJSON(w, http.StatusOK, captureKnowledgeResponse{
		CaptureID: captureID,
		Items:     responseItems,
	})
}

type captureKnowledgeResponse struct {
	CaptureID string                 `json:"capture_id"`
	Items     []captureKnowledgeItem `json:"items"`
}

type captureKnowledgeItem struct {
	KnowledgeItemID string  `json:"knowledge_item_id"`
	SurfaceText     string  `json:"surface_text"`
	ItemType        string  `json:"item_type"`
	PronunciationKo string  `json:"pronunciation_ko,omitempty"`
	MeaningKo       string  `json:"meaning_ko,omitempty"`
	Role            string  `json:"role"`
	Confidence      float64 `json:"confidence"`
	Status          string  `json:"status"`
	AskCount        int     `json:"ask_count"`
	UnknownCount    int     `json:"unknown_count"`
}
