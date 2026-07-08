package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"neulsang/desktopd/internal/domain/review"
)

type ReviewService interface {
	Due(ctx context.Context, input review.DueInput) ([]review.Card, error)
	StartSession(ctx context.Context, input review.DueInput) ([]review.Card, error)
	Grade(ctx context.Context, input review.GradeInput) (review.GradeResult, error)
}

type Review struct {
	svc ReviewService
	log *slog.Logger
}

func NewReview(svc ReviewService, log *slog.Logger) *Review {
	return &Review{svc: svc, log: log}
}

func (h *Review) Due(w http.ResponseWriter, r *http.Request) {
	h.listCards(w, r, "list due review cards", h.svc.Due)
}

// StartSession begins a review session (PRD §15.5); for MVP it returns the due list.
func (h *Review) StartSession(w http.ResponseWriter, r *http.Request) {
	h.listCards(w, r, "start review session", h.svc.StartSession)
}

func (h *Review) listCards(w http.ResponseWriter, r *http.Request, action string, fn func(context.Context, review.DueInput) ([]review.Card, error)) {
	limit := 0
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsedLimit
	}

	cards, err := fn(r.Context(), review.DueInput{Limit: limit})
	if err != nil {
		if errors.Is(err, review.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error(action, "error", err)
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

func (h *Review) Grade(w http.ResponseWriter, r *http.Request) {
	cardID := r.PathValue("id")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer func() {
		if err := r.Body.Close(); err != nil {
			h.log.Error("close grade request body", "error", err)
		}
	}()

	var request struct {
		Rating    string `json:"rating"`
		ElapsedMs int    `json:"elapsed_ms"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.Grade(r.Context(), review.GradeInput{
		CardID:    cardID,
		Rating:    request.Rating,
		ElapsedMs: request.ElapsedMs,
	})
	if err != nil {
		switch {
		case errors.Is(err, review.ErrCardNotFound):
			writeError(w, http.StatusNotFound, "review card not found")
		case errors.Is(err, review.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.log.Error("grade review card", "card_id", cardID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, reviewGradeResponse{
		CardID:       result.CardID,
		Rating:       result.Rating,
		State:        result.State,
		Reps:         result.Reps,
		DueAt:        result.DueAt,
		MasteryScore: result.MasteryScore,
	})
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

type reviewGradeResponse struct {
	CardID       string    `json:"card_id"`
	Rating       string    `json:"rating"`
	State        string    `json:"state"`
	Reps         int       `json:"reps"`
	DueAt        time.Time `json:"due_at"`
	MasteryScore float64   `json:"mastery_score"`
}
