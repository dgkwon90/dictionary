package explain

import (
	"context"
	"errors"
)

var ErrCaptureNotFound = errors.New("capture not found")

type Snapshot struct {
	Status       string
	ErrorMessage string
	Result       *ExplainResult
}

type Reader interface {
	GetSnapshot(ctx context.Context, captureID string) (Snapshot, error)
}
