package explain

import (
	"context"
	"fmt"
	"time"

	"neulsang/desktopd/internal/id"
)

const saveFailureTimeout = 5 * time.Second

type Repository interface {
	MarkRunning(ctx context.Context, jobID string, startedAt time.Time) error
	SaveSuccess(ctx context.Context, jobID, captureID string, result ExplainResult, rawResponseJSON string, finishedAt time.Time) error
	SaveFailure(ctx context.Context, jobID string, errMessage string, finishedAt time.Time) error
}

type Service struct {
	explainer Explainer
	repo      Repository
	now       func() time.Time
	newID     func() string
}

func NewService(explainer Explainer, repo Repository) *Service {
	return &Service{
		explainer: explainer,
		repo:      repo,
		now:       time.Now,
		newID:     id.New,
	}
}

func (s *Service) Process(ctx context.Context, jobID, captureID, text string) error {
	if err := s.repo.MarkRunning(ctx, jobID, s.now().UTC()); err != nil {
		return err
	}

	result, rawJSON, err := s.explainer.Explain(ctx, text)
	if err != nil {
		if saveErr := s.saveFailure(ctx, jobID, err.Error()); saveErr != nil {
			return fmt.Errorf("explain: %w; save failure: %v", err, saveErr)
		}
		return fmt.Errorf("explain: %w", err)
	}
	if err := result.Validate(); err != nil {
		if saveErr := s.saveFailure(ctx, jobID, err.Error()); saveErr != nil {
			return fmt.Errorf("validate explain result: %w; save failure: %v", err, saveErr)
		}
		return err
	}
	if err := s.repo.SaveSuccess(ctx, jobID, captureID, result, rawJSON, s.now().UTC()); err != nil {
		if saveErr := s.saveFailure(ctx, jobID, err.Error()); saveErr != nil {
			return fmt.Errorf("save explain result: %w; save failure: %v", err, saveErr)
		}
		return fmt.Errorf("save explain result: %w", err)
	}
	return nil
}

func (s *Service) saveFailure(ctx context.Context, jobID, errMessage string) error {
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), saveFailureTimeout)
	defer cancel()
	return s.repo.SaveFailure(saveCtx, jobID, errMessage, s.now().UTC())
}
