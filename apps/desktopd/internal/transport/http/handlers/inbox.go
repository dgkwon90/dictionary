package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"neulsang/desktopd/internal/domain/inbox"
)

type InboxService interface {
	List(ctx context.Context, input inbox.ListInput) ([]inbox.Item, error)
	SetStatus(ctx context.Context, captureID, status string) error
}

type Inbox struct {
	svc InboxService
	log *slog.Logger
}

func NewInbox(svc InboxService, log *slog.Logger) *Inbox {
	return &Inbox{svc: svc, log: log}
}

func (h *Inbox) List(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsedLimit
	}

	items, err := h.svc.List(r.Context(), inbox.ListInput{
		Status: r.URL.Query().Get("status"),
		Limit:  limit,
	})
	if err != nil {
		if errors.Is(err, inbox.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error("list inbox", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	responseItems := make([]inboxItemResponse, 0, len(items))
	for _, item := range items {
		responseItems = append(responseItems, inboxItemResponse{
			CaptureID:    item.CaptureID,
			SelectedText: item.SelectedText,
			SourceApp:    item.SourceApp,
			SourceType:   item.SourceType,
			InputMode:    item.InputMode,
			Status:       item.Status,
			JobStatus:    item.JobStatus,
			BriefKo:      item.BriefKo,
			CreatedAt:    item.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, inboxListResponse{Items: responseItems})
}

func (h *Inbox) Save(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, "saved")
}

func (h *Inbox) Archive(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, "archived")
}

func (h *Inbox) setStatus(w http.ResponseWriter, r *http.Request, status string) {
	captureID := r.PathValue("id")
	if err := h.svc.SetStatus(r.Context(), captureID, status); err != nil {
		switch {
		case errors.Is(err, inbox.ErrCaptureNotFound):
			writeError(w, http.StatusNotFound, "capture not found")
		case errors.Is(err, inbox.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.log.Error("set inbox status", "capture_id", captureID, "status", status, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, inboxStatusResponse{CaptureID: captureID, Status: status})
}

type inboxListResponse struct {
	Items []inboxItemResponse `json:"items"`
}

type inboxItemResponse struct {
	CaptureID    string    `json:"capture_id"`
	SelectedText string    `json:"selected_text"`
	SourceApp    string    `json:"source_app,omitempty"`
	SourceType   string    `json:"source_type,omitempty"`
	InputMode    string    `json:"input_mode"`
	Status       string    `json:"status"`
	JobStatus    string    `json:"job_status"`
	BriefKo      string    `json:"brief_ko,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type inboxStatusResponse struct {
	CaptureID string `json:"capture_id"`
	Status    string `json:"status"`
}
