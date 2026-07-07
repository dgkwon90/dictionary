package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/inbox"
)

func TestInboxRepositoryListDerivesEffectiveStatus(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewInboxRepository(database)
	base := time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC)
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{captureID: "capture-new", inboxStatus: "new", jobStatus: "done", createdAt: base.Add(1 * time.Minute)})
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{captureID: "capture-failed", inboxStatus: "new", jobStatus: "done", createdAt: base.Add(2 * time.Minute)})
	insertLookupJobFixture(t, database, "capture-failed-job-latest", "capture-failed", "failed", base.Add(3*time.Minute))
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{captureID: "capture-review", inboxStatus: "new", jobStatus: "done", createdAt: base.Add(4 * time.Minute)})
	insertReviewCardFixture(t, database, "capture-review", "knowledge-review", base.Add(4*time.Minute))
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{captureID: "capture-saved", inboxStatus: "saved", jobStatus: "failed", createdAt: base.Add(5 * time.Minute)})
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{captureID: "capture-archived", inboxStatus: "archived", jobStatus: "done", createdAt: base.Add(6 * time.Minute)})

	items, err := repo.List(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	statusByID := map[string]string{}
	jobStatusByID := map[string]string{}
	for _, item := range items {
		statusByID[item.CaptureID] = item.Status
		jobStatusByID[item.CaptureID] = item.JobStatus
	}
	wantStatusByID := map[string]string{
		"capture-new":      "new",
		"capture-failed":   "failed",
		"capture-review":   "review_added",
		"capture-saved":    "saved",
		"capture-archived": "archived",
	}
	if !reflect.DeepEqual(statusByID, wantStatusByID) {
		t.Fatalf("statusByID = %#v, want %#v", statusByID, wantStatusByID)
	}
	if jobStatusByID["capture-failed"] != "failed" || jobStatusByID["capture-saved"] != "failed" {
		t.Fatalf("jobStatusByID = %#v", jobStatusByID)
	}
}

func TestInboxRepositoryListFiltersByStatus(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewInboxRepository(database)
	base := time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC)
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{captureID: "capture-new", inboxStatus: "new", jobStatus: "done", createdAt: base})
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{captureID: "capture-failed", inboxStatus: "new", jobStatus: "failed", createdAt: base.Add(1 * time.Minute)})
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{captureID: "capture-saved", inboxStatus: "saved", jobStatus: "failed", createdAt: base.Add(2 * time.Minute)})

	items, err := repo.List(context.Background(), "failed", 50)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 || items[0].CaptureID != "capture-failed" || items[0].Status != "failed" {
		t.Fatalf("items = %#v", items)
	}
}

func TestInboxRepositoryListLimitAndSort(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewInboxRepository(database)
	base := time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC)
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{captureID: "capture-old", inboxStatus: "new", jobStatus: "done", createdAt: base})
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{captureID: "capture-new", inboxStatus: "new", jobStatus: "done", createdAt: base.Add(2 * time.Minute)})
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{captureID: "capture-mid", inboxStatus: "new", jobStatus: "done", createdAt: base.Add(1 * time.Minute)})

	items, err := repo.List(context.Background(), "", 2)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	gotIDs := []string{items[0].CaptureID, items[1].CaptureID}
	wantIDs := []string{"capture-new", "capture-mid"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("ids = %#v, want %#v", gotIDs, wantIDs)
	}
}

func TestInboxRepositoryListNullableFieldsAndBrief(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewInboxRepository(database)
	base := time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC)
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{captureID: "capture-empty", inboxStatus: "new", jobStatus: "done", createdAt: base})
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{
		captureID:   "capture-brief",
		inboxStatus: "new",
		jobStatus:   "done",
		sourceApp:   "Safari",
		sourceType:  "browser",
		createdAt:   base.Add(1 * time.Minute),
	})
	insertExplanationFixture(t, database, "capture-brief", "brief ko", base.Add(1*time.Minute))

	items, err := repo.List(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	byID := map[string]inbox.Item{}
	for _, item := range items {
		byID[item.CaptureID] = item
	}
	if byID["capture-empty"].BriefKo != "" || byID["capture-empty"].SourceApp != "" || byID["capture-empty"].SourceType != "" {
		t.Fatalf("capture-empty = %#v", byID["capture-empty"])
	}
	if byID["capture-brief"].BriefKo != "brief ko" || byID["capture-brief"].SourceApp != "Safari" || byID["capture-brief"].SourceType != "browser" {
		t.Fatalf("capture-brief = %#v", byID["capture-brief"])
	}
}

func TestInboxRepositorySetStatus(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewInboxRepository(database)
	insertInboxCaptureFixture(t, database, inboxCaptureFixture{
		captureID:   "capture-1",
		inboxStatus: "new",
		jobStatus:   "done",
		createdAt:   time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC),
	})

	if err := repo.SetStatus(context.Background(), "capture-1", "archived"); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	var status string
	if err := database.QueryRowContext(context.Background(), `SELECT inbox_status FROM captures WHERE id = ?`, "capture-1").Scan(&status); err != nil {
		t.Fatalf("query capture status: %v", err)
	}
	if status != "archived" {
		t.Fatalf("inbox_status = %q, want archived", status)
	}

	err := repo.SetStatus(context.Background(), "missing", "saved")
	if !errors.Is(err, inbox.ErrCaptureNotFound) {
		t.Fatalf("SetStatus() error = %v, want ErrCaptureNotFound", err)
	}
}

type inboxCaptureFixture struct {
	captureID    string
	inboxStatus  string
	jobStatus    string
	selectedText string
	sourceApp    string
	sourceType   string
	createdAt    time.Time
}

func insertInboxCaptureFixture(t *testing.T, database *sql.DB, fixture inboxCaptureFixture) {
	t.Helper()
	selectedText := fixture.selectedText
	if selectedText == "" {
		selectedText = "hello"
	}
	if _, err := database.ExecContext(
		context.Background(),
		`INSERT INTO captures(id, source_app, source_type, selected_text, input_mode, text_hash, created_at, inbox_status)
VALUES (?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?)`,
		fixture.captureID,
		fixture.sourceApp,
		fixture.sourceType,
		selectedText,
		"manual",
		fixture.captureID+"-hash",
		fixture.createdAt,
		fixture.inboxStatus,
	); err != nil {
		t.Fatalf("insert capture fixture: %v", err)
	}
	insertLookupJobFixture(t, database, fixture.captureID+"-job", fixture.captureID, fixture.jobStatus, fixture.createdAt)
}

func insertLookupJobFixture(t *testing.T, database *sql.DB, jobID, captureID, status string, createdAt time.Time) {
	t.Helper()
	if _, err := database.ExecContext(
		context.Background(),
		`INSERT INTO lookup_jobs(id, capture_id, status, created_at) VALUES (?, ?, ?, ?)`,
		jobID,
		captureID,
		status,
		createdAt,
	); err != nil {
		t.Fatalf("insert lookup job fixture: %v", err)
	}
}

func insertExplanationFixture(t *testing.T, database *sql.DB, captureID, briefKo string, createdAt time.Time) {
	t.Helper()
	if _, err := database.ExecContext(
		context.Background(),
		`INSERT INTO explanations(id, capture_id, brief_ko, detailed_ko, created_at) VALUES (?, ?, ?, ?, ?)`,
		captureID+"-explanation",
		captureID,
		briefKo,
		"detailed",
		createdAt,
	); err != nil {
		t.Fatalf("insert explanation fixture: %v", err)
	}
}

func insertReviewCardFixture(t *testing.T, database *sql.DB, captureID, knowledgeItemID string, createdAt time.Time) {
	t.Helper()
	if _, err := database.ExecContext(
		context.Background(),
		`INSERT INTO knowledge_items(id, normalized_key, surface_text, item_type, language, first_seen_at, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		knowledgeItemID,
		"hello-"+captureID,
		"hello",
		"word",
		"en",
		createdAt,
		createdAt,
	); err != nil {
		t.Fatalf("insert knowledge item fixture: %v", err)
	}
	if _, err := database.ExecContext(
		context.Background(),
		`INSERT INTO capture_items(id, capture_id, knowledge_item_id, role, confidence, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		captureID+"-capture-item",
		captureID,
		knowledgeItemID,
		"primary",
		1.0,
		createdAt,
	); err != nil {
		t.Fatalf("insert capture item fixture: %v", err)
	}
	if _, err := database.ExecContext(
		context.Background(),
		`INSERT INTO review_cards(id, knowledge_item_id, card_type, question, answer, state, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		captureID+"-review-card",
		knowledgeItemID,
		"meaning",
		"question",
		"answer",
		"active",
		createdAt,
		createdAt,
	); err != nil {
		t.Fatalf("insert review card fixture: %v", err)
	}
}
