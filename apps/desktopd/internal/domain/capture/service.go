package capture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"neulsang/desktopd/internal/id"
)

var ErrInvalidInput = errors.New("invalid capture input")

// Repository stores a newly-created capture, lookup job, and outbox event in one transaction.
type Repository interface {
	SaveNew(ctx context.Context, c Capture, j LookupJob, e OutboxEvent) error
}

type Service struct {
	repo  Repository
	now   func() time.Time
	newID func() string
}

func NewService(repo Repository) *Service {
	return &Service{
		repo:  repo,
		now:   time.Now,
		newID: id.New,
	}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (CreateResult, error) {
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return CreateResult{}, fmt.Errorf("%w: text is required", ErrInvalidInput)
	}
	if !validInputMode(input.InputMode) {
		return CreateResult{}, fmt.Errorf("%w: unsupported input_mode %q", ErrInvalidInput, input.InputMode)
	}
	if !validSourceType(input.SourceType) {
		return CreateResult{}, fmt.Errorf("%w: unsupported source_type %q", ErrInvalidInput, input.SourceType)
	}

	createdAt := s.now().UTC()
	sum := sha256.Sum256([]byte(text))
	c := Capture{
		ID:           s.newID(),
		SourceApp:    input.SourceApp,
		SourceType:   input.SourceType,
		SelectedText: text,
		InputMode:    input.InputMode,
		TextHash:     hex.EncodeToString(sum[:]),
		InboxStatus:  "new",
		CreatedAt:    createdAt,
	}
	j := LookupJob{
		ID:        s.newID(),
		CaptureID: c.ID,
		Status:    "queued",
		CreatedAt: createdAt,
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return CreateResult{}, fmt.Errorf("marshal capture outbox payload: %w", err)
	}
	e := OutboxEvent{
		EventID:       s.newID(),
		AggregateType: "capture",
		AggregateID:   c.ID,
		EventType:     "capture_created",
		PayloadJSON:   string(payload),
		CreatedAt:     createdAt,
	}
	if err := s.repo.SaveNew(ctx, c, j, e); err != nil {
		return CreateResult{}, fmt.Errorf("save capture: %w", err)
	}
	return CreateResult{CaptureID: c.ID, LookupJobID: j.ID, Status: j.Status}, nil
}

func validInputMode(value string) bool {
	switch value {
	case "clipboard", "manual", "pronunciation":
		return true
	default:
		return false
	}
}

func validSourceType(value string) bool {
	switch value {
	case "", "browser", "ide", "terminal", "document", "manual":
		return true
	default:
		return false
	}
}
