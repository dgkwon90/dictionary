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
	Get(ctx context.Context, captureID string) (search.Detail, error)
	Triage(ctx context.Context, captureID string, transition capture.Transition) (search.TriageResult, error)
	Select(ctx context.Context, captureID, knowledgeItemID string, selected bool) error
	SetLearnKind(ctx context.Context, captureID, learnKind string) (search.TriageResult, error)
	CompleteSelection(ctx context.Context, input search.CompleteInput) (search.TriageResult, error)
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
		h.writeTriageError(w, captureID, "triage "+string(transition), err)
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

// Get serves GET /v1/searches/{id} — the detail panel: the result plus the terms found
// in it and which ones the user has picked.
func (h *Search) Get(w http.ResponseWriter, r *http.Request) {
	captureID := r.PathValue("id")
	detail, err := h.svc.Get(r.Context(), captureID)
	if err != nil {
		h.writeTriageError(w, captureID, "get", err)
		return
	}
	items := make([]searchDetailItemResponse, 0, len(detail.Items))
	for _, item := range detail.Items {
		items = append(items, searchDetailItemResponse{
			KnowledgeItemID: item.KnowledgeItemID,
			SurfaceText:     item.SurfaceText,
			MeaningKo:       item.MeaningKo,
			DescriptionKo:   item.DescriptionKo,
			PronunciationKo: item.PronunciationKo,
			CharStart:       item.CharStart,
			CharEnd:         item.CharEnd,
			Selected:        item.Selected,
		})
	}
	writeJSON(w, http.StatusOK, searchDetailResponse{
		CaptureID:    detail.CaptureID,
		SelectedText: detail.SelectedText,
		LearnKind:    detail.LearnKind,
		TriageState:  detail.TriageState,
		JobStatus:    detail.JobStatus,
		BriefKo:      detail.BriefKo,
		DetailedKo:   detail.DetailedKo,
		CreatedAt:    detail.CreatedAt,
		Items:        items,
	})
}

// SetKind corrects the server's word/sentence call (D1). The classification is
// automatic, so this is the one place the user can say it got it wrong.
func (h *Search) SetKind(w http.ResponseWriter, r *http.Request) {
	var request struct {
		LearnKind string `json:"learn_kind"`
	}
	if err := decodeJSONBody(w, r, &request, 1<<16, h.log); err != nil {
		writeJSONDecodeError(w, err)
		return
	}
	captureID := r.PathValue("id")
	result, err := h.svc.SetLearnKind(r.Context(), captureID, request.LearnKind)
	if err != nil {
		h.writeTriageError(w, captureID, "set-kind", err)
		return
	}
	writeJSON(w, http.StatusOK, searchTriageResponse{
		CaptureID:       result.CaptureID,
		TriageState:     result.TriageState,
		LearningItemIDs: []string{},
		CardsCreated:    0,
	})
}

// Select marks one extracted term as a word the user did not know.
func (h *Search) Select(w http.ResponseWriter, r *http.Request) {
	var request struct {
		KnowledgeItemID string `json:"knowledge_item_id"`
	}
	if err := decodeJSONBody(w, r, &request, 1<<16, h.log); err != nil {
		writeJSONDecodeError(w, err)
		return
	}
	h.setSelected(w, r, request.KnowledgeItemID, true)
}

// Deselect takes a word back off the list.
func (h *Search) Deselect(w http.ResponseWriter, r *http.Request) {
	h.setSelected(w, r, r.PathValue("knowledgeItemId"), false)
}

func (h *Search) setSelected(w http.ResponseWriter, r *http.Request, knowledgeItemID string, selected bool) {
	captureID := r.PathValue("id")
	if err := h.svc.Select(r.Context(), captureID, knowledgeItemID, selected); err != nil {
		h.writeTriageError(w, captureID, "select", err)
		return
	}
	writeJSON(w, http.StatusOK, searchSelectionResponse{
		CaptureID:       captureID,
		KnowledgeItemID: knowledgeItemID,
		Selected:        selected,
	})
}

// CompleteSelection finishes a sentence: it and the words picked out of it become
// learning items together.
func (h *Search) CompleteSelection(w http.ResponseWriter, r *http.Request) {
	var request struct {
		NoUnknownWords bool `json:"no_unknown_words"`
	}
	// An empty body is the common case ("I picked my words, done"), so a missing body
	// must not be an error — only decode when something was actually sent.
	if r.ContentLength > 0 {
		if err := decodeJSONBody(w, r, &request, 1<<16, h.log); err != nil {
			writeJSONDecodeError(w, err)
			return
		}
	}
	captureID := r.PathValue("id")
	result, err := h.svc.CompleteSelection(r.Context(), search.CompleteInput{
		CaptureID:      captureID,
		NoUnknownWords: request.NoUnknownWords,
	})
	if err != nil {
		h.writeTriageError(w, captureID, "complete-selection", err)
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

func (h *Search) writeTriageError(w http.ResponseWriter, captureID, action string, err error) {
	switch {
	case errors.Is(err, search.ErrCaptureNotFound):
		writeError(w, http.StatusNotFound, "capture not found")
	case errors.Is(err, capture.ErrSelectionRequired):
		writeError(w, http.StatusBadRequest, "모르는 단어를 먼저 선택하세요")
	case errors.Is(err, capture.ErrInvalidInput), errors.Is(err, search.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		h.log.Error("search "+action, "capture_id", captureID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

type searchDetailResponse struct {
	CaptureID    string                     `json:"capture_id"`
	SelectedText string                     `json:"selected_text"`
	LearnKind    string                     `json:"learn_kind,omitempty"`
	TriageState  string                     `json:"triage_state"`
	JobStatus    string                     `json:"job_status"`
	BriefKo      string                     `json:"brief_ko,omitempty"`
	DetailedKo   string                     `json:"detailed_ko,omitempty"`
	CreatedAt    time.Time                  `json:"created_at"`
	Items        []searchDetailItemResponse `json:"items"`
}

type searchDetailItemResponse struct {
	KnowledgeItemID string `json:"knowledge_item_id"`
	SurfaceText     string `json:"surface_text"`
	MeaningKo       string `json:"meaning_ko,omitempty"`
	DescriptionKo   string `json:"description_ko,omitempty"`
	PronunciationKo string `json:"pronunciation_ko,omitempty"`
	// char_start/char_end are rune offsets into selected_text, or -1 when the term was
	// not found verbatim in the sentence.
	CharStart int  `json:"char_start"`
	CharEnd   int  `json:"char_end"`
	Selected  bool `json:"selected"`
}

type searchSelectionResponse struct {
	CaptureID       string `json:"capture_id"`
	KnowledgeItemID string `json:"knowledge_item_id"`
	Selected        bool   `json:"selected"`
}
