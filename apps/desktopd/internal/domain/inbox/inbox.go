package inbox

import (
	"context"
	"errors"
	"time"
)

type Item struct {
	CaptureID    string
	SelectedText string
	SourceApp    string
	SourceType   string
	InputMode    string
	CreatedAt    time.Time
	JobStatus    string
	BriefKo      string
	Status       string
}

var (
	ErrInvalidInput    = errors.New("invalid inbox input")
	ErrCaptureNotFound = errors.New("capture not found")
)

type ListInput struct {
	Status string
	Limit  int
}

type Repository interface {
	List(ctx context.Context, status string, limit int) ([]Item, error)
	SetStatus(ctx context.Context, captureID, status string) error
}
