package backup

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidPath      = errors.New("invalid backup path")
	ErrSnapshotTooLarge = errors.New("import snapshot exceeds row limit")
)

// MaxSnapshotRowsPerTable bounds each table in an import snapshot (review R-01/R-08,
// RW-02: import decoded the whole body with no upper bound on row counts). This is
// deliberately generous — years of solo daily usage stay well under it — and exists
// as a backstop against a malformed or hostile snapshot forcing an unbounded DB
// transaction, not as a realistic usage ceiling.
const MaxSnapshotRowsPerTable = 500_000

// ValidateSnapshotSize rejects snapshots whose table row counts exceed
// MaxSnapshotRowsPerTable, before the caller starts an import transaction.
func ValidateSnapshotSize(s *Snapshot) error {
	tables := []struct {
		name string
		n    int
	}{
		{"knowledge_items", len(s.KnowledgeItems)},
		{"captures", len(s.Captures)},
		{"explanations", len(s.Explanations)},
		{"capture_items", len(s.CaptureItems)},
		{"learner_items", len(s.LearnerItems)},
		{"review_cards", len(s.ReviewCards)},
		{"review_logs", len(s.ReviewLogs)},
	}
	for _, table := range tables {
		if table.n > MaxSnapshotRowsPerTable {
			return fmt.Errorf("%w: %s has %d rows, max %d", ErrSnapshotTooLarge, table.name, table.n, MaxSnapshotRowsPerTable)
		}
	}
	return nil
}

type Snapshot struct {
	Version        int                `json:"version"`
	ExportedAt     time.Time          `json:"exported_at"`
	KnowledgeItems []KnowledgeItemRow `json:"knowledge_items"`
	Captures       []CaptureRow       `json:"captures"`
	Explanations   []ExplanationRow   `json:"explanations"`
	CaptureItems   []CaptureItemRow   `json:"capture_items"`
	LearnerItems   []LearnerItemRow   `json:"learner_items"`
	ReviewCards    []ReviewCardRow    `json:"review_cards"`
	ReviewLogs     []ReviewLogRow     `json:"review_logs"`
}

type CaptureRow struct {
	ID           string    `json:"id"`
	SourceApp    *string   `json:"source_app"`
	SourceType   *string   `json:"source_type"`
	SourceTitle  *string   `json:"source_title"`
	SourceURL    *string   `json:"source_url"`
	SelectedText string    `json:"selected_text"`
	DetectedLang *string   `json:"detected_lang"`
	InputMode    string    `json:"input_mode"`
	TextHash     string    `json:"text_hash"`
	CreatedAt    time.Time `json:"created_at"`
	InboxStatus  string    `json:"inbox_status"`
}

type ExplanationRow struct {
	ID                 string    `json:"id"`
	CaptureID          string    `json:"capture_id"`
	BriefKo            string    `json:"brief_ko"`
	DetailedKo         string    `json:"detailed_ko"`
	Pronunciation      *string   `json:"pronunciation"`
	ExamplesJSON       *string   `json:"examples_json"`
	TermsJSON          *string   `json:"terms_json"`
	DifficultyEstimate *float64  `json:"difficulty_estimate"`
	Category           *string   `json:"category"`
	RawResponseJSON    *string   `json:"raw_response_json"`
	CreatedAt          time.Time `json:"created_at"`
}

type KnowledgeItemRow struct {
	ID             string    `json:"id"`
	NormalizedKey  string    `json:"normalized_key"`
	SurfaceText    string    `json:"surface_text"`
	ItemType       string    `json:"item_type"`
	Language       string    `json:"language"`
	Pos            *string   `json:"pos"`
	Pronunciation  *string   `json:"pronunciation"`
	MeaningKo      *string   `json:"meaning_ko"`
	DescriptionKo  *string   `json:"description_ko"`
	DomainCategory *string   `json:"domain_category"`
	FirstSeenAt    time.Time `json:"first_seen_at"`
	LastSeenAt     time.Time `json:"last_seen_at"`
}

type CaptureItemRow struct {
	ID              string    `json:"id"`
	CaptureID       string    `json:"capture_id"`
	KnowledgeItemID string    `json:"knowledge_item_id"`
	Role            string    `json:"role"`
	Confidence      float64   `json:"confidence"`
	CreatedAt       time.Time `json:"created_at"`
}

type LearnerItemRow struct {
	ID               string     `json:"id"`
	KnowledgeItemID  string     `json:"knowledge_item_id"`
	FamiliarityScore float64    `json:"familiarity_score"`
	MasteryScore     float64    `json:"mastery_score"`
	AskCount         int64      `json:"ask_count"`
	WrongCount       int64      `json:"wrong_count"`
	ReviewCount      int64      `json:"review_count"`
	LastAskedAt      *time.Time `json:"last_asked_at"`
	LastWrongAt      *time.Time `json:"last_wrong_at"`
	LastReviewedAt   *time.Time `json:"last_reviewed_at"`
	Status           string     `json:"status"`
}

type ReviewCardRow struct {
	ID              string     `json:"id"`
	KnowledgeItemID string     `json:"knowledge_item_id"`
	CardType        string     `json:"card_type"`
	Question        string     `json:"question"`
	Answer          string     `json:"answer"`
	Explanation     *string    `json:"explanation"`
	State           string     `json:"state"`
	DueAt           *time.Time `json:"due_at"`
	Stability       float64    `json:"stability"`
	Difficulty      float64    `json:"difficulty"`
	Retrievability  *float64   `json:"retrievability"`
	Reps            int64      `json:"reps"`
	Lapses          int64      `json:"lapses"`
	LastReviewAt    *time.Time `json:"last_review_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ReviewLogRow struct {
	ID           string    `json:"id"`
	ReviewCardID string    `json:"review_card_id"`
	Source       string    `json:"source"`
	Rating       string    `json:"rating"`
	ElapsedMs    *int64    `json:"elapsed_ms"`
	ReviewedAt   time.Time `json:"reviewed_at"`
}

type ImportResult struct {
	KnowledgeItems TableImportResult `json:"knowledge_items"`
	Captures       TableImportResult `json:"captures"`
	Explanations   TableImportResult `json:"explanations"`
	CaptureItems   TableImportResult `json:"capture_items"`
	LearnerItems   TableImportResult `json:"learner_items"`
	ReviewCards    TableImportResult `json:"review_cards"`
	ReviewLogs     TableImportResult `json:"review_logs"`
}

type TableImportResult struct {
	Inserted int `json:"inserted"`
	Merged   int `json:"merged"`
	Updated  int `json:"updated"`
	Skipped  int `json:"skipped"`
}

type BackupResult struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

type Repository interface {
	Export(ctx context.Context) (*Snapshot, error)
	Import(ctx context.Context, snapshot *Snapshot) (*ImportResult, error)
	BackupFile(ctx context.Context, path string) (*BackupResult, error)
}
