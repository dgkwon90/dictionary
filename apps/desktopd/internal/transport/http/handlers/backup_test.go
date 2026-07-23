package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/backup"
)

type fakeBackupService struct {
	exportSnapshot *backup.Snapshot
	exportErr      error
	importResult   *backup.ImportResult
	importErr      error
	backupResult   *backup.BackupResult
	backupErr      error
	importSnapshot *backup.Snapshot
	backupPath     string
}

func (f *fakeBackupService) Export(context.Context) (*backup.Snapshot, error) {
	return f.exportSnapshot, f.exportErr
}

func (f *fakeBackupService) Import(_ context.Context, snapshot *backup.Snapshot) (*backup.ImportResult, error) {
	f.importSnapshot = snapshot
	return f.importResult, f.importErr
}

func (f *fakeBackupService) BackupFile(_ context.Context, path string) (*backup.BackupResult, error) {
	f.backupPath = path
	return f.backupResult, f.backupErr
}

func TestBackupExportOK(t *testing.T) {
	exportedAt := time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC)
	svc := &fakeBackupService{exportSnapshot: &backup.Snapshot{
		Version:    1,
		ExportedAt: exportedAt,
		Captures:   []backup.CaptureRow{{ID: "cap-1", SelectedText: "hello", InputMode: "manual", TextHash: "hash", CreatedAt: exportedAt, InboxStatus: "new"}},
	}}
	handler := NewBackup(svc, slog.Default())
	recorder := httptest.NewRecorder()

	handler.Export(recorder, httptest.NewRequest(http.MethodGet, "/v1/export", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body backup.Snapshot
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Version != 1 || len(body.Captures) != 1 || body.Captures[0].ID != "cap-1" {
		t.Fatalf("body = %#v", body)
	}
}

func TestBackupImportOK(t *testing.T) {
	svc := &fakeBackupService{importResult: &backup.ImportResult{
		KnowledgeItems: backup.TableImportResult{Inserted: 1},
	}}
	handler := NewBackup(svc, slog.Default())
	recorder := httptest.NewRecorder()
	req := newJSONRequest(http.MethodPost, "/v1/import",
		strings.NewReader(`{"version":1,"exported_at":"2026-07-16T03:00:00Z","knowledge_items":[{"id":"ki-1"}]}`))

	handler.Import(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if svc.importSnapshot == nil || svc.importSnapshot.Version != 1 || len(svc.importSnapshot.KnowledgeItems) != 1 {
		t.Fatalf("service snapshot = %#v", svc.importSnapshot)
	}
	var body backup.ImportResult
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.KnowledgeItems.Inserted != 1 {
		t.Fatalf("result = %#v", body)
	}
}

func TestBackupImportBadJSONReturns400(t *testing.T) {
	svc := &fakeBackupService{}
	handler := NewBackup(svc, slog.Default())
	recorder := httptest.NewRecorder()

	handler.Import(recorder, newJSONRequest(http.MethodPost, "/v1/import", strings.NewReader(`{bad json`)))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if svc.importSnapshot != nil {
		t.Fatalf("service should not be called")
	}
}

func TestBackupFileOK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.db")
	svc := &fakeBackupService{backupResult: &backup.BackupResult{Path: path, SizeBytes: 1024}}
	handler := NewBackup(svc, slog.Default())
	recorder := httptest.NewRecorder()

	handler.BackupFile(recorder, newJSONRequest(http.MethodPost, "/v1/backup",
		strings.NewReader(`{"path":"`+path+`"}`)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if svc.backupPath != path {
		t.Fatalf("service path = %q, want %q", svc.backupPath, path)
	}
	var body backup.BackupResult
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Path != path || body.SizeBytes != 1024 {
		t.Fatalf("body = %#v", body)
	}
}

func TestBackupFileInvalidPathReturns400(t *testing.T) {
	svc := &fakeBackupService{backupErr: backup.ErrInvalidPath}
	handler := NewBackup(svc, slog.Default())
	recorder := httptest.NewRecorder()

	handler.BackupFile(recorder, newJSONRequest(http.MethodPost, "/v1/backup",
		strings.NewReader(`{"path":"relative.db"}`)))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestBackupFileBadJSONReturns400(t *testing.T) {
	svc := &fakeBackupService{}
	handler := NewBackup(svc, slog.Default())
	recorder := httptest.NewRecorder()

	handler.BackupFile(recorder, newJSONRequest(http.MethodPost, "/v1/backup", strings.NewReader(`{bad json`)))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if svc.backupPath != "" {
		t.Fatalf("service should not be called")
	}
}

func TestBackupInternalErrorsReturn500(t *testing.T) {
	svc := &fakeBackupService{exportErr: errors.New("database down")}
	handler := NewBackup(svc, slog.Default())
	recorder := httptest.NewRecorder()

	handler.Export(recorder, httptest.NewRequest(http.MethodGet, "/v1/export", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}
