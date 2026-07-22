package backup

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

type fakeRepository struct {
	exportSnapshot *Snapshot
	backupPath     string
}

func (f *fakeRepository) Export(context.Context) (*Snapshot, error) {
	return f.exportSnapshot, nil
}

func (f *fakeRepository) Import(context.Context, *Snapshot) (*ImportResult, error) {
	return &ImportResult{}, nil
}

func (f *fakeRepository) BackupFile(_ context.Context, path string) (*BackupResult, error) {
	f.backupPath = path
	return &BackupResult{Path: path, SizeBytes: 42}, nil
}

func TestServiceExportStampsVersionAndExportedAt(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 30, 0, 0, time.FixedZone("KST", 9*60*60))
	repo := &fakeRepository{exportSnapshot: &Snapshot{
		KnowledgeItems: []KnowledgeItemRow{{ID: "ki-1"}},
	}}
	svc := &Service{repo: repo, now: func() time.Time { return now }}

	snapshot, err := svc.Export(context.Background())
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if snapshot.Version != 1 {
		t.Fatalf("Version = %d, want 1", snapshot.Version)
	}
	if got, want := snapshot.ExportedAt, now.UTC(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("ExportedAt = %v (%v), want %v UTC", got, got.Location(), want)
	}
	if len(snapshot.KnowledgeItems) != 1 {
		t.Fatalf("KnowledgeItems = %d, want preserved rows", len(snapshot.KnowledgeItems))
	}
}

func TestServiceBackupFileValidatesPath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "empty", path: ""},
		{name: "relative", path: "backup.db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{}
			svc := NewService(repo)

			_, err := svc.BackupFile(context.Background(), tt.path)
			if !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("BackupFile() error = %v, want ErrInvalidPath", err)
			}
			if repo.backupPath != "" {
				t.Fatalf("repository called with %q", repo.backupPath)
			}
		})
	}
}

func TestServiceBackupFileDelegatesAbsolutePath(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)
	path := filepath.Join(t.TempDir(), "backup.db")

	result, err := svc.BackupFile(context.Background(), path)
	if err != nil {
		t.Fatalf("BackupFile() error = %v", err)
	}
	if repo.backupPath != path {
		t.Fatalf("repository path = %q, want %q", repo.backupPath, path)
	}
	if result.Path != path || result.SizeBytes != 42 {
		t.Fatalf("BackupFile() = %#v", result)
	}
}

func TestValidateSnapshotSizeWithinLimit(t *testing.T) {
	snapshot := &Snapshot{
		KnowledgeItems: make([]KnowledgeItemRow, 3),
		Captures:       make([]CaptureRow, 3),
	}
	if err := ValidateSnapshotSize(snapshot); err != nil {
		t.Fatalf("ValidateSnapshotSize() error = %v, want nil", err)
	}
}

func TestValidateSnapshotSizeRejectsOversizedTable(t *testing.T) {
	tests := []struct {
		name     string
		snapshot *Snapshot
	}{
		{name: "knowledge_items", snapshot: &Snapshot{KnowledgeItems: make([]KnowledgeItemRow, MaxSnapshotRowsPerTable+1)}},
		{name: "captures", snapshot: &Snapshot{Captures: make([]CaptureRow, MaxSnapshotRowsPerTable+1)}},
		{name: "explanations", snapshot: &Snapshot{Explanations: make([]ExplanationRow, MaxSnapshotRowsPerTable+1)}},
		{name: "capture_items", snapshot: &Snapshot{CaptureItems: make([]CaptureItemRow, MaxSnapshotRowsPerTable+1)}},
		{name: "learner_items", snapshot: &Snapshot{LearnerItems: make([]LearnerItemRow, MaxSnapshotRowsPerTable+1)}},
		{name: "review_cards", snapshot: &Snapshot{ReviewCards: make([]ReviewCardRow, MaxSnapshotRowsPerTable+1)}},
		{name: "review_logs", snapshot: &Snapshot{ReviewLogs: make([]ReviewLogRow, MaxSnapshotRowsPerTable+1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSnapshotSize(tt.snapshot)
			if !errors.Is(err, ErrSnapshotTooLarge) {
				t.Fatalf("ValidateSnapshotSize() error = %v, want ErrSnapshotTooLarge", err)
			}
		})
	}
}
