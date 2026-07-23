package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	dbpkg "neulsang/desktopd/internal/db"
	"neulsang/desktopd/internal/domain/backup"
)

func TestBackupRepositoryExportImportRoundTripIntoEmptyDB(t *testing.T) {
	ctx := context.Background()
	sourceDB := openMigratedDB(t)
	insertSnapshotRows(t, sourceDB, backupTestSnapshot())

	exported, err := NewBackupRepository(sourceDB).Export(ctx)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	// Repository.Export doesn't stamp Version (that's Service's job, RW-04) —
	// simulate it here since Repository.Import now validates it directly.
	exported.Version = backup.CurrentSnapshotVersion

	targetDB := openMigratedDB(t)
	result, err := NewBackupRepository(targetDB).Import(ctx, exported)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.KnowledgeItems.Inserted != 1 || result.Captures.Inserted != 1 ||
		result.Explanations.Inserted != 1 || result.CaptureItems.Inserted != 1 ||
		result.LearnerItems.Inserted != 1 || result.ReviewCards.Inserted != 1 ||
		result.ReviewLogs.Inserted != 1 || result.LookupJobs.Inserted != 1 ||
		result.ReviewCardCandidates.Inserted != 1 {
		t.Fatalf("Import() result = %#v", result)
	}

	roundTrip, err := NewBackupRepository(targetDB).Export(ctx)
	if err != nil {
		t.Fatalf("Export() after import error = %v", err)
	}
	roundTrip.Version = backup.CurrentSnapshotVersion // see note above
	if got, want := mustJSON(t, roundTrip), mustJSON(t, exported); got != want {
		t.Fatalf("round trip mismatch\ngot  %s\nwant %s", got, want)
	}
}

func TestBackupRepositoryImportIsIdempotent(t *testing.T) {
	ctx := context.Background()
	targetDB := openMigratedDB(t)
	repo := NewBackupRepository(targetDB)
	snapshot := backupTestSnapshot()

	if _, err := repo.Import(ctx, snapshot); err != nil {
		t.Fatalf("first Import() error = %v", err)
	}
	before := coreTableCounts(t, targetDB)
	second, err := repo.Import(ctx, snapshot)
	if err != nil {
		t.Fatalf("second Import() error = %v", err)
	}
	after := coreTableCounts(t, targetDB)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("counts after re-import = %#v, want %#v", after, before)
	}
	if after["knowledge_items"] != 1 {
		t.Fatalf("knowledge_items count = %d, want 1", after["knowledge_items"])
	}
	if second.KnowledgeItems.Merged != 1 || second.Captures.Skipped != 1 ||
		second.Explanations.Skipped != 1 || second.CaptureItems.Skipped != 1 ||
		second.LearnerItems.Updated != 1 || second.ReviewCards.Skipped != 1 ||
		second.ReviewLogs.Skipped != 1 || second.LookupJobs.Skipped != 1 ||
		second.ReviewCardCandidates.Skipped != 1 {
		t.Fatalf("second Import() result = %#v", second)
	}
}

func TestBackupRepositoryRoundTripPreservesMultipleCardsSameType(t *testing.T) {
	ctx := context.Background()
	// A knowledge item can accrue multiple cards of the same card_type (re-marking a word
	// unknown across captures — there is no UNIQUE(knowledge_item_id, card_type)). A backup
	// must restore every one of them, so review_cards dedup is by id, not (ki, card_type).
	snapshot := backupTestSnapshot()
	base := backupBaseTime()
	snapshot.ReviewCards = append(snapshot.ReviewCards, backup.ReviewCardRow{
		ID:              "rc-2",
		KnowledgeItemID: "ki-1",
		CardType:        "meaning",
		Question:        "What does stale mean? (again)",
		Answer:          "오래된",
		State:           "new",
		CreatedAt:       base.Add(10 * time.Minute),
		UpdatedAt:       base.Add(10 * time.Minute),
	})

	targetDB := openMigratedDB(t)
	result, err := NewBackupRepository(targetDB).Import(ctx, snapshot)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.ReviewCards.Inserted != 2 {
		t.Fatalf("review cards inserted = %d, want 2", result.ReviewCards.Inserted)
	}
	if count := tableCount(t, targetDB, "review_cards"); count != 2 {
		t.Fatalf("review_cards count = %d, want 2 (both same-type cards preserved)", count)
	}
}

func TestBackupRepositoryImportMergesIntoPopulatedDB(t *testing.T) {
	ctx := context.Background()
	database := openMigratedDB(t)
	base := backupBaseTime()
	existingDue := base.Add(30 * 24 * time.Hour)
	seedMergeTarget(t, database, base, existingDue)

	snapshot := backupTestSnapshot()
	result, err := NewBackupRepository(database).Import(ctx, snapshot)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.KnowledgeItems.Merged != 1 || result.LearnerItems.Updated != 1 || result.ReviewCards.Inserted != 1 {
		t.Fatalf("Import() result = %#v", result)
	}

	var knowledgeCount int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_items WHERE normalized_key = ? AND item_type = ?`, "stale", "term").Scan(&knowledgeCount); err != nil {
		t.Fatalf("count knowledge_items: %v", err)
	}
	if knowledgeCount != 1 {
		t.Fatalf("knowledge_items count = %d, want 1", knowledgeCount)
	}

	var knowledgeID, surface string
	var firstSeen, lastSeen time.Time
	if err := database.QueryRowContext(ctx,
		`SELECT id, surface_text, first_seen_at, last_seen_at FROM knowledge_items WHERE normalized_key = ? AND item_type = ?`,
		"stale", "term").Scan(&knowledgeID, &surface, &firstSeen, &lastSeen); err != nil {
		t.Fatalf("select knowledge item: %v", err)
	}
	if knowledgeID != "ki-existing" || surface != "imported surface" {
		t.Fatalf("knowledge item = id %q surface %q", knowledgeID, surface)
	}
	if !firstSeen.Equal(base.Add(-48*time.Hour)) || !lastSeen.Equal(base.Add(48*time.Hour)) {
		t.Fatalf("knowledge times = first %v last %v", firstSeen, lastSeen)
	}

	var askCount, wrongCount, reviewCount int64
	var familiarity, mastery float64
	var status string
	var lastWrong sql.NullTime
	if err := database.QueryRowContext(ctx,
		`SELECT ask_count, wrong_count, review_count, familiarity_score, mastery_score, status, last_wrong_at
FROM learner_items WHERE knowledge_item_id = ?`, "ki-existing").
		Scan(&askCount, &wrongCount, &reviewCount, &familiarity, &mastery, &status, &lastWrong); err != nil {
		t.Fatalf("select learner item: %v", err)
	}
	if askCount != 5 || wrongCount != 4 || reviewCount != 3 || familiarity != 0.6 || mastery != 0.8 || status != "struggling" {
		t.Fatalf("learner item = ask %d wrong %d review %d familiarity %.2f mastery %.2f status %q", askCount, wrongCount, reviewCount, familiarity, mastery, status)
	}
	if !lastWrong.Valid || !lastWrong.Time.Equal(base.Add(72*time.Hour)) {
		t.Fatalf("last_wrong_at = %#v, want imported newer time", lastWrong)
	}

	// Existing live card is preserved untouched (non-destructive: live SRS state intact).
	var existingReps int64
	var existingDueAt sql.NullTime
	if err := database.QueryRowContext(ctx,
		`SELECT reps, due_at FROM review_cards WHERE id = ?`, "rc-existing").Scan(&existingReps, &existingDueAt); err != nil {
		t.Fatalf("select existing review card: %v", err)
	}
	if existingReps != 9 || !existingDueAt.Valid || !existingDueAt.Time.Equal(existingDue) {
		t.Fatalf("existing review card = reps %d due %#v, want live SRS preserved", existingReps, existingDueAt)
	}

	// The imported card (distinct id) is added as its own card under the merged knowledge item.
	var importedKI, importedType string
	var importedReps int64
	if err := database.QueryRowContext(ctx,
		`SELECT knowledge_item_id, card_type, reps FROM review_cards WHERE id = ?`, "rc-1").Scan(&importedKI, &importedType, &importedReps); err != nil {
		t.Fatalf("select imported review card: %v", err)
	}
	if importedKI != "ki-existing" || importedType != "meaning" || importedReps != 1 {
		t.Fatalf("imported review card = ki %q type %q reps %d", importedKI, importedType, importedReps)
	}

	// Its log stays attached to the imported card (id-based remap keeps identity).
	var logCardID string
	if err := database.QueryRowContext(ctx, `SELECT review_card_id FROM review_logs WHERE id = ?`, "rl-1").Scan(&logCardID); err != nil {
		t.Fatalf("select review log: %v", err)
	}
	if logCardID != "rc-1" {
		t.Fatalf("review log card id = %q, want rc-1", logCardID)
	}
}

// TestBackupRepositoryRestoreEnablesExplanationLookup is RW-04's core
// completion criterion (review R-02): restoring into an empty DB must leave a
// capture's explanation reachable through the same path the app actually uses
// (GetSnapshot, which requires a lookup_jobs row — explanations alone aren't
// enough, ADR-0007). Before RW-04, lookup_jobs wasn't in the snapshot at all,
// so this returned ErrCaptureNotFound even though the explanation row existed.
func TestBackupRepositoryRestoreEnablesExplanationLookup(t *testing.T) {
	ctx := context.Background()
	base := backupBaseTime()
	// GetSnapshot's "done" path scans pronunciation as a plain (non-nullable)
	// string (a pre-existing constraint unrelated to RW-04), so unlike
	// backupTestSnapshot()'s explanation this one needs a non-nil value.
	snapshot := &backup.Snapshot{
		Version: backup.CurrentSnapshotVersion,
		Captures: []backup.CaptureRow{{
			ID: "cap-1", SelectedText: "stale", InputMode: "manual",
			TextHash: "hash-1", CreatedAt: base, InboxStatus: "saved",
		}},
		Explanations: []backup.ExplanationRow{{
			ID: "exp-1", CaptureID: "cap-1", BriefKo: "짧은 설명", DetailedKo: "자세한 설명",
			Pronunciation: stringPtr("steil"), ExamplesJSON: stringPtr(`[]`), TermsJSON: stringPtr(`[]`),
			DifficultyEstimate: floatPtr(0.4), Category: stringPtr("general"),
			CreatedAt: base.Add(time.Minute),
		}},
		LookupJobs: []backup.LookupJobRow{{
			ID: "job-1", CaptureID: "cap-1", Status: "done",
			CreatedAt: base, FinishedAt: timePtr(base.Add(time.Minute)),
		}},
	}

	targetDB := openMigratedDB(t)
	if _, err := NewBackupRepository(targetDB).Import(ctx, snapshot); err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	got, err := NewExplainRepository(targetDB).GetSnapshot(ctx, "cap-1")
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v, want success (lookup_jobs row restored)", err)
	}
	if got.Status != "done" {
		t.Fatalf("GetSnapshot().Status = %q, want done", got.Status)
	}
	if got.Result == nil || got.Result.BriefKo == "" {
		t.Fatalf("GetSnapshot().Result = %#v, want the restored explanation", got.Result)
	}
}

// TestBackupRepositoryRestorePreservesFailedLookupJobStatus covers the other
// completion criterion: a failed capture must still read back as failed, not
// silently regress to some other status because the supporting table was
// dropped on restore (review R-02).
func TestBackupRepositoryRestorePreservesFailedLookupJobStatus(t *testing.T) {
	ctx := context.Background()
	base := backupBaseTime()
	snapshot := &backup.Snapshot{
		Version: backup.CurrentSnapshotVersion,
		Captures: []backup.CaptureRow{{
			ID: "cap-failed", SelectedText: "whatever", InputMode: "manual",
			TextHash: "hash-failed", CreatedAt: base, InboxStatus: "new",
		}},
		LookupJobs: []backup.LookupJobRow{{
			ID: "job-failed", CaptureID: "cap-failed", Status: "failed",
			ErrorMessage: stringPtr("gemini: empty response"),
			CreatedAt:    base, FinishedAt: timePtr(base.Add(time.Minute)),
		}},
	}

	targetDB := openMigratedDB(t)
	if _, err := NewBackupRepository(targetDB).Import(ctx, snapshot); err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	got, err := NewExplainRepository(targetDB).GetSnapshot(ctx, "cap-failed")
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v", err)
	}
	if got.Status != "failed" || got.ErrorMessage != "gemini: empty response" {
		t.Fatalf("GetSnapshot() = %#v, want status=failed with the restored error message", got)
	}
}

// TestBackupRepositoryRestoreUnconsumedCandidateEnablesMarkUnknownCard is
// RW-04's third completion criterion: a knowledge item restored along with an
// unconsumed review_card_candidate must still be able to produce a review card
// via mark-unknown (review R-02). Before RW-04, review_card_candidates wasn't
// in the snapshot, so a restored item that hadn't been marked unknown yet
// permanently lost its ability to ever become a card.
func TestBackupRepositoryRestoreUnconsumedCandidateEnablesMarkUnknownCard(t *testing.T) {
	ctx := context.Background()
	base := backupBaseTime()
	snapshot := &backup.Snapshot{
		Version: backup.CurrentSnapshotVersion,
		KnowledgeItems: []backup.KnowledgeItemRow{{
			ID: "ki-fresh", NormalizedKey: "idempotent", SurfaceText: "idempotent", ItemType: "term",
			Language: "en", FirstSeenAt: base, LastSeenAt: base,
		}},
		Captures: []backup.CaptureRow{{
			ID: "cap-fresh", SelectedText: "idempotent", InputMode: "manual",
			TextHash: "hash-fresh", CreatedAt: base, InboxStatus: "new",
		}},
		LearnerItems: []backup.LearnerItemRow{{
			ID: "li-fresh", KnowledgeItemID: "ki-fresh", Status: "active",
		}},
		ReviewCardCandidates: []backup.ReviewCardCandidateRow{{
			ID: "cand-fresh", CaptureID: "cap-fresh", KnowledgeItemID: stringPtr("ki-fresh"),
			CardType: "meaning", Question: "What does idempotent mean?", Answer: "멱등의",
			CreatedAt: base, ConsumedAt: nil, // not yet consumed — this is the point of the test
		}},
	}

	targetDB := openMigratedDB(t)
	result, err := NewBackupRepository(targetDB).Import(ctx, snapshot)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.ReviewCardCandidates.Inserted != 1 {
		t.Fatalf("ReviewCardCandidates.Inserted = %d, want 1", result.ReviewCardCandidates.Inserted)
	}

	mark, err := NewKnowledgeRepository(targetDB).MarkUnknown(ctx, "ki-fresh", base.Add(time.Hour))
	if err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}
	if mark.CandidateCount != 1 || mark.CardsCreated != 1 {
		t.Fatalf("MarkUnknown() = %#v, want candidate_count=1 cards_created=1 (restored candidate consumed)", mark)
	}
	if count := tableCount(t, targetDB, "review_cards"); count != 1 {
		t.Fatalf("review_cards count = %d, want 1", count)
	}
}

// TestBackupRepositoryImportRejectsUnsupportedVersionDirectly closes the gap
// codex review flagged: Service validates version before delegating, but a
// future caller reaching for Repository directly (a CLI tool, a migration
// script) must not be able to skip that gate — so Repository.Import
// re-validates at its own boundary too.
func TestBackupRepositoryImportRejectsUnsupportedVersionDirectly(t *testing.T) {
	ctx := context.Background()
	targetDB := openMigratedDB(t)
	snapshot := backupTestSnapshot()
	snapshot.Version = backup.CurrentSnapshotVersion + 1

	_, err := NewBackupRepository(targetDB).Import(ctx, snapshot)
	if !errors.Is(err, backup.ErrUnsupportedSnapshotVersion) {
		t.Fatalf("Import() error = %v, want ErrUnsupportedSnapshotVersion (repository must validate directly, not just Service)", err)
	}
	if count := tableCount(t, targetDB, "captures"); count != 0 {
		t.Fatalf("captures count = %d, want 0 (rejected before any insert)", count)
	}
}

// TestBackupRepositoryImportAcceptsV1FixtureWithoutNewTables is the backward-
// compatibility half of RW-04: a snapshot produced before RW-04 (version 1,
// no lookup_jobs/review_card_candidates fields at all) must still import
// cleanly — those two importers just iterate zero rows.
func TestBackupRepositoryImportAcceptsV1FixtureWithoutNewTables(t *testing.T) {
	ctx := context.Background()
	v1JSON := `{
		"version": 1,
		"exported_at": "2026-07-01T00:00:00Z",
		"knowledge_items": [],
		"captures": [{"id":"cap-v1","selected_text":"legacy","input_mode":"manual","text_hash":"hash-v1","created_at":"2026-07-01T00:00:00Z","inbox_status":"new"}],
		"explanations": [],
		"capture_items": [],
		"learner_items": [],
		"review_cards": [],
		"review_logs": []
	}`
	var snapshot backup.Snapshot
	if err := json.Unmarshal([]byte(v1JSON), &snapshot); err != nil {
		t.Fatalf("unmarshal v1 fixture: %v", err)
	}
	if snapshot.LookupJobs != nil || snapshot.ReviewCardCandidates != nil {
		t.Fatalf("v1 fixture unexpectedly populated new fields: %#v / %#v", snapshot.LookupJobs, snapshot.ReviewCardCandidates)
	}

	targetDB := openMigratedDB(t)
	result, err := NewBackupRepository(targetDB).Import(ctx, &snapshot)
	if err != nil {
		t.Fatalf("Import() v1 fixture error = %v, want success", err)
	}
	if result.Captures.Inserted != 1 {
		t.Fatalf("Captures.Inserted = %d, want 1", result.Captures.Inserted)
	}
	if result.LookupJobs.Inserted != 0 || result.ReviewCardCandidates.Inserted != 0 {
		t.Fatalf("expected zero rows for the new tables from a v1 fixture, got %#v", result)
	}
}

func TestBackupRepositoryBackupFileWritesValidSQLiteFile(t *testing.T) {
	ctx := context.Background()
	database := openMigratedDB(t)
	insertSnapshotRows(t, database, backupTestSnapshot())
	path := filepath.Join(t.TempDir(), "backup.db")

	result, err := NewBackupRepository(database).BackupFile(ctx, path)
	if err != nil {
		t.Fatalf("BackupFile() error = %v", err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat backup file: %v", err)
	}
	if result.Path != path || result.SizeBytes != stat.Size() || result.SizeBytes <= 0 {
		t.Fatalf("BackupFile() = %#v, stat size %d", result, stat.Size())
	}

	backupDB, err := dbpkg.Open(path)
	if err != nil {
		t.Fatalf("open backup db: %v", err)
	}
	defer func() {
		if err := backupDB.Close(); err != nil {
			t.Fatalf("close backup db: %v", err)
		}
	}()
	var integrity string
	if err := backupDB.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check = %q, want ok", integrity)
	}
	if count := tableCount(t, backupDB, "captures"); count != 1 {
		t.Fatalf("backup captures count = %d, want 1", count)
	}
}

func backupTestSnapshot() *backup.Snapshot {
	base := backupBaseTime()
	return &backup.Snapshot{
		Version: backup.CurrentSnapshotVersion,
		KnowledgeItems: []backup.KnowledgeItemRow{{
			ID:             "ki-1",
			NormalizedKey:  "stale",
			SurfaceText:    "imported surface",
			ItemType:       "term",
			Language:       "en",
			Pos:            stringPtr("adj"),
			Pronunciation:  stringPtr("steil"),
			MeaningKo:      stringPtr("오래된"),
			DescriptionKo:  stringPtr("not fresh"),
			DomainCategory: stringPtr("general"),
			FirstSeenAt:    base.Add(-48 * time.Hour),
			LastSeenAt:     base.Add(48 * time.Hour),
		}},
		Captures: []backup.CaptureRow{{
			ID:           "cap-1",
			SourceApp:    stringPtr("Safari"),
			SourceType:   stringPtr("browser"),
			SourceTitle:  stringPtr("Article"),
			SourceURL:    nil,
			SelectedText: "stale",
			DetectedLang: stringPtr("en"),
			InputMode:    "clipboard",
			TextHash:     "hash-1",
			CreatedAt:    base,
			InboxStatus:  "saved",
		}},
		Explanations: []backup.ExplanationRow{{
			ID:                 "exp-1",
			CaptureID:          "cap-1",
			BriefKo:            "짧은 설명",
			DetailedKo:         "자세한 설명",
			Pronunciation:      nil,
			ExamplesJSON:       stringPtr(`[{"en":"stale bread"}]`),
			TermsJSON:          stringPtr(`[{"surface_text":"stale"}]`),
			DifficultyEstimate: floatPtr(0.4),
			Category:           stringPtr("general"),
			RawResponseJSON:    nil,
			CreatedAt:          base.Add(time.Minute),
		}},
		CaptureItems: []backup.CaptureItemRow{{
			ID:              "ci-1",
			CaptureID:       "cap-1",
			KnowledgeItemID: "ki-1",
			Role:            "primary",
			Confidence:      0.91,
			CreatedAt:       base.Add(2 * time.Minute),
		}},
		LearnerItems: []backup.LearnerItemRow{{
			ID:               "li-1",
			KnowledgeItemID:  "ki-1",
			FamiliarityScore: 0.5,
			MasteryScore:     0.8,
			AskCount:         2,
			WrongCount:       4,
			ReviewCount:      2,
			LastAskedAt:      timePtr(base.Add(-24 * time.Hour)),
			LastWrongAt:      timePtr(base.Add(72 * time.Hour)),
			LastReviewedAt:   timePtr(base.Add(2 * time.Hour)),
			Status:           "struggling",
		}},
		ReviewCards: []backup.ReviewCardRow{{
			ID:              "rc-1",
			KnowledgeItemID: "ki-1",
			CardType:        "meaning",
			Question:        "What does stale mean?",
			Answer:          "오래된",
			Explanation:     stringPtr("Used for old food or ideas."),
			State:           "review",
			DueAt:           timePtr(base.Add(4 * time.Hour)),
			Stability:       2.5,
			Difficulty:      0.7,
			Retrievability:  floatPtr(0.8),
			Reps:            1,
			Lapses:          0,
			LastReviewAt:    timePtr(base.Add(-2 * time.Hour)),
			CreatedAt:       base.Add(3 * time.Minute),
			UpdatedAt:       base.Add(4 * time.Minute),
		}},
		ReviewLogs: []backup.ReviewLogRow{{
			ID:           "rl-1",
			ReviewCardID: "rc-1",
			Source:       "review",
			Rating:       "good",
			ElapsedMs:    intPtr(123),
			ReviewedAt:   base.Add(5 * time.Minute),
		}},
		LookupJobs: []backup.LookupJobRow{{
			ID:            "job-1",
			CaptureID:     "cap-1",
			Status:        "done",
			Provider:      stringPtr("gemini"),
			Model:         stringPtr("gemini-flash-lite-latest"),
			PromptVersion: stringPtr("v1"),
			StartedAt:     timePtr(base.Add(30 * time.Second)),
			FinishedAt:    timePtr(base.Add(time.Minute)),
			CreatedAt:     base,
		}},
		ReviewCardCandidates: []backup.ReviewCardCandidateRow{{
			ID:              "cand-1",
			CaptureID:       "cap-1",
			KnowledgeItemID: stringPtr("ki-1"),
			CardType:        "meaning",
			Question:        "What does stale mean? (candidate)",
			Answer:          "오래된",
			Explanation:     stringPtr("Used for old food or ideas."),
			CreatedAt:       base.Add(time.Minute),
			ConsumedAt:      timePtr(base.Add(3 * time.Minute)),
		}},
	}
}

func backupBaseTime() time.Time {
	return time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC)
}

func insertSnapshotRows(t *testing.T, database *sql.DB, snapshot *backup.Snapshot) {
	t.Helper()
	ctx := context.Background()
	for _, row := range snapshot.KnowledgeItems {
		execTestSQL(t, database, `INSERT INTO knowledge_items(
id, normalized_key, surface_text, item_type, language, pos, pronunciation, meaning_ko, description_ko, domain_category, first_seen_at, last_seen_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.NormalizedKey, row.SurfaceText, row.ItemType, row.Language, row.Pos, row.Pronunciation, row.MeaningKo,
			row.DescriptionKo, row.DomainCategory, row.FirstSeenAt.UTC(), row.LastSeenAt.UTC())
	}
	for _, row := range snapshot.Captures {
		execTestSQL(t, database, `INSERT INTO captures(
id, source_app, source_type, source_title, source_url, selected_text, detected_lang, input_mode, text_hash, created_at, inbox_status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.SourceApp, row.SourceType, row.SourceTitle, row.SourceURL, row.SelectedText, row.DetectedLang, row.InputMode,
			row.TextHash, row.CreatedAt.UTC(), row.InboxStatus)
	}
	for _, row := range snapshot.Explanations {
		execTestSQL(t, database, `INSERT INTO explanations(
id, capture_id, brief_ko, detailed_ko, pronunciation, examples_json, terms_json, difficulty_estimate, category, raw_response_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.CaptureID, row.BriefKo, row.DetailedKo, row.Pronunciation, row.ExamplesJSON, row.TermsJSON,
			row.DifficultyEstimate, row.Category, row.RawResponseJSON, row.CreatedAt.UTC())
	}
	for _, row := range snapshot.CaptureItems {
		execTestSQL(t, database, `INSERT INTO capture_items(id, capture_id, knowledge_item_id, role, confidence, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
			row.ID, row.CaptureID, row.KnowledgeItemID, row.Role, row.Confidence, row.CreatedAt.UTC())
	}
	for _, row := range snapshot.LearnerItems {
		execTestSQL(t, database, `INSERT INTO learner_items(
id, knowledge_item_id, familiarity_score, mastery_score, ask_count, wrong_count, review_count, last_asked_at, last_wrong_at, last_reviewed_at, status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.KnowledgeItemID, row.FamiliarityScore, row.MasteryScore, row.AskCount, row.WrongCount, row.ReviewCount,
			timePtrArg(row.LastAskedAt), timePtrArg(row.LastWrongAt), timePtrArg(row.LastReviewedAt), row.Status)
	}
	for _, row := range snapshot.ReviewCards {
		execTestSQL(t, database, `INSERT INTO review_cards(
id, knowledge_item_id, card_type, question, answer, explanation, state, due_at, stability, difficulty, retrievability, reps, lapses, last_review_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.KnowledgeItemID, row.CardType, row.Question, row.Answer, row.Explanation, row.State, timePtrArg(row.DueAt),
			row.Stability, row.Difficulty, row.Retrievability, row.Reps, row.Lapses, timePtrArg(row.LastReviewAt), row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	}
	for _, row := range snapshot.ReviewLogs {
		execTestSQL(t, database, `INSERT INTO review_logs(id, review_card_id, source, rating, elapsed_ms, reviewed_at)
VALUES (?, ?, ?, ?, ?, ?)`,
			row.ID, row.ReviewCardID, row.Source, row.Rating, row.ElapsedMs, row.ReviewedAt.UTC())
	}
	for _, row := range snapshot.LookupJobs {
		execTestSQL(t, database, `INSERT INTO lookup_jobs(
id, capture_id, status, provider, model, prompt_version, error_message, started_at, finished_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.CaptureID, row.Status, row.Provider, row.Model, row.PromptVersion,
			row.ErrorMessage, timePtrArg(row.StartedAt), timePtrArg(row.FinishedAt), row.CreatedAt.UTC())
	}
	for _, row := range snapshot.ReviewCardCandidates {
		execTestSQL(t, database, `INSERT INTO review_card_candidates(
id, capture_id, knowledge_item_id, card_type, question, answer, explanation, created_at, consumed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.CaptureID, row.KnowledgeItemID, row.CardType, row.Question, row.Answer,
			row.Explanation, row.CreatedAt.UTC(), timePtrArg(row.ConsumedAt))
	}
	_ = ctx
}

func seedMergeTarget(t *testing.T, database *sql.DB, base, existingDue time.Time) {
	t.Helper()
	execTestSQL(t, database, `INSERT INTO knowledge_items(
id, normalized_key, surface_text, item_type, language, first_seen_at, last_seen_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"ki-existing", "stale", "live surface", "term", "en", base.Add(-24*time.Hour).UTC(), base.Add(-time.Hour).UTC())
	execTestSQL(t, database, `INSERT INTO learner_items(
id, knowledge_item_id, familiarity_score, mastery_score, ask_count, wrong_count, review_count, last_asked_at, last_wrong_at, last_reviewed_at, status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"li-existing", "ki-existing", 0.6, 0.4, 5, 2, 3, base.Add(time.Hour).UTC(), base.Add(time.Hour).UTC(), base.Add(time.Hour).UTC(), "active")
	execTestSQL(t, database, `INSERT INTO review_cards(
id, knowledge_item_id, card_type, question, answer, state, due_at, stability, difficulty, reps, lapses, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"rc-existing", "ki-existing", "meaning", "live q", "live a", "review", existingDue.UTC(), 9.0, 0.1, 9, 1, base.UTC(), base.UTC())
}

func execTestSQL(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func coreTableCounts(t *testing.T, database *sql.DB) map[string]int {
	t.Helper()
	counts := make(map[string]int)
	for _, table := range []string{"knowledge_items", "captures", "explanations", "capture_items", "learner_items", "review_cards", "review_logs", "lookup_jobs", "review_card_candidates"} {
		counts[table] = tableCount(t, database, table)
	}
	return counts
}

func tableCount(t *testing.T, database *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := database.QueryRowContext(context.Background(), fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(data)
}

func stringPtr(value string) *string {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}

func intPtr(value int64) *int64 {
	return &value
}

func timePtr(value time.Time) *time.Time {
	utc := value.UTC()
	return &utc
}

func timePtrArg(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}
