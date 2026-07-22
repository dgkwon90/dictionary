package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"neulsang/desktopd/internal/domain/outbox"
)

type SyncService interface {
	Status(ctx context.Context) (outbox.Status, error)
}

type Sync struct {
	svc SyncService
	log *slog.Logger
}

func NewSync(svc SyncService, log *slog.Logger) *Sync {
	return &Sync{svc: svc, log: log}
}

func (h *Sync) Status(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.Status(r.Context())
	if err != nil {
		h.log.Error("sync status", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, status)
}
