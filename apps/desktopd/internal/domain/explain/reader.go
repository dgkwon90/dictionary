package explain

import (
	"context"
	"errors"
)

var ErrCaptureNotFound = errors.New("capture not found")

type Snapshot struct {
	Status       string
	ErrorMessage string
	// LearnKind is the server's word/sentence call for this capture (empty until the
	// lookup finishes). It belongs to the snapshot because the caller that polls for a
	// result is the one that has to offer "학습할래요" or "모르는 단어 고르기" the moment
	// it arrives — making it fetch the capture separately would be a second round trip
	// for one word.
	LearnKind string
	Result    *ExplainResult
}

type Reader interface {
	GetSnapshot(ctx context.Context, captureID string) (Snapshot, error)
}
