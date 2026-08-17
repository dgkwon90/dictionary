package backup

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidPath = errors.New("invalid backup path")
	// Reaches the user on export as well as import, so it does not say "import".
	ErrSnapshotTooLarge = errors.New("snapshot exceeds row limit")
)

// MaxSnapshotRowsPerTable bounds each table in an import snapshot (review R-01/R-08,
// RW-02: import decoded the whole body with no upper bound on row counts). This is
// deliberately generous — years of solo daily usage stay well under it — and exists
// as a backstop against a malformed or hostile snapshot forcing an unbounded DB
// transaction, not as a realistic usage ceiling.
const MaxSnapshotRowsPerTable = 500_000

// ValidateSnapshotSize rejects snapshots whose table row counts exceed
// MaxSnapshotRowsPerTable, before the caller starts an import transaction — and
// on the way out too, so the app cannot write a backup file it would then refuse
// to read (Service.Export).
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
		// These two were added to the snapshot later and were left out of the size
		// check, so a hand-edited file could push unbounded rows through them.
		{"lookup_jobs", len(s.LookupJobs)},
		{"review_card_candidates", len(s.ReviewCardCandidates)},
		{"app_settings", len(s.AppSettings)},
	}
	for _, table := range tables {
		if table.n > MaxSnapshotRowsPerTable {
			return fmt.Errorf("%w: %s has %d rows, max %d", ErrSnapshotTooLarge, table.name, table.n, MaxSnapshotRowsPerTable)
		}
	}
	return nil
}

// ErrUnsupportedSnapshotVersion signals a snapshot whose version this build
// cannot import — either older than we still know how to read, or newer than
// this build understands (review R-02, RW-04).
var ErrUnsupportedSnapshotVersion = errors.New("unsupported snapshot version")

const (
	// MinSnapshotVersion is the oldest version Import still accepts.
	//
	// v1 and v2 are refused rather than migrated. They describe the pre-redesign
	// model — captures sorted into saved/archived, no word/sentence distinction, no
	// accuracy, and no way to express a sentence as a learning item. There is no
	// mapping from those rows to the current one that preserves meaning: a "saved"
	// capture says nothing about whether the user decided to learn it, so importing
	// one would have to invent a decision the user never made.
	MinSnapshotVersion = 3
	// CurrentSnapshotVersion is what Export produces.
	CurrentSnapshotVersion = 3
)

// ValidateSnapshotVersion rejects a snapshot version this build cannot import.
func ValidateSnapshotVersion(version int) error {
	if version < MinSnapshotVersion || version > CurrentSnapshotVersion {
		return fmt.Errorf("%w: %d (supported: %d-%d)", ErrUnsupportedSnapshotVersion, version, MinSnapshotVersion, CurrentSnapshotVersion)
	}
	return nil
}

type Snapshot struct {
	Version              int                      `json:"version"`
	ExportedAt           time.Time                `json:"exported_at"`
	KnowledgeItems       []KnowledgeItemRow       `json:"knowledge_items"`
	Captures             []CaptureRow             `json:"captures"`
	Explanations         []ExplanationRow         `json:"explanations"`
	CaptureItems         []CaptureItemRow         `json:"capture_items"`
	LearnerItems         []LearnerItemRow         `json:"learner_items"`
	ReviewCards          []ReviewCardRow          `json:"review_cards"`
	ReviewLogs           []ReviewLogRow           `json:"review_logs"`
	LookupJobs           []LookupJobRow           `json:"lookup_jobs"`
	ReviewCardCandidates []ReviewCardCandidateRow `json:"review_card_candidates"`
	AppSettings          []AppSettingRow          `json:"app_settings"`
}

// AppSettingRow carries the user's preferences — review schedule, AI style, reminder
// times. They were left out of the snapshot until now, so a restore rebuilt every card
// the user had earned and then asked them in a rhythm they never chose.
//
// Secrets are not here to be left out: the API key lives in the environment or the OS
// keychain and never reaches this table (ADR-0004 부록), which is exactly why the whole
// table can be exported without filtering.
type AppSettingRow struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CaptureRow struct {
	ID              string    `json:"id"`
	ParentCaptureID *string   `json:"parent_capture_id"`
	SourceApp       *string   `json:"source_app"`
	SourceType      *string   `json:"source_type"`
	SelectedText    string    `json:"selected_text"`
	DetectedLang    *string   `json:"detected_lang"`
	InputMode       string    `json:"input_mode"`
	TextHash        string    `json:"text_hash"`
	InputType       *string   `json:"input_type"`
	LearnKind       *string   `json:"learn_kind"`
	TriageState     string    `json:"triage_state"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
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
	LearnKind      string    `json:"learn_kind"`
	ItemType       *string   `json:"item_type"`
	Language       string    `json:"language"`
	Pronunciation  *string   `json:"pronunciation"`
	MeaningKo      *string   `json:"meaning_ko"`
	DescriptionKo  *string   `json:"description_ko"`
	DomainCategory *string   `json:"domain_category"`
	FirstSeenAt    time.Time `json:"first_seen_at"`
	LastSeenAt     time.Time `json:"last_seen_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CaptureItemRow struct {
	ID              string     `json:"id"`
	CaptureID       string     `json:"capture_id"`
	KnowledgeItemID string     `json:"knowledge_item_id"`
	Role            string     `json:"role"`
	Confidence      float64    `json:"confidence"`
	CharStart       *int64     `json:"char_start"`
	CharEnd         *int64     `json:"char_end"`
	SelectedAt      *time.Time `json:"selected_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type LearnerItemRow struct {
	ID              string     `json:"id"`
	KnowledgeItemID string     `json:"knowledge_item_id"`
	AskCount        int64      `json:"ask_count"`
	UnknownCount    int64      `json:"unknown_count"`
	AttemptCount    int64      `json:"attempt_count"`
	CorrectCount    int64      `json:"correct_count"`
	RegisteredAt    time.Time  `json:"registered_at"`
	LastAskedAt     *time.Time `json:"last_asked_at"`
	LastUnknownAt   *time.Time `json:"last_unknown_at"`
	LastGradedAt    *time.Time `json:"last_graded_at"`
	Status          string     `json:"status"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ReviewCardRow struct {
	ID                     string     `json:"id"`
	KnowledgeItemID        string     `json:"knowledge_item_id"`
	ContextKnowledgeItemID *string    `json:"context_knowledge_item_id"`
	CardType               string     `json:"card_type"`
	Question               string     `json:"question"`
	Answer                 string     `json:"answer"`
	Explanation            *string    `json:"explanation"`
	State                  string     `json:"state"`
	DueAt                  *time.Time `json:"due_at"`
	IntervalDays           float64    `json:"interval_days"`
	Reps                   int64      `json:"reps"`
	Lapses                 int64      `json:"lapses"`
	LastReviewAt           *time.Time `json:"last_review_at"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type ReviewLogRow struct {
	ID           string    `json:"id"`
	ReviewCardID string    `json:"review_card_id"`
	Source       string    `json:"source"`
	Rating       string    `json:"rating"`
	IsCorrect    bool      `json:"is_correct"`
	ElapsedMs    *int64    `json:"elapsed_ms"`
	ReviewedAt   time.Time `json:"reviewed_at"`
}

// validLookupJobStatuses are the only status values explain.Service and
// GetSnapshot understand (internal/domain/explain). An import validates
// against this set so a corrupt/hand-edited snapshot can't silently produce a
// lookup_job whose status the rest of the app doesn't recognize.
var validLookupJobStatuses = map[string]bool{
	"queued":  true,
	"running": true,
	"done":    true,
	"failed":  true,
}

// LookupJobRow restores the AI-processing status a capture's explanation
// needs (RW-04/review R-02): without it, GetSnapshot finds no lookup_jobs row
// for a restored capture_id and reports the capture as not found at all, even
// though its explanation row is sitting right there.
type LookupJobRow struct {
	ID            string     `json:"id"`
	CaptureID     string     `json:"capture_id"`
	Status        string     `json:"status"`
	Provider      *string    `json:"provider"`
	Model         *string    `json:"model"`
	PromptVersion *string    `json:"prompt_version"`
	ErrorMessage  *string    `json:"error_message"`
	StartedAt     *time.Time `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ReviewCardCandidateRow restores the not-yet-consumed review card candidates
// a capture's explanation produced (RW-04/review R-02): without it, a restored
// knowledge item that hadn't been marked "unknown" yet before the backup loses
// its ability to ever become a review card.
type ReviewCardCandidateRow struct {
	ID              string  `json:"id"`
	CaptureID       string  `json:"capture_id"`
	KnowledgeItemID *string `json:"knowledge_item_id"`
	// ContextKnowledgeItemID carries the sentence a cloze candidate came from, so a
	// restored candidate still produces a card tied to the right context.
	ContextKnowledgeItemID *string    `json:"context_knowledge_item_id"`
	CardType               string     `json:"card_type"`
	Question               string     `json:"question"`
	Answer                 string     `json:"answer"`
	Explanation            *string    `json:"explanation"`
	CreatedAt              time.Time  `json:"created_at"`
	ConsumedAt             *time.Time `json:"consumed_at"`
}

// ErrInvalidLookupJobStatus signals a lookup_jobs row whose status isn't one
// explain.Service/GetSnapshot understands (queued/running/done/failed) — e.g.
// a hand-edited or corrupted snapshot file.
var ErrInvalidLookupJobStatus = errors.New("invalid lookup_job status")

// ValidateLookupJobs rejects a snapshot whose lookup_jobs contain a status
// value the rest of the app doesn't recognize, before an import transaction
// starts (RW-04).
func ValidateLookupJobs(jobs []LookupJobRow) error {
	for _, job := range jobs {
		if !validLookupJobStatuses[job.Status] {
			return fmt.Errorf("%w: job %q has status %q", ErrInvalidLookupJobStatus, job.ID, job.Status)
		}
	}
	return nil
}

type ImportResult struct {
	KnowledgeItems       TableImportResult `json:"knowledge_items"`
	Captures             TableImportResult `json:"captures"`
	Explanations         TableImportResult `json:"explanations"`
	CaptureItems         TableImportResult `json:"capture_items"`
	LearnerItems         TableImportResult `json:"learner_items"`
	ReviewCards          TableImportResult `json:"review_cards"`
	ReviewLogs           TableImportResult `json:"review_logs"`
	LookupJobs           TableImportResult `json:"lookup_jobs"`
	ReviewCardCandidates TableImportResult `json:"review_card_candidates"`
	AppSettings          TableImportResult `json:"app_settings"`
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
