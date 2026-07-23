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
	importCalled   bool
}

func (f *fakeRepository) Export(context.Context) (*Snapshot, error) {
	return f.exportSnapshot, nil
}

func (f *fakeRepository) Import(context.Context, *Snapshot) (*ImportResult, error) {
	f.importCalled = true
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
	if snapshot.Version != CurrentSnapshotVersion {
		t.Fatalf("Version = %d, want %d", snapshot.Version, CurrentSnapshotVersion)
	}
	if got, want := snapshot.ExportedAt, now.UTC(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("ExportedAt = %v (%v), want %v UTC", got, got.Location(), want)
	}
	if len(snapshot.KnowledgeItems) != 1 {
		t.Fatalf("KnowledgeItems = %d, want preserved rows", len(snapshot.KnowledgeItems))
	}
}

func TestValidateSnapshotVersion(t *testing.T) {
	tests := []struct {
		version int
		wantErr bool
	}{
		{version: 0, wantErr: true},
		{version: MinSnapshotVersion, wantErr: false},
		{version: CurrentSnapshotVersion, wantErr: false},
		{version: CurrentSnapshotVersion + 1, wantErr: true},
	}
	for _, tt := range tests {
		err := ValidateSnapshotVersion(tt.version)
		if tt.wantErr && !errors.Is(err, ErrUnsupportedSnapshotVersion) {
			t.Errorf("ValidateSnapshotVersion(%d) error = %v, want ErrUnsupportedSnapshotVersion", tt.version, err)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("ValidateSnapshotVersion(%d) error = %v, want nil", tt.version, err)
		}
	}
}

func TestValidateLookupJobs(t *testing.T) {
	valid := []LookupJobRow{{ID: "j1", Status: "queued"}, {ID: "j2", Status: "running"}, {ID: "j3", Status: "done"}, {ID: "j4", Status: "failed"}}
	if err := ValidateLookupJobs(valid); err != nil {
		t.Errorf("ValidateLookupJobs(valid) error = %v, want nil", err)
	}

	invalid := []LookupJobRow{{ID: "j1", Status: "queued"}, {ID: "j-bad", Status: "in_progress"}}
	err := ValidateLookupJobs(invalid)
	if !errors.Is(err, ErrInvalidLookupJobStatus) {
		t.Fatalf("ValidateLookupJobs(invalid) error = %v, want ErrInvalidLookupJobStatus", err)
	}
}

func TestServiceImportRejectsUnsupportedVersionWithoutCallingRepository(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)

	_, err := svc.Import(context.Background(), &Snapshot{Version: CurrentSnapshotVersion + 1})
	if !errors.Is(err, ErrUnsupportedSnapshotVersion) {
		t.Fatalf("Import() error = %v, want ErrUnsupportedSnapshotVersion", err)
	}
	if repo.importCalled {
		t.Fatal("repository.Import() was called despite the version rejection — import must be all-or-nothing")
	}
}

func TestServiceImportRejectsInvalidLookupJobStatusWithoutCallingRepository(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)

	_, err := svc.Import(context.Background(), &Snapshot{
		Version:    CurrentSnapshotVersion,
		LookupJobs: []LookupJobRow{{ID: "job-1", Status: "bogus"}},
	})
	if !errors.Is(err, ErrInvalidLookupJobStatus) {
		t.Fatalf("Import() error = %v, want ErrInvalidLookupJobStatus", err)
	}
	if repo.importCalled {
		t.Fatal("repository.Import() was called despite the invalid status")
	}
}

func TestServiceImportDelegatesValidSnapshot(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)

	if _, err := svc.Import(context.Background(), &Snapshot{Version: CurrentSnapshotVersion}); err != nil {
		t.Fatalf("Import() error = %v, want nil", err)
	}
	if !repo.importCalled {
		t.Fatal("repository.Import() was not called for a valid snapshot")
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
