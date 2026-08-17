package backup

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) Export(ctx context.Context) (*Snapshot, error) {
	snapshot, err := s.repo.Export(ctx)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		snapshot = &Snapshot{}
	}
	snapshot.Version = CurrentSnapshotVersion
	snapshot.ExportedAt = s.now().UTC()
	// Export is held to the same row limit Import enforces. Without this the app
	// would happily write a backup file it then refuses to read — the worst kind of
	// backup, since the user only discovers it on the day they need it. Failing here
	// is louder and earlier: it says the export did not happen, at a moment when the
	// data is still there.
	if err := ValidateSnapshotSize(snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *Service) Import(ctx context.Context, snapshot *Snapshot) (*ImportResult, error) {
	if err := ValidateSnapshotVersion(snapshot.Version); err != nil {
		return nil, err
	}
	if err := ValidateLookupJobs(snapshot.LookupJobs); err != nil {
		return nil, err
	}
	return s.repo.Import(ctx, snapshot)
}

func (s *Service) BackupFile(ctx context.Context, path string) (*BackupResult, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%w: path must be a non-empty absolute path", ErrInvalidPath)
	}
	// Clean collapses any "..", so what reaches the repository is the single
	// normalized form of the destination rather than one of the many spellings that
	// resolve to it.
	return s.repo.BackupFile(ctx, filepath.Clean(path))
}
