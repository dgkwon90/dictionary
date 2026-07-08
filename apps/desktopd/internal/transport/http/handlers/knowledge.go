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
