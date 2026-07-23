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
	return s.repo.BackupFile(ctx, path)
}
