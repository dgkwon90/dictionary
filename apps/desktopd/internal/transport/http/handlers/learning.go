package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"neulsang/desktopd/internal/domain/learning"
)

type LearningService interface {
	List(ctx context.Context, input learning.ListInput) ([]learning.Item, error)
	Retire(ctx context.Context, knowledgeItemID string) (learning.Item, error)
	Remove(ctx context.Context, knowledgeItemID string) (learning.Item, error)
	Restore(ctx context.Context, knowledgeItemID string) (learning.Item, error)
}

type Learning struct {
	svc LearningService
	log *slog.Logger
}

func NewLearning(svc LearningService, log *slog.Logger) *Learning {
	return &Learning{svc: svc, log: log}
}

// List serves GET /v1/learning. With no parameters it returns everything currently
// being learned, newest registration first.
func (h *Learning) List(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsedLimit
	}

	items, err := h.svc.List(r.Context(), learning.ListInput{
		Scope:      r.URL.Query().Get("scope"),
		Membership: r.URL.Query().Get("membership"),
		LearnKind:  r.URL.Query().Get("kind"),
		Query:      r.URL.Query().Get("q"),
		Limit:      limit,
	})
	if err != nil {
		if errors.Is(err, learning.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error("list learning items", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	responseItems := make([]learningItemResponse, 0, len(items))
	for _, item := range items {
		responseItems = append(responseItems, newLearningItemResponse(item))
	}
	writeJSON(w, http.StatusOK, learningListResponse{Items: responseItems})
}

// Retire serves POST /v1/learning/{id}/retire — "알겠어요".
func (h *Learning) Retire(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, "retire", h.svc.Retire)
}

// Remove serves DELETE /v1/learning/{id}. The row is kept with status 'removed'; the
// item simply leaves the list.
func (h *Learning) Remove(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, "remove", h.svc.Remove)
}

// Restore serves POST /v1/learning/{id}/restore — the undo for both exits.
func (h *Learning) Restore(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, "restore", h.svc.Restore)
}

func (h *Learning) setStatus(w http.ResponseWriter, r *http.Request, action string, apply func(context.Context, string) (learning.Item, error)) {
	knowledgeItemID := r.PathValue("id")
	item, err := apply(r.Context(), knowledgeItemID)
	if err != nil {
		switch {
		case errors.Is(err, learning.ErrItemNotFound):
			writeError(w, http.StatusNotFound, "learning item not found")
		case errors.Is(err, learning.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.log.Error("learning "+action, "knowledge_item_id", knowledgeItemID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, newLearningItemResponse(item))
}

type learningListResponse struct {
	Items []learningItemResponse `json:"items"`
}

type learningItemResponse struct {
	KnowledgeItemID string `json:"knowledge_item_id"`
	SurfaceText     string `json:"surface_text"`
	LearnKind       string `json:"learn_kind"`
	MeaningKo       string `json:"meaning_ko,omitempty"`
	PronunciationKo string `json:"pronunciation_ko,omitempty"`
	DescriptionKo   string `json:"description_ko,omitempty"`
	Status          string `json:"status"`
	AskCount        int    `json:"ask_count"`
	UnknownCount    int    `json:"unknown_count"`
	// attempt_count travels with accuracy on purpose: accuracy is 0 both for an item
	// that has never been reviewed and for one that has always been missed, and only
	// the count tells the screen which of the two it is showing.
	AttemptCount  int        `json:"attempt_count"`
	CorrectCount  int        `json:"correct_count"`
	Accuracy      float64    `json:"accuracy"`
	WeaknessScore float64    `json:"weakness_score"`
	RegisteredAt  time.Time  `json:"registered_at"`
	LastGradedAt  *time.Time `json:"last_graded_at,omitempty"`
	NextDueAt     *time.Time `json:"next_due_at,omitempty"`
	CardCount     int        `json:"card_count"`
}

func newLearningItemResponse(item learning.Item) learningItemResponse {
	response := learningItemResponse{
		KnowledgeItemID: item.KnowledgeItemID,
		SurfaceText:     item.SurfaceText,
		LearnKind:       item.LearnKind,
		MeaningKo:       item.MeaningKo,
		PronunciationKo: item.PronunciationKo,
		DescriptionKo:   item.DescriptionKo,
		Status:          item.Status,
		AskCount:        item.AskCount,
		UnknownCount:    item.UnknownCount,
		AttemptCount:    item.AttemptCount,
		CorrectCount:    item.CorrectCount,
		Accuracy:        item.Accuracy,
		WeaknessScore:   item.WeaknessScore,
		RegisteredAt:    item.RegisteredAt,
		CardCount:       item.CardCount,
	}
	if !item.LastGradedAt.IsZero() {
		response.LastGradedAt = &item.LastGradedAt
	}
	if !item.NextDueAt.IsZero() {
		response.NextDueAt = &item.NextDueAt
	}
	return response
}
