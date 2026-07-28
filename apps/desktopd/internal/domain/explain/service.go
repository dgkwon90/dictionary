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
	format    FormatSource
	now       func() time.Time
	newID     func() string
}

// NewService wires the lookup use case. A nil format source means "always the default
// format", which is what tests and any caller with no settings storage want.
func NewService(explainer Explainer, repo Repository, format FormatSource) *Service {
	if format == nil {
		format = FixedFormat(DefaultFormat())
	}
	return &Service{
		explainer: explainer,
		repo:      repo,
		format:    format,
		now:       time.Now,
		newID:     id.New,
	}
}

func (s *Service) Process(ctx context.Context, jobID, captureID, text string) error {
	if err := s.repo.MarkRunning(ctx, jobID, s.now().UTC()); err != nil {
		return err
	}

	// Read the user's saved formatting for this lookup. A failure here is recorded like
	// any other lookup failure rather than swallowed: the settings row lives in the same
	// local database this job is about to write its result to, so if it cannot be read
	// the save was not going to succeed either, and a capture stuck at "running" tells
	// the user less than one that says it failed.
	format, err := s.format.ExplainFormat(ctx)
	if err != nil {
		err = fmt.Errorf("load explanation format: %w", err)
		if saveErr := s.saveFailure(ctx, jobID, err.Error()); saveErr != nil {
			return fmt.Errorf("%w; save failure: %v", err, saveErr)
		}
		return err
	}

	result, rawJSON, err := s.explainer.Explain(ctx, text, format)
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
