package outbox

import (
	"context"
	"log/slog"
	"time"
)

const (
	defaultBatchLimit    = 100
	defaultFlushInterval = 15 * time.Second
)

type Service struct {
	repo       Repository
	publisher  Publisher
	log        *slog.Logger
	batchLimit int
	interval   time.Duration
}

func NewService(repo Repository, publisher Publisher, log *slog.Logger) *Service {
	return &Service{
		repo:       repo,
		publisher:  publisher,
		log:        log,
		batchLimit: defaultBatchLimit,
		interval:   defaultFlushInterval,
	}
}

func (s *Service) Flush(ctx context.Context) (acked int, err error) {
	if s.publisher == nil {
		return 0, nil
	}
	for {
		events, err := s.repo.ListUnsent(ctx, s.batchLimit)
		if err != nil {
			return acked, err
		}
		if len(events) == 0 {
			return acked, nil
		}
		// A permanent non-2xx will retry forever in this skeleton; 4xx quarantine is
		// a future refinement.
		if err := s.publisher.Publish(ctx, events); err != nil {
			return acked, err
		}
		if err := s.repo.MarkAcked(ctx, eventIDs(events), time.Now().UTC()); err != nil {
			return acked, err
		}
		acked += len(events)
		if len(events) < s.batchLimit {
			return acked, nil
		}
	}
}

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	if _, err := s.Flush(ctx); err != nil {
		s.log.Debug("sync flush failed", "error", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.Flush(ctx); err != nil {
				s.log.Debug("sync flush failed", "error", err)
			}
		}
	}
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	pending, err := s.repo.PendingCount(ctx)
	if err != nil {
		return Status{}, err
	}
	return Status{Enabled: s.publisher != nil, Pending: pending}, nil
}

func eventIDs(events []Event) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.EventID)
	}
	return ids
}
