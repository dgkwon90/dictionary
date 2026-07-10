package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"neulsang/desktopd/internal/domain/knowledge"
)

type KnowledgeService interface {
	MarkUnknown(ctx context.Context, knowledgeItemID string) (knowledge.MarkResult, error)
	MarkKnown(ctx context.Context, knowledgeItemID string) (knowledge.MarkResult, error)
	ListByCapture(ctx context.Context, captureID string) ([]knowledge.CaptureItem, error)
}

type Knowledge struct {
	svc KnowledgeService
	log *slog.Logger
}

func NewKnowledge(svc KnowledgeService, log *slog.Logger) *Knowledge {
	return &Knowledge{svc: svc, log: log}
}

func (h *Knowledge) MarkUnknown(w http.ResponseWriter, r *http.Request) {
	h.mark(w, r, "mark-unknown", h.svc.MarkUnknown)
}

func (h *Knowledge) MarkKnown(w http.ResponseWriter, r *http.Request) {
	h.mark(w, r, "mark-known", h.svc.MarkKnown)
}

// ListByCapture serves GET /v1/captures/{id}/knowledge — the capture's extracted
// words with learner state, so the Inbox (#15) can offer per-word 모름/알아요.
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
			WrongCount:      item.WrongCount,
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
	WrongCount      int     `json:"wrong_count"`
}

func (h *Knowledge) mark(w http.ResponseWriter, r *http.Request, action string, fn func(context.Context, string) (knowledge.MarkResult, error)) {
	itemID := r.PathValue("id")
	result, err := fn(r.Context(), itemID)
	if err != nil {
		switch {
		case errors.Is(err, knowledge.ErrKnowledgeItemNotFound):
			writeError(w, http.StatusNotFound, "knowledge item not found")
		case errors.Is(err, knowledge.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.log.Error("mark knowledge item", "action", action, "knowledge_item_id", itemID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, knowledgeMarkResponse{
		KnowledgeItemID: result.KnowledgeItemID,
		Status:          result.Status,
		AskCount:        result.AskCount,
		WrongCount:      result.WrongCount,
		CandidateCount:  result.CandidateCount,
		CardsCreated:    result.CardsCreated,
	})
}

type knowledgeMarkResponse struct {
	KnowledgeItemID string `json:"knowledge_item_id"`
	Status          string `json:"status"`
	AskCount        int    `json:"ask_count"`
	WrongCount      int    `json:"wrong_count"`
	CandidateCount  int    `json:"candidate_count"`
	CardsCreated    int    `json:"cards_created"`
}
