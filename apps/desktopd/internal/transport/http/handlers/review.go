package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"neulsang/desktopd/internal/domain/review"
)

type ReviewService interface {
	Due(ctx context.Context, input review.DueInput) ([]review.Card, error)
}

type Review struct {
	svc ReviewService
	log *slog.Logger
}

func NewReview(svc ReviewService, log *slog.Logger) *Review {
	return &Review{svc: svc, log: log}
}

func (h *Review) Due(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsedLimit
	}

	cards, err := h.svc.Due(r.Context(), review.DueInput{Limit: limit})
	if err != nil {
		if errors.Is(err, review.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error("list due review cards", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	responseCards := make([]reviewCardResponse, 0, len(cards))
	for _, card := range cards {
		responseCards = append(responseCards, reviewCardResponse{
			CardID:          card.CardID,
			KnowledgeItemID: card.KnowledgeItemID,
			CardType:        card.CardType,
			Question:        card.Question,
			State:           card.State,
			DueAt:           card.DueAt,
		})
	}
	writeJSON(w, http.StatusOK, reviewDueResponse{Cards: responseCards})
}

type reviewDueResponse struct {
	Cards []reviewCardResponse `json:"cards"`
}

type reviewCardResponse struct {
	CardID          string    `json:"card_id"`
	KnowledgeItemID string    `json:"knowledge_item_id"`
	CardType        string    `json:"card_type"`
	Question        string    `json:"question"`
	State           string    `json:"state"`
	DueAt           time.Time `json:"due_at"`
}
