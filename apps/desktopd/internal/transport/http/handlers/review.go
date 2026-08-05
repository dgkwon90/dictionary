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
	Practice(ctx context.Context, input review.PracticeInput) ([]review.Card, error)
	Grade(ctx context.Context, input review.GradeInput) (review.GradeResult, error)
	GradePractice(ctx context.Context, input review.GradeInput) (review.PracticeResult, error)
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

func (h *Review) PracticeCards(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	limit := 0
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsedLimit
	}

	cards, err := h.svc.Practice(r.Context(), review.PracticeInput{Query: query, Limit: limit})
	if err != nil {
		if errors.Is(err, review.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error("list practice review cards", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeCards(w, cards)
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

	writeCards(w, cards)
}

func writeCards(w http.ResponseWriter, cards []review.Card) {
	responseCards := make([]reviewCardResponse, 0, len(cards))
	for _, card := range cards {
		responseCards = append(responseCards, reviewCardResponse{
			CardID:          card.CardID,
			KnowledgeItemID: card.KnowledgeItemID,
			CardType:        card.CardType,
			Question:        card.Question,
			Answer:          card.Answer,
			Explanation:     card.Explanation,
			State:           card.State,
			DueAt:           card.DueAt,
		})
	}
	writeJSON(w, http.StatusOK, reviewDueResponse{Cards: responseCards})
}

func (h *Review) Grade(w http.ResponseWriter, r *http.Request) {
	input, ok := h.decodeGrade(w, r)
	if !ok {
		return
	}

	result, err := h.svc.Grade(r.Context(), input)
	if err != nil {
		h.writeGradeError(w, "grade review card", input.CardID, err)
		return
	}

	writeJSON(w, http.StatusOK, reviewGradeResponse{
		CardID:       result.CardID,
		Rating:       result.Rating,
		State:        result.State,
		Reps:         result.Reps,
		DueAt:        result.DueAt,
		Accuracy:     result.Accuracy,
		AttemptCount: result.AttemptCount,
		CorrectCount: result.CorrectCount,
	})
}

// GradePractice records a practice answer (POST /v1/practice/{id}/grade). The response
// omits the schedule fields the review grade returns, because practice changes none of
// them — reporting a due date here would suggest it had just been recalculated.
func (h *Review) GradePractice(w http.ResponseWriter, r *http.Request) {
	input, ok := h.decodeGrade(w, r)
	if !ok {
		return
	}

	result, err := h.svc.GradePractice(r.Context(), input)
	if err != nil {
		h.writeGradeError(w, "grade practice card", input.CardID, err)
		return
	}

	writeJSON(w, http.StatusOK, practiceGradeResponse{
		CardID:       result.CardID,
		Rating:       result.Rating,
		Accuracy:     result.Accuracy,
		AttemptCount: result.AttemptCount,
		CorrectCount: result.CorrectCount,
	})
}

func (h *Review) decodeGrade(w http.ResponseWriter, r *http.Request) (review.GradeInput, bool) {
	var request struct {
		Rating    string `json:"rating"`
		ElapsedMs int    `json:"elapsed_ms"`
	}
	if err := decodeJSONBody(w, r, &request, 1<<20, h.log); err != nil {
		writeJSONDecodeError(w, err)
		return review.GradeInput{}, false
	}
	return review.GradeInput{
		CardID:    r.PathValue("id"),
		Rating:    request.Rating,
		ElapsedMs: request.ElapsedMs,
	}, true
}

func (h *Review) writeGradeError(w http.ResponseWriter, action, cardID string, err error) {
	switch {
	case errors.Is(err, review.ErrCardNotFound):
		writeError(w, http.StatusNotFound, "review card not found")
	case errors.Is(err, review.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		h.log.Error(action, "card_id", cardID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

type reviewDueResponse struct {
	Cards []reviewCardResponse `json:"cards"`
}

type reviewCardResponse struct {
	CardID          string    `json:"card_id"`
	KnowledgeItemID string    `json:"knowledge_item_id"`
	CardType        string    `json:"card_type"`
	Question        string    `json:"question"`
	Answer          string    `json:"answer"`
	Explanation     string    `json:"explanation,omitempty"`
	State           string    `json:"state"`
	DueAt           time.Time `json:"due_at"`
}

type practiceGradeResponse struct {
	CardID       string  `json:"card_id"`
	Rating       string  `json:"rating"`
	Accuracy     float64 `json:"accuracy"`
	AttemptCount int     `json:"attempt_count"`
	CorrectCount int     `json:"correct_count"`
}

type reviewGradeResponse struct {
	CardID       string    `json:"card_id"`
	Rating       string    `json:"rating"`
	State        string    `json:"state"`
	Reps         int       `json:"reps"`
	DueAt        time.Time `json:"due_at"`
	Accuracy     float64   `json:"accuracy"`
	AttemptCount int       `json:"attempt_count"`
	CorrectCount int       `json:"correct_count"`
}
