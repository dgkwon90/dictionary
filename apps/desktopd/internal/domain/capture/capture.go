package capture

import "time"

type Capture struct {
	ID string `json:"id"`
	// ParentCaptureID is set when this capture exists only to explain a span the
	// user picked inside another capture's sentence.
	ParentCaptureID string `json:"parent_capture_id,omitempty"`
	SourceApp       string `json:"source_app"`
	SourceType      string `json:"source_type"`
	SelectedText    string `json:"selected_text"`
	DetectedLang    string `json:"detected_lang"`
	InputMode       string `json:"input_mode"`
	TextHash        string `json:"text_hash"`
	// InputType and LearnKind stay empty until the AI result lands; the explain
	// repository fills them in the same transaction that stores the explanation.
	InputType   string    `json:"input_type,omitempty"`
	LearnKind   string    `json:"learn_kind,omitempty"`
	TriageState string    `json:"triage_state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type LookupJob struct {
	ID        string    `json:"id"`
	CaptureID string    `json:"capture_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type OutboxEvent struct {
	EventID       string    `json:"event_id"`
	AggregateType string    `json:"aggregate_type"`
	AggregateID   string    `json:"aggregate_id"`
	EventType     string    `json:"event_type"`
	PayloadJSON   string    `json:"payload_json"`
	CreatedAt     time.Time `json:"created_at"`
}

type CreateInput struct {
	Text       string
	InputMode  string
	SourceApp  string
	SourceType string
	// ParentCaptureID is set only by the sentence word-selection flow, never by the
	// public capture endpoint.
	ParentCaptureID string
}

type CreateResult struct {
	CaptureID   string
	LookupJobID string
	Status      string
}
