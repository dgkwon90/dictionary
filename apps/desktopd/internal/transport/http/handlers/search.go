package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"neulsang/desktopd/internal/domain/capture"
	"neulsang/desktopd/internal/domain/search"
)

type SearchService interface {
	List(ctx context.Context, input search.ListInput) ([]search.Item, error)
	Triage(ctx context.Context, captureID string, transition capture.Transition) (search.TriageResult, error)
}

type Search struct {
	svc SearchService
	log *slog.Logger
}

func NewSearch(svc SearchService, log *slog.Logger) *Search {
	return &Search{svc: svc, log: log}
}

// List serves GET /v1/searches. With no query parameters it returns the unresolved
// searches — the ones still waiting on a decision — which is what the screen opens to.
func (h *Search) List(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsedLimit
	}

	items, err := h.svc.List(r.Context(), search.ListInput{
		View:      r.URL.Query().Get("view"),
		LearnKind: r.URL.Query().Get("kind"),
		Limit:     limit,
	})
	if err != nil {
		if errors.Is(err, search.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error("list searches", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	responseItems := make([]searchItemResponse, 0, len(items))
	for _, item := range items {
		responseItems = append(responseItems, searchItemResponse{
			CaptureID:    item.CaptureID,
			SelectedText: item.SelectedText,
			SourceApp:    item.SourceApp,
			SourceType:   item.SourceType,
			InputMode:    item.InputMode,
			LearnKind:    item.LearnKind,
			TriageState:  item.TriageState,
			JobStatus:    item.JobStatus,
			BriefKo:      item.BriefKo,
			CreatedAt:    item.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, searchListResponse{Items: responseItems})
}

// Open marks a sentence as awaiting word selection.
func (h *Search) Open(w http.ResponseWriter, r *http.Request) {
	h.triage(w, r, capture.TransitionOpen)
}

// Learn is "학습할래요".
func (h *Search) Learn(w http.ResponseWriter, r *http.Request) {
	h.triage(w, r, capture.TransitionLearn)
}

// Discard throws a search away.
func (h *Search) Discard(w http.ResponseWriter, r *http.Request) {
	h.triage(w, r, capture.TransitionDiscard)
}

func (h *Search) triage(w http.ResponseWriter, r *http.Request, transition capture.Transition) {
	captureID := r.PathValue("id")
	result, err := h.svc.Triage(r.Context(), captureID, transition)
	if err != nil {
		switch {
		case errors.Is(err, search.ErrCaptureNotFound):
			writeError(w, http.StatusNotFound, "capture not found")
		// ErrSelectionRequired wraps capture.ErrInvalidInput, so it is already a 400;
		// it is listed first so its own message reaches the user instead of a generic one.
		case errors.Is(err, capture.ErrSelectionRequired):
			writeError(w, http.StatusBadRequest, "모르는 단어를 먼저 선택하세요")
		case errors.Is(err, capture.ErrInvalidInput), errors.Is(err, search.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.log.Error("triage capture", "capture_id", captureID, "transition", string(transition), "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	learningItems := result.LearningItemIDs
	if learningItems == nil {
		learningItems = []string{}
	}
	writeJSON(w, http.StatusOK, searchTriageResponse{
		CaptureID:       result.CaptureID,
		TriageState:     result.TriageState,
		LearningItemIDs: learningItems,
		CardsCreated:    result.CardsCreated,
	})
}

type searchListResponse struct {
	Items []searchItemResponse `json:"items"`
}

type searchItemResponse struct {
	CaptureID    string `json:"capture_id"`
	SelectedText string `json:"selected_text"`
	SourceApp    string `json:"source_app,omitempty"`
	SourceType   string `json:"source_type,omitempty"`
	InputMode    string `json:"input_mode"`
	// LearnKind is absent until the lookup finishes and the server classifies it.
	LearnKind   string    `json:"learn_kind,omitempty"`
	TriageState string    `json:"triage_state"`
	JobStatus   string    `json:"job_status"`
	BriefKo     string    `json:"brief_ko,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type searchTriageResponse struct {
	CaptureID       string   `json:"capture_id"`
	TriageState     string   `json:"triage_state"`
	LearningItemIDs []string `json:"learning_item_ids"`
	CardsCreated    int      `json:"cards_created"`
}
