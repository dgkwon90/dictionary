package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"neulsang/desktopd/internal/domain/backup"
)

type BackupService interface {
	Export(ctx context.Context) (*backup.Snapshot, error)
	Import(ctx context.Context, snapshot *backup.Snapshot) (*backup.ImportResult, error)
	BackupFile(ctx context.Context, path string) (*backup.BackupResult, error)
}

type Backup struct {
	svc BackupService
	log *slog.Logger
}

func NewBackup(svc BackupService, log *slog.Logger) *Backup {
	return &Backup{svc: svc, log: log}
}

func (h *Backup) Export(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.svc.Export(r.Context())
	if err != nil {
		h.log.Error("export backup snapshot", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Backup) Import(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := r.Body.Close(); err != nil {
			h.log.Error("close backup import request body", "error", err)
		}
	}()

	var snapshot backup.Snapshot
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.Import(r.Context(), &snapshot)
	if err != nil {
		h.log.Error("import backup snapshot", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Backup) BackupFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer func() {
		if err := r.Body.Close(); err != nil {
			h.log.Error("close backup file request body", "error", err)
		}
	}()

	var request struct {
		Path string `json:"path"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.BackupFile(r.Context(), request.Path)
	if err != nil {
		if errors.Is(err, backup.ErrInvalidPath) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error("write backup file", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
