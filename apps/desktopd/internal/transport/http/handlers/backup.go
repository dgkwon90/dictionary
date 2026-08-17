package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"neulsang/desktopd/internal/domain/backup"
)

// maxImportBodyBytes is the actual memory bound for an import request (review
// R-01/R-08, RW-02) — it caps bytes read off the wire before decoding starts.
// backup.ValidateSnapshotSize runs afterward, on the already-decoded snapshot; it
// does not add a further memory bound (decoding happens either way once the byte
// cap is satisfied) but does stop a pathologically high row count — one that a
// compact-but-huge snapshot could pack within maxImportBodyBytes — from ever
// reaching an import DB transaction (codex review, RW-02).
//
// Where 200MiB comes from, since the number was previously unexplained: a capture
// costs roughly 2.5KiB of compact JSON, most of it the explanation
// (explanations.raw_response_json measured ~1.2KiB average, plus brief/detailed
// text, the capture row, and its knowledge_items/capture_items). That puts the cap
// somewhere near 80,000 captures — well past a decade of daily use — so it bounds
// memory without a real user ever meeting it. The estimate comes from a small
// sample and is meant as an order of magnitude, not a measurement.
//
// Note which limit binds first: at that size the per-table cap of
// backup.MaxSnapshotRowsPerTable (500,000) is unreachable for the text-heavy
// tables, since 500,000 explanations would be over a gigabyte. Bytes stop the
// realistic snapshot; the row cap exists for the compact-but-huge shape that slips
// under the byte cap. They are not redundant, and neither is the tighter one in
// general.
const maxImportBodyBytes = 200 << 20 // 200MiB

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
		if errors.Is(err, backup.ErrSnapshotTooLarge) {
			// Not a generic failure: the export stopped because the result would be
			// a file this build refuses to import. Say so, or the user retries a
			// button that cannot start working.
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Backup) Import(w http.ResponseWriter, r *http.Request) {
	var snapshot backup.Snapshot
	if err := decodeJSONBody(w, r, &snapshot, maxImportBodyBytes, h.log); err != nil {
		writeJSONDecodeError(w, err)
		return
	}
	if err := backup.ValidateSnapshotSize(&snapshot); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.Import(r.Context(), &snapshot)
	if err != nil {
		switch {
		case errors.Is(err, backup.ErrUnsupportedSnapshotVersion), errors.Is(err, backup.ErrInvalidLookupJobStatus):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.log.Error("import backup snapshot", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Backup) BackupFile(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Path string `json:"path"`
	}
	if err := decodeJSONBody(w, r, &request, 1<<20, h.log); err != nil {
		writeJSONDecodeError(w, err)
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
