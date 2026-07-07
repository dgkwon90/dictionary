package capture

import "time"

type Capture struct {
	ID           string    `json:"id"`
	SourceApp    string    `json:"source_app"`
	SourceType   string    `json:"source_type"`
	SourceTitle  string    `json:"source_title"`
	SourceURL    string    `json:"source_url"`
	SelectedText string    `json:"selected_text"`
	DetectedLang string    `json:"detected_lang"`
	InputMode    string    `json:"input_mode"`
	TextHash     string    `json:"text_hash"`
	InboxStatus  string    `json:"inbox_status"`
	CreatedAt    time.Time `json:"created_at"`
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
}

type CreateResult struct {
	CaptureID   string
	LookupJobID string
	Status      string
}
