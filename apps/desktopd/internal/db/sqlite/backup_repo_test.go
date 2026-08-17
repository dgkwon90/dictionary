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
	"neulsang/desktopd/internal/domain/explain"
	"neulsang/desktopd/internal/domain/settings"
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
	// One knowledge item can own several cards of the same card_type, but only when they
	// differ in context — a cloze for "stale" inside one sentence is a different card from
	// a cloze for the same word inside another. A backup must restore every one of them.
	// Two cards identical in (owner, type, context) are the same card and must not double
	// up, which is what ux_review_cards_identity enforces.
	snapshot := backupTestSnapshot()
	base := backupBaseTime()
	snapshot.KnowledgeItems = append(snapshot.KnowledgeItems, backup.KnowledgeItemRow{
		ID: "ki-sent", NormalizedKey: "the bread went stale", SurfaceText: "The bread went stale.",
		LearnKind: "sentence", Language: "en",
		FirstSeenAt: base, LastSeenAt: base, UpdatedAt: base,
	})
	snapshot.ReviewCards = append(snapshot.ReviewCards, backup.ReviewCardRow{
		ID:                     "rc-2",
		KnowledgeItemID:        "ki-1",
		ContextKnowledgeItemID: stringPtr("ki-sent"),
		CardType:               "meaning",
		Question:               "What does stale mean? (in the bread sentence)",
		Answer:                 "오래된",
		State:                  "new",
		CreatedAt:              base.Add(10 * time.Minute),
		UpdatedAt:              base.Add(10 * time.Minute),
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
		t.Fatalf("review_cards count = %d, want 2 (same type, different context)", count)
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
	// The imported card has the same identity (owner, type, no context) as the one already
	// here, so it is skipped in favour of the destination's live scheduling state rather
	// than added as a duplicate. Its id is redirected to the surviving card, which is what
	// lets the imported review_log attach to something real.
	if result.KnowledgeItems.Merged != 1 || result.LearnerItems.Updated != 1 || result.ReviewCards.Skipped != 1 {
		t.Fatalf("Import() result = %#v", result)
	}
	if result.ReviewLogs.Inserted != 1 {
		t.Fatalf("ReviewLogs.Inserted = %d, want 1 (log follows the surviving card)", result.ReviewLogs.Inserted)
	}

	var knowledgeCount int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_items WHERE normalized_key = ? AND learn_kind = ?`, "stale", "word").Scan(&knowledgeCount); err != nil {
		t.Fatalf("count knowledge_items: %v", err)
	}
	if knowledgeCount != 1 {
		t.Fatalf("knowledge_items count = %d, want 1", knowledgeCount)
	}

	var knowledgeID, surface string
	var firstSeen, lastSeen time.Time
	if err := database.QueryRowContext(ctx,
		`SELECT id, surface_text, first_seen_at, last_seen_at FROM knowledge_items WHERE normalized_key = ? AND learn_kind = ?`,
		"stale", "word").Scan(&knowledgeID, &surface, &firstSeen, &lastSeen); err != nil {
		t.Fatalf("select knowledge item: %v", err)
	}
	if knowledgeID != "ki-existing" || surface != "imported surface" {
		t.Fatalf("knowledge item = id %q surface %q", knowledgeID, surface)
	}
	if !firstSeen.Equal(base.Add(-48*time.Hour)) || !lastSeen.Equal(base.Add(48*time.Hour)) {
		t.Fatalf("knowledge times = first %v last %v", firstSeen, lastSeen)
	}

	var askCount, wrongCount, attemptCount, correctCount int64
	var status string
	var lastUnknown sql.NullTime
	if err := database.QueryRowContext(ctx,
		`SELECT ask_count, unknown_count, attempt_count, correct_count, status, last_unknown_at
FROM learner_items WHERE knowledge_item_id = ?`, "ki-existing").
		Scan(&askCount, &wrongCount, &attemptCount, &correctCount, &status, &lastUnknown); err != nil {
		t.Fatalf("select learner item: %v", err)
	}
	// Counters take the higher of the two histories, except attempts/correct which move
	// together — maxing them apart could report more correct answers than attempts.
	if askCount != 5 || wrongCount != 4 || attemptCount != 5 || correctCount != 4 || status != "active" {
		t.Fatalf("learner item = ask %d unknown %d attempts %d correct %d status %q", askCount, wrongCount, attemptCount, correctCount, status)
	}
	if !lastUnknown.Valid || !lastUnknown.Time.Equal(base.Add(72*time.Hour)) {
		t.Fatalf("last_unknown_at = %#v, want imported newer time", lastUnknown)
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

	// The imported card has the same identity as the live one, so only the live card
	// remains — with its own scheduling state untouched.
	var cardCount int
	if err := database.QueryRowContext(ctx,
		`SELECT count(*) FROM review_cards WHERE knowledge_item_id = ?`, "ki-existing").Scan(&cardCount); err != nil {
		t.Fatalf("count review cards: %v", err)
	}
	if cardCount != 1 {
		t.Fatalf("review_cards count = %d, want 1 (imported duplicate collapsed into the live card)", cardCount)
	}

	// The imported log must follow the surviving card rather than dangle on an id that
	// was never inserted — otherwise the whole import fails on a foreign key.
	var logCardID string
	if err := database.QueryRowContext(ctx, `SELECT review_card_id FROM review_logs WHERE id = ?`, "rl-1").Scan(&logCardID); err != nil {
		t.Fatalf("select review log: %v", err)
	}
	if logCardID != "rc-existing" {
		t.Fatalf("review log card id = %q, want rc-existing", logCardID)
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
			TextHash: "hash-1", CreatedAt: base, TriageState: "unseen",
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
			TextHash: "hash-failed", CreatedAt: base, TriageState: "unseen",
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

// TestBackupRepositoryRestoreUnconsumedCandidateStillBecomesCard is RW-04's third
// completion criterion: a knowledge item restored along with an unconsumed
// review_card_candidate must still be able to produce a review card. Before RW-04,
// review_card_candidates was not in the snapshot, so a restored item that had not
// been registered for learning yet permanently lost its ability to ever become a
// card. The registration path is now "학습할래요" on the word rather than the old
// mark-unknown endpoint, so the restore has to carry the capture_items link too.
func TestBackupRepositoryRestoreUnconsumedCandidateStillBecomesCard(t *testing.T) {
	ctx := context.Background()
	base := backupBaseTime()
	snapshot := &backup.Snapshot{
		Version: backup.CurrentSnapshotVersion,
		KnowledgeItems: []backup.KnowledgeItemRow{{
			ID: "ki-fresh", NormalizedKey: "idempotent", SurfaceText: "idempotent", LearnKind: "word",
			Language: "en", FirstSeenAt: base, LastSeenAt: base,
		}},
		Captures: []backup.CaptureRow{{
			ID: "cap-fresh", SelectedText: "idempotent", InputMode: "manual",
			TextHash: "hash-fresh", CreatedAt: base, TriageState: "unseen",
		}},
		CaptureItems: []backup.CaptureItemRow{{
			ID: "ci-fresh", CaptureID: "cap-fresh", KnowledgeItemID: "ki-fresh",
			Role: "sub_item", Confidence: 0.9, CreatedAt: base, UpdatedAt: base,
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

	registered, err := NewSearchRepository(targetDB).RegisterWordForLearning(ctx, "cap-fresh", base.Add(time.Hour))
	if err != nil {
		t.Fatalf("RegisterWordForLearning() error = %v", err)
	}
	if registered.CardsCreated != 1 {
		t.Fatalf("RegisterWordForLearning() cards_created = %d, want 1 (restored candidate consumed)", registered.CardsCreated)
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

// Pre-redesign snapshots are refused rather than migrated. A v1/v2 file describes a
// model that no longer exists — captures filed as saved/archived, no word/sentence
// split, no accuracy — and nothing in it says whether the user ever decided to learn
// a given capture. Importing one would have to invent that decision, so the honest
// answer is a clear rejection with the data left untouched.
func TestBackupRepositoryImportRejectsPreRedesignSnapshots(t *testing.T) {
	ctx := context.Background()
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			targetDB := openMigratedDB(t)
			snapshot := backupTestSnapshot()
			snapshot.Version = version

			_, err := NewBackupRepository(targetDB).Import(ctx, snapshot)
			if !errors.Is(err, backup.ErrUnsupportedSnapshotVersion) {
				t.Fatalf("Import() error = %v, want ErrUnsupportedSnapshotVersion", err)
			}
			if count := tableCount(t, targetDB, "captures"); count != 0 {
				t.Fatalf("captures count = %d, want 0 (rejected before any insert)", count)
			}
		})
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
			LearnKind:      "word",
			Language:       "en",
			Pronunciation:  stringPtr("steil"),
			MeaningKo:      stringPtr("오래된"),
			DescriptionKo:  stringPtr("not fresh"),
			DomainCategory: stringPtr("general"),
			FirstSeenAt:    base.Add(-48 * time.Hour),
			LastSeenAt:     base.Add(48 * time.Hour),
			UpdatedAt:      base.Add(48 * time.Hour),
		}},
		Captures: []backup.CaptureRow{{
			ID:           "cap-1",
			SourceApp:    stringPtr("Safari"),
			SourceType:   stringPtr("browser"),
			SelectedText: "stale",
			DetectedLang: stringPtr("en"),
			InputMode:    "clipboard",
			TextHash:     "hash-1",
			InputType:    stringPtr("word"),
			LearnKind:    stringPtr("word"),
			TriageState:  "learning",
			CreatedAt:    base,
			UpdatedAt:    base,
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
			Role:            "sub_item",
			Confidence:      0.91,
			CharStart:       intPtr(4),
			CharEnd:         intPtr(9),
			SelectedAt:      timePtr(base.Add(2 * time.Minute)),
			CreatedAt:       base.Add(2 * time.Minute),
			UpdatedAt:       base.Add(2 * time.Minute),
		}},
		LearnerItems: []backup.LearnerItemRow{{
			ID:              "li-1",
			KnowledgeItemID: "ki-1",
			AskCount:        2,
			UnknownCount:    4,
			AttemptCount:    5,
			CorrectCount:    4,
			LastAskedAt:     timePtr(base.Add(-24 * time.Hour)),
			LastUnknownAt:   timePtr(base.Add(72 * time.Hour)),
			LastGradedAt:    timePtr(base.Add(2 * time.Hour)),
			RegisteredAt:    base.Add(-24 * time.Hour),
			Status:          "active",
			UpdatedAt:       base.Add(2 * time.Hour),
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
			IntervalDays:    2.5,
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
			IsCorrect:    true,
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
id, normalized_key, surface_text, learn_kind, item_type, language, pronunciation, meaning_ko, description_ko, domain_category, first_seen_at, last_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.NormalizedKey, row.SurfaceText, row.LearnKind, row.ItemType, row.Language, row.Pronunciation, row.MeaningKo,
			row.DescriptionKo, row.DomainCategory, row.FirstSeenAt.UTC(), row.LastSeenAt.UTC(), row.UpdatedAt.UTC())
	}
	for _, row := range snapshot.Captures {
		execTestSQL(t, database, `INSERT INTO captures(
id, parent_capture_id, source_app, source_type, selected_text, detected_lang, input_mode, text_hash, input_type, learn_kind, triage_state, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.ParentCaptureID, row.SourceApp, row.SourceType, row.SelectedText, row.DetectedLang, row.InputMode,
			row.TextHash, row.InputType, row.LearnKind, row.TriageState, row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	}
	for _, row := range snapshot.Explanations {
		execTestSQL(t, database, `INSERT INTO explanations(
id, capture_id, brief_ko, detailed_ko, pronunciation, examples_json, terms_json, difficulty_estimate, category, raw_response_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.CaptureID, row.BriefKo, row.DetailedKo, row.Pronunciation, row.ExamplesJSON, row.TermsJSON,
			row.DifficultyEstimate, row.Category, row.RawResponseJSON, row.CreatedAt.UTC())
	}
	for _, row := range snapshot.CaptureItems {
		execTestSQL(t, database, `INSERT INTO capture_items(id, capture_id, knowledge_item_id, role, confidence, char_start, char_end, selected_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.CaptureID, row.KnowledgeItemID, row.Role, row.Confidence,
			row.CharStart, row.CharEnd, timePtrArg(row.SelectedAt), row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	}
	for _, row := range snapshot.LearnerItems {
		execTestSQL(t, database, `INSERT INTO learner_items(
id, knowledge_item_id, ask_count, unknown_count, attempt_count, correct_count, registered_at, last_asked_at, last_unknown_at, last_graded_at, status, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.KnowledgeItemID, row.AskCount, row.UnknownCount, row.AttemptCount, row.CorrectCount, row.RegisteredAt.UTC(),
			timePtrArg(row.LastAskedAt), timePtrArg(row.LastUnknownAt), timePtrArg(row.LastGradedAt), row.Status, row.UpdatedAt.UTC())
	}
	for _, row := range snapshot.ReviewCards {
		execTestSQL(t, database, `INSERT INTO review_cards(
id, knowledge_item_id, context_knowledge_item_id, card_type, question, answer, explanation, state, due_at, interval_days, reps, lapses, last_review_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.KnowledgeItemID, row.ContextKnowledgeItemID, row.CardType, row.Question, row.Answer, row.Explanation, row.State, timePtrArg(row.DueAt),
			row.IntervalDays, row.Reps, row.Lapses, timePtrArg(row.LastReviewAt), row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	}
	for _, row := range snapshot.ReviewLogs {
		execTestSQL(t, database, `INSERT INTO review_logs(id, review_card_id, source, rating, is_correct, elapsed_ms, reviewed_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.ReviewCardID, row.Source, row.Rating, row.IsCorrect, row.ElapsedMs, row.ReviewedAt.UTC())
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
id, normalized_key, surface_text, learn_kind, language, first_seen_at, last_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"ki-existing", "stale", "live surface", "word", "en", base.Add(-24*time.Hour).UTC(), base.Add(-time.Hour).UTC(), base.Add(-time.Hour).UTC())
	execTestSQL(t, database, `INSERT INTO learner_items(
id, knowledge_item_id, ask_count, unknown_count, attempt_count, correct_count, registered_at, last_asked_at, last_unknown_at, last_graded_at, status, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"li-existing", "ki-existing", 5, 2, 3, 1, base.Add(-24*time.Hour).UTC(), base.Add(time.Hour).UTC(), base.Add(time.Hour).UTC(), base.Add(time.Hour).UTC(), "active", base.Add(time.Hour).UTC())
	execTestSQL(t, database, `INSERT INTO review_cards(
id, knowledge_item_id, card_type, question, answer, state, due_at, interval_days, reps, lapses, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"rc-existing", "ki-existing", "meaning", "live q", "live a", "review", existingDue.UTC(), 9.0, 9, 1, base.UTC(), base.UTC())
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

// Which words the user said they did not know, and where those words sit in the
// sentence, are decisions only they can make — a restore that loses them hands back a
// sentence that looks untouched. Export is asserted against the seeded rows rather than
// against another Export, because two exports agree with each other even when both are
// missing the same column.
func TestBackupRepositoryExportPreservesWordSelections(t *testing.T) {
	database := openMigratedDB(t)
	snapshot := backupTestSnapshot()
	insertSnapshotRows(t, database, snapshot)

	exported, err := NewBackupRepository(database).Export(context.Background())
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(exported.CaptureItems) != 1 {
		t.Fatalf("exported %d capture_items, want 1", len(exported.CaptureItems))
	}
	got, want := exported.CaptureItems[0], snapshot.CaptureItems[0]
	if got.SelectedAt == nil || !got.SelectedAt.Equal(*want.SelectedAt) {
		t.Errorf("selected_at = %v, want %v", got.SelectedAt, want.SelectedAt)
	}
	if got.CharStart == nil || *got.CharStart != *want.CharStart || got.CharEnd == nil || *got.CharEnd != *want.CharEnd {
		t.Errorf("char range = [%v, %v], want [%v, %v]", got.CharStart, got.CharEnd, want.CharStart, want.CharEnd)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("updated_at = %v, want %v", got.UpdatedAt, want.UpdatedAt)
	}
}

// Restoring rebuilt every card the user had earned and then asked them in a rhythm they
// never chose, because preferences were not in the snapshot at all.
func TestBackupRepositoryAppSettingsSurviveRestore(t *testing.T) {
	ctx := context.Background()
	sourceDB := openMigratedDB(t)

	saved := settings.Defaults()
	saved.MorningReviewTime = "06:45"
	saved.ReviewIntervals.FirstGoodDays = 5
	saved.ReviewIntervals.GoodMultiplier = 1.8
	saved.AIFormat = explain.Format{PromptStyle: "짧게, 백엔드 문맥으로", ExamplesCount: 1}
	if err := NewSettingsRepository(sourceDB).Save(ctx, saved); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	exported, err := NewBackupRepository(sourceDB).Export(ctx)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	exported.Version = backup.CurrentSnapshotVersion
	if len(exported.AppSettings) == 0 {
		t.Fatal("exported no app_settings")
	}

	targetDB := openMigratedDB(t)
	result, err := NewBackupRepository(targetDB).Import(ctx, exported)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.AppSettings.Inserted != len(exported.AppSettings) {
		t.Fatalf("imported %d app_settings, want %d", result.AppSettings.Inserted, len(exported.AppSettings))
	}

	restored, err := NewSettingsRepository(targetDB).Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if restored != saved {
		t.Fatalf("restored preferences = %+v, want %+v", restored, saved)
	}
}

// Import is additive everywhere else, and a setting is the one row where overwriting
// changes how the app behaves the moment the import finishes: restoring a year-old
// backup must not silently replace the schedule in use today.
func TestBackupRepositoryImportKeepsExistingSettings(t *testing.T) {
	ctx := context.Background()
	sourceDB := openMigratedDB(t)
	old := settings.Defaults()
	old.MorningReviewTime = "05:00"
	if err := NewSettingsRepository(sourceDB).Save(ctx, old); err != nil {
		t.Fatalf("source Save() error = %v", err)
	}
	exported, err := NewBackupRepository(sourceDB).Export(ctx)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	exported.Version = backup.CurrentSnapshotVersion

	targetDB := openMigratedDB(t)
	live := settings.Defaults()
	live.MorningReviewTime = "11:11"
	if err := NewSettingsRepository(targetDB).Save(ctx, live); err != nil {
		t.Fatalf("target Save() error = %v", err)
	}

	result, err := NewBackupRepository(targetDB).Import(ctx, exported)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.AppSettings.Inserted != 0 || result.AppSettings.Skipped != len(exported.AppSettings) {
		t.Fatalf("app_settings result = %+v, want everything skipped", result.AppSettings)
	}

	after, err := NewSettingsRepository(targetDB).Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if after != live {
		t.Fatalf("live preferences changed to %+v, want %+v", after, live)
	}
}

// Backing up twice to the same file is the ordinary case — the user keeps one
// "neulsang-backup.db" and refreshes it — and it used to fail: VACUUM INTO
// refuses a destination that already holds bytes, so the second run returned a
// 500 after the save dialog had already asked about replacing the file.
func TestBackupRepositoryBackupFileReplacesExistingBackup(t *testing.T) {
	ctx := context.Background()
	database := openMigratedDB(t)
	insertSnapshotRows(t, database, backupTestSnapshot())
	repo := NewBackupRepository(database)
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.db")

	if _, err := repo.BackupFile(ctx, path); err != nil {
		t.Fatalf("first BackupFile() error = %v", err)
	}
	result, err := repo.BackupFile(ctx, path)
	if err != nil {
		t.Fatalf("second BackupFile() error = %v", err)
	}
	if result.SizeBytes <= 0 {
		t.Fatalf("BackupFile() = %#v, want a non-empty backup", result)
	}

	// Before opening the backup below — that call is what creates -wal/-shm.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "backup.db" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("backup dir contains %v, want only backup.db (temp file left behind)", names)
	}

	backupDB, err := dbpkg.Open(path)
	if err != nil {
		t.Fatalf("open replaced backup db: %v", err)
	}
	defer func() {
		if err := backupDB.Close(); err != nil {
			t.Errorf("close backup db: %v", err)
		}
	}()
	if count := tableCount(t, backupDB, "captures"); count == 0 {
		t.Fatal("replaced backup has no captures")
	}
}

// The driver we ship (modernc) guards VACUUM INTO with a size check rather than
// an existence check, so handing it the destination directly followed a dangling
// symlink and wrote the database wherever the link pointed. Going through a temp
// file and a rename replaces the link itself instead.
func TestBackupRepositoryBackupFileDoesNotFollowDanglingSymlink(t *testing.T) {
	ctx := context.Background()
	database := openMigratedDB(t)
	insertSnapshotRows(t, database, backupTestSnapshot())
	dir := t.TempDir()
	planted := filepath.Join(dir, "planted-elsewhere.db")
	path := filepath.Join(dir, "backup.db")
	if err := os.Symlink(planted, path); err != nil {
		t.Fatalf("create dangling symlink: %v", err)
	}

	if _, err := NewBackupRepository(database).BackupFile(ctx, path); err != nil {
		t.Fatalf("BackupFile() error = %v", err)
	}

	if _, err := os.Lstat(planted); !os.IsNotExist(err) {
		t.Fatalf("symlink target %q exists (err = %v), want the link replaced instead of followed", planted, err)
	}
	stat, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat backup path: %v", err)
	}
	if stat.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("backup path is still a symlink (mode %v)", stat.Mode())
	}
}

func TestBackupRepositoryBackupFileRejectsDirectory(t *testing.T) {
	database := openMigratedDB(t)
	dir := t.TempDir()

	_, err := NewBackupRepository(database).BackupFile(context.Background(), dir)
	if !errors.Is(err, backup.ErrInvalidPath) {
		t.Fatalf("BackupFile() error = %v, want ErrInvalidPath", err)
	}
}

// Nothing is written when the destination's directory does not exist, and the
// failure names the step rather than surfacing as a bare SQLite error.
func TestBackupRepositoryBackupFileMissingDirectory(t *testing.T) {
	database := openMigratedDB(t)
	path := filepath.Join(t.TempDir(), "nodir", "backup.db")

	if _, err := NewBackupRepository(database).BackupFile(context.Background(), path); err == nil {
		t.Fatal("BackupFile() error = nil, want failure for a missing directory")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stat backup path: err = %v, want not-exist", err)
	}
}
