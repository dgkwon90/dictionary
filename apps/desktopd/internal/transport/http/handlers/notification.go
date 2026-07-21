package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"neulsang/desktopd/internal/domain/notification"
)

type NotificationService interface {
	Pending(ctx context.Context) (notification.Pending, error)
	Recent(ctx context.Context, limit int) ([]notification.Notification, error)
	Ack(ctx context.Context, id string) error
	AckCapture(ctx context.Context, captureID string) error
}

type Notification struct {
	svc NotificationService
	log *slog.Logger
}

func NewNotification(svc NotificationService, log *slog.Logger) *Notification {
	return &Notification{svc: svc, log: log}
}

// List returns unacked notifications plus the badge count (ADR-0008). The Rust shell
// polls this, renders OS notifications + tray "New", then acks each.
func (h *Notification) List(w http.ResponseWriter, r *http.Request) {
	pending, err := h.svc.Pending(r.Context())
	if err != nil {
		h.log.Error("list notifications", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	items := make([]notificationResponse, 0, len(pending.Notifications))
	for _, n := range pending.Notifications {
		items = append(items, toNotificationResponse(n))
	}
	writeJSON(w, http.StatusOK, notificationListResponse{
		Notifications: items,
		UnackedCount:  pending.UnackedCount,
	})
}

// History returns recent notifications (acked included) newest-first for the in-app
// notification log (#24). Not gated by the enabled toggle. `unacked_count` counts the
// still-unacked rows in the returned window so the UI can hint at pending items.
func (h *Notification) History(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}

	list, err := h.svc.Recent(r.Context(), limit)
	if err != nil {
		h.log.Error("list notification history", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	items := make([]notificationResponse, 0, len(list))
	unacked := 0
	for _, n := range list {
		item := toNotificationResponse(n)
		if !item.Acked {
			unacked++
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, notificationListResponse{Notifications: items, UnackedCount: unacked})
}

func (h *Notification) Ack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.Ack(r.Context(), id); err != nil {
		if errors.Is(err, notification.ErrNotFound) {
			writeError(w, http.StatusNotFound, "notification not found")
			return
		}
		h.log.Error("ack notification", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "ok"})
}

// AckByCapture acks a capture's result_ready (best-effort) — called by the Quick Search
// popup once it has shown the result, so the poll loop skips a redundant notification.
func (h *Notification) AckByCapture(w http.ResponseWriter, r *http.Request) {
	captureID := r.PathValue("id")
	if err := h.svc.AckCapture(r.Context(), captureID); err != nil {
		h.log.Error("ack notification by capture", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "ok"})
}

func toNotificationResponse(n notification.Notification) notificationResponse {
	res := notificationResponse{
		ID:        n.ID,
		Kind:      n.Kind,
		Title:     n.Title,
		Body:      n.Body,
		Route:     n.Route,
		PayloadID: n.PayloadID,
		CreatedAt: n.CreatedAt.Format(time.RFC3339),
	}
	if !n.AckedAt.IsZero() {
		res.Acked = true
		res.AckedAt = n.AckedAt.Format(time.RFC3339)
	}
	return res
}

type notificationResponse struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Route     string `json:"route,omitempty"`
	PayloadID string `json:"payload_id,omitempty"`
	CreatedAt string `json:"created_at"`
	Acked     bool   `json:"acked"`
	AckedAt   string `json:"acked_at,omitempty"`
}

type notificationListResponse struct {
	Notifications []notificationResponse `json:"notifications"`
	UnackedCount  int                    `json:"unacked_count"`
}
