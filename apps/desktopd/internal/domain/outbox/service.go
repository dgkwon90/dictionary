package outbox

import (
	"context"
	"errors"
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

		switch err := s.publisher.Publish(ctx, events); {
		case err == nil:
			if err := s.repo.MarkAcked(ctx, eventIDs(events), time.Now().UTC()); err != nil {
				return acked, err
			}
			acked += len(events)
		case errors.Is(err, ErrPermanent):
			// The server refused the batch, but it does not say which event it
			// refused. Retrying the batch as a whole would loop forever, and
			// quarantining it as a whole would throw away every good event that
			// happened to travel with the bad one — so send them one at a time and
			// let the server point at the offender.
			sent, err := s.isolate(ctx, events)
			acked += sent
			if err != nil {
				return acked, err
			}
		default:
			return acked, err
		}

		if len(events) < s.batchLimit {
			return acked, nil
		}
	}
}

// isolate publishes events individually after a batch was permanently rejected.
//
// Each event then gets its own verdict: accepted (acked), permanently rejected
// (quarantined and skipped from now on), or transiently failed (left queued — the
// next tick tries again, including the ones after it).
func (s *Service) isolate(ctx context.Context, events []Event) (acked int, err error) {
	for _, event := range events {
		single := []Event{event}
		switch err := s.publisher.Publish(ctx, single); {
		case err == nil:
			if err := s.repo.MarkAcked(ctx, eventIDs(single), time.Now().UTC()); err != nil {
				return acked, err
			}
			acked++
		case errors.Is(err, ErrPermanent):
			s.log.Warn("sync event rejected permanently; quarantined",
				"event_id", event.EventID, "event_type", event.EventType, "error", err)
			if err := s.repo.MarkFailed(ctx, event.EventID, time.Now().UTC(), err.Error()); err != nil {
				return acked, err
			}
		default:
			return acked, err
		}
	}
	return acked, nil
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
	failed, err := s.repo.FailedCount(ctx)
	if err != nil {
		return Status{}, err
	}
	return Status{Enabled: s.publisher != nil, Pending: pending, Failed: failed}, nil
}

func eventIDs(events []Event) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.EventID)
	}
	return ids
}
