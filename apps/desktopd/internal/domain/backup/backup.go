package backup

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidPath = errors.New("invalid backup path")

// ErrUnsupportedSnapshotVersion signals a snapshot whose version this build
// cannot import — either older than we still know how to read, or newer than
// this build understands (review R-02, RW-04).
var ErrUnsupportedSnapshotVersion = errors.New("unsupported snapshot version")

const (
	// MinSnapshotVersion is the oldest version Import still accepts. v1 (the
	// original 7-table snapshot, no lookup_jobs/review_card_candidates) reads
	// fine as-is: those two fields are simply absent/empty in v1 JSON, and the
	// importers for them just iterate zero rows — no special-casing needed.
	MinSnapshotVersion = 1
	// CurrentSnapshotVersion is what Export produces: v1's 7 tables plus
	// lookup_jobs and review_card_candidates (RW-04), so a restored capture's
	// explanation is reachable again and unconsumed candidates can still
	// become review cards.
	CurrentSnapshotVersion = 2
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
	ID              string     `json:"id"`
	CaptureID       string     `json:"capture_id"`
	KnowledgeItemID *string    `json:"knowledge_item_id"`
	CardType        string     `json:"card_type"`
	Question        string     `json:"question"`
	Answer          string     `json:"answer"`
	Explanation     *string    `json:"explanation"`
	CreatedAt       time.Time  `json:"created_at"`
	ConsumedAt      *time.Time `json:"consumed_at"`
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
