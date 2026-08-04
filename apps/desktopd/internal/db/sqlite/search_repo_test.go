package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/capture"
	"neulsang/desktopd/internal/domain/search"
)

func seedSentenceCapture(t *testing.T, database *sql.DB, captureID, text string, at time.Time) {
	t.Helper()
	execTestSQL(t, database, `INSERT INTO captures(
id, selected_text, input_mode, text_hash, learn_kind, triage_state, created_at, updated_at
) VALUES (?, ?, 'manual', ?, 'sentence', 'needs_selection', ?, ?)`,
		captureID, text, captureID+"-hash", at, at)
	execTestSQL(t, database, `INSERT INTO explanations(
id, capture_id, brief_ko, detailed_ko, created_at
) VALUES (?, ?, ?, ?, ?)`, captureID+"-exp", captureID, "캐시 항목이 오래돼 버려졌다", "자세한 설명", at)
}

// seedSentenceWord links an extracted term to a sentence capture the way the explain
// repository would.
func seedSentenceWord(t *testing.T, database *sql.DB, captureID, knowledgeID, surface string, charStart, charEnd int, at time.Time) {
	t.Helper()
	execTestSQL(t, database, `INSERT INTO knowledge_items(
id, normalized_key, surface_text, learn_kind, language, meaning_ko, first_seen_at, last_seen_at, updated_at
) VALUES (?, ?, ?, 'word', 'en', ?, ?, ?, ?)`,
		knowledgeID, knowledgeID+"-key", surface, "뜻-"+surface, at, at, at)
	execTestSQL(t, database, `INSERT INTO capture_items(
id, capture_id, knowledge_item_id, role, confidence, char_start, char_end, created_at, updated_at
) VALUES (?, ?, ?, 'sub_item', 0.8, ?, ?, ?, ?)`,
		"ci-"+knowledgeID, captureID, knowledgeID, charStart, charEnd, at, at)
	execTestSQL(t, database, `INSERT INTO review_card_candidates(
id, capture_id, knowledge_item_id, card_type, question, answer, created_at
) VALUES (?, ?, ?, 'meaning', ?, ?, ?)`,
		"cand-"+knowledgeID, captureID, knowledgeID, "q-"+surface, "a-"+surface, at)
}

func TestSearchRepositoryCompleteSelectionRegistersSentenceAndWords(t *testing.T) {
	database := openMigratedDB(t)
	ctx := context.Background()
	at := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	seedSentenceCapture(t, database, "cap-1", "The cache entry went stale after five minutes.", at)
	seedSentenceWord(t, database, "cap-1", "know-stale", "stale", 22, 27, at)
	seedSentenceWord(t, database, "cap-1", "know-entry", "entry", 10, 15, at)

	repo := NewSearchRepository(database)
	if err := repo.SetSelected(ctx, "cap-1", "know-stale", true, at); err != nil {
		t.Fatalf("SetSelected() error = %v", err)
	}

	result, err := repo.CompleteSelection(ctx, search.CompleteInput{CaptureID: "cap-1"}, at)
	if err != nil {
		t.Fatalf("CompleteSelection() error = %v", err)
	}
	if result.TriageState != capture.TriageLearning {
		t.Fatalf("triage_state = %q, want learning", result.TriageState)
	}
	// The sentence and the one chosen word both become learning items; the word the user
	// did not pick must not.
	if len(result.LearningItemIDs) != 2 {
		t.Fatalf("learning items = %#v, want the sentence and one word", result.LearningItemIDs)
	}
	assertLearnerExists(t, database, "know-stale", true)
	assertLearnerExists(t, database, "know-entry", false)

	// The sentence is stored as a knowledge item of its own so it can own cards.
	var sentenceID, surface string
	if err := database.QueryRowContext(ctx,
		`SELECT id, surface_text FROM knowledge_items WHERE learn_kind = 'sentence'`).Scan(&sentenceID, &surface); err != nil {
		t.Fatalf("select sentence knowledge item: %v", err)
	}
	if surface != "The cache entry went stale after five minutes." {
		t.Fatalf("sentence surface_text = %q", surface)
	}
	var role string
	if err := database.QueryRowContext(ctx,
		`SELECT role FROM capture_items WHERE capture_id = ? AND knowledge_item_id = ?`,
		"cap-1", sentenceID).Scan(&role); err != nil {
		t.Fatalf("select sentence link: %v", err)
	}
	if role != captureItemRoleSentenceSelf {
		t.Fatalf("sentence link role = %q, want %q", role, captureItemRoleSentenceSelf)
	}

	// One card for the sentence's meaning, one for the chosen word.
	if result.CardsCreated != 2 {
		t.Fatalf("cards created = %d, want 2", result.CardsCreated)
	}
	assertCardCount(t, database, sentenceID, 1)
	assertCardCount(t, database, "know-stale", 1)
	assertCardCount(t, database, "know-entry", 0)
}

// A sentence the user could not break down into words is still a real thing they did
// not understand, so it registers on its own rather than trapping them in the list.
func TestSearchRepositoryCompleteSelectionWithNoUnknownWords(t *testing.T) {
	database := openMigratedDB(t)
	ctx := context.Background()
	at := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	seedSentenceCapture(t, database, "cap-1", "It is what it is.", at)
	seedSentenceWord(t, database, "cap-1", "know-is", "is", 3, 5, at)

	result, err := NewSearchRepository(database).CompleteSelection(ctx,
		search.CompleteInput{CaptureID: "cap-1", NoUnknownWords: true}, at)
	if err != nil {
		t.Fatalf("CompleteSelection() error = %v", err)
	}
	if len(result.LearningItemIDs) != 1 {
		t.Fatalf("learning items = %#v, want only the sentence", result.LearningItemIDs)
	}
	assertLearnerExists(t, database, "know-is", false)
	if result.CardsCreated != 1 {
		t.Fatalf("cards created = %d, want 1 (sentence meaning only)", result.CardsCreated)
	}
}

func TestSearchRepositoryCompleteSelectionRefusesWithoutChoices(t *testing.T) {
	database := openMigratedDB(t)
	ctx := context.Background()
	at := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	seedSentenceCapture(t, database, "cap-1", "The cache entry went stale.", at)
	seedSentenceWord(t, database, "cap-1", "know-stale", "stale", 22, 27, at)

	_, err := NewSearchRepository(database).CompleteSelection(ctx, search.CompleteInput{CaptureID: "cap-1"}, at)
	if !errors.Is(err, capture.ErrSelectionRequired) {
		t.Fatalf("CompleteSelection() error = %v, want ErrSelectionRequired", err)
	}
	// Nothing may be left behind by the refused attempt.
	var captureState string
	if err := database.QueryRowContext(ctx, `SELECT triage_state FROM captures WHERE id = ?`, "cap-1").Scan(&captureState); err != nil {
		t.Fatalf("select capture: %v", err)
	}
	if captureState != capture.TriageNeedsSelection {
		t.Fatalf("triage_state = %q, want it unchanged", captureState)
	}
	if count := tableCount(t, database, "learner_items"); count != 0 {
		t.Fatalf("learner_items count = %d, want 0", count)
	}
}

// Completing twice must not double-register anything — the same sentence merges by key
// and the cards are held to their identity.
func TestSearchRepositoryCompleteSelectionIsIdempotent(t *testing.T) {
	database := openMigratedDB(t)
	ctx := context.Background()
	at := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	seedSentenceCapture(t, database, "cap-1", "The cache entry went stale.", at)
	seedSentenceWord(t, database, "cap-1", "know-stale", "stale", 22, 27, at)

	repo := NewSearchRepository(database)
	if err := repo.SetSelected(ctx, "cap-1", "know-stale", true, at); err != nil {
		t.Fatalf("SetSelected() error = %v", err)
	}
	first, err := repo.CompleteSelection(ctx, search.CompleteInput{CaptureID: "cap-1"}, at)
	if err != nil {
		t.Fatalf("first CompleteSelection() error = %v", err)
	}
	second, err := repo.CompleteSelection(ctx, search.CompleteInput{CaptureID: "cap-1"}, at.Add(time.Hour))
	if err != nil {
		t.Fatalf("second CompleteSelection() error = %v", err)
	}
	if first.CardsCreated != 2 || second.CardsCreated != 0 {
		t.Fatalf("cards created first=%d second=%d, want 2 then 0", first.CardsCreated, second.CardsCreated)
	}
	if count := tableCount(t, database, "review_cards"); count != 2 {
		t.Fatalf("review_cards count = %d, want 2", count)
	}
	if count := tableCount(t, database, "knowledge_items"); count != 2 {
		t.Fatalf("knowledge_items count = %d, want 2 (one word, one sentence)", count)
	}
}

func TestSearchRepositoryGetReportsSelectionState(t *testing.T) {
	database := openMigratedDB(t)
	ctx := context.Background()
	at := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	seedSentenceCapture(t, database, "cap-1", "The cache entry went stale.", at)
	seedSentenceWord(t, database, "cap-1", "know-entry", "entry", 10, 15, at)
	seedSentenceWord(t, database, "cap-1", "know-stale", "stale", 22, 27, at)

	repo := NewSearchRepository(database)
	if err := repo.SetSelected(ctx, "cap-1", "know-stale", true, at); err != nil {
		t.Fatalf("SetSelected() error = %v", err)
	}

	detail, err := repo.Get(ctx, "cap-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if detail.LearnKind != capture.LearnKindSentence || detail.BriefKo == "" {
		t.Fatalf("detail = %#v", detail)
	}
	if len(detail.Items) != 2 {
		t.Fatalf("items = %#v, want 2", detail.Items)
	}
	// Ordered by position in the sentence so the UI can walk it left to right.
	if detail.Items[0].SurfaceText != "entry" || detail.Items[1].SurfaceText != "stale" {
		t.Fatalf("item order = %q, %q", detail.Items[0].SurfaceText, detail.Items[1].SurfaceText)
	}
	if detail.Items[0].Selected || !detail.Items[1].Selected {
		t.Fatalf("selection flags = %v, %v", detail.Items[0].Selected, detail.Items[1].Selected)
	}
	if detail.Items[1].CharStart != 22 || detail.Items[1].CharEnd != 27 {
		t.Fatalf("offsets = %d..%d, want 22..27", detail.Items[1].CharStart, detail.Items[1].CharEnd)
	}
}

func TestSearchRepositorySetSelectedRejectsForeignItem(t *testing.T) {
	database := openMigratedDB(t)
	at := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	seedSentenceCapture(t, database, "cap-1", "The cache entry went stale.", at)

	err := NewSearchRepository(database).SetSelected(context.Background(), "cap-1", "know-missing", true, at)
	if !errors.Is(err, search.ErrCaptureNotFound) {
		t.Fatalf("SetSelected() error = %v, want ErrCaptureNotFound", err)
	}
}

func assertLearnerExists(t *testing.T, database *sql.DB, knowledgeItemID string, want bool) {
	t.Helper()
	var count int
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM learner_items WHERE knowledge_item_id = ?`, knowledgeItemID).Scan(&count); err != nil {
		t.Fatalf("count learner_items for %s: %v", knowledgeItemID, err)
	}
	if want && count != 1 {
		t.Errorf("learner_items for %s = %d, want 1", knowledgeItemID, count)
	}
	if !want && count != 0 {
		t.Errorf("learner_items for %s = %d, want 0", knowledgeItemID, count)
	}
}

func assertCardCount(t *testing.T, database *sql.DB, knowledgeItemID string, want int) {
	t.Helper()
	var count int
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM review_cards WHERE knowledge_item_id = ?`, knowledgeItemID).Scan(&count); err != nil {
		t.Fatalf("count review_cards for %s: %v", knowledgeItemID, err)
	}
	if count != want {
		t.Errorf("review_cards for %s = %d, want %d", knowledgeItemID, count, want)
	}
}

// The schema forbids needs_selection on a word, so a mislabelled sentence being
// corrected has to change both columns at once — two statements would break the CHECK
// constraint in between.
func TestSearchRepositorySetLearnKindWritesBothColumns(t *testing.T) {
	database := openMigratedDB(t)
	ctx := context.Background()
	at := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	seedSentenceCapture(t, database, "cap-kind", "the bread went stale", at)
	repo := NewSearchRepository(database)

	if err := repo.SetLearnKind(ctx, "cap-kind", capture.LearnKindWord, capture.TriageUnseen, at); err != nil {
		t.Fatalf("SetLearnKind() error = %v", err)
	}

	triage, err := repo.LoadTriage(ctx, "cap-kind")
	if err != nil {
		t.Fatalf("LoadTriage() error = %v", err)
	}
	if triage.LearnKind != capture.LearnKindWord || triage.TriageState != capture.TriageUnseen {
		t.Fatalf("triage = %+v, want word/unseen", triage)
	}
}

// A failed lookup leaves the capture with no classification, so the result screen has
// no buttons at all — retry is the only way back for it that does not create a second
// capture for the same text.
func TestSearchRepositoryCreateRetryJobOnlyAfterAFailure(t *testing.T) {
	database := openMigratedDB(t)
	ctx := context.Background()
	at := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	seedSentenceCapture(t, database, "cap-retry", "Allowed by auto mode classifier", at)
	execTestSQL(t, database,
		`INSERT INTO lookup_jobs(id, capture_id, status, error_message, created_at)
VALUES ('job-1', 'cap-retry', 'failed', 'context deadline exceeded', ?)`, at)
	repo := NewSearchRepository(database)

	text, err := repo.CreateRetryJob(ctx, "cap-retry", "job-2", at.Add(time.Minute))
	if err != nil {
		t.Fatalf("CreateRetryJob() error = %v", err)
	}
	if text != "Allowed by auto mode classifier" {
		t.Fatalf("text = %q — the retry must look up the capture's own text", text)
	}

	// The failure stays: the history is a record of what happened, and the list reads
	// the newest job, so the queued one is the status the screen shows.
	items, err := repo.List(ctx, search.ListInput{View: search.ViewAll, Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 || items[0].JobStatus != "queued" {
		t.Fatalf("items = %#v, want one capture showing the queued retry", items)
	}
	var jobCount int
	if err := database.QueryRowContext(ctx,
		`SELECT count(*) FROM lookup_jobs WHERE capture_id = 'cap-retry'`).Scan(&jobCount); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if jobCount != 2 {
		t.Fatalf("job rows = %d, want 2 (the failure is kept)", jobCount)
	}

	// The retry it just queued is not itself a failure, so asking again is refused —
	// otherwise a double click spends two AI calls on the same text.
	if _, err := repo.CreateRetryJob(ctx, "cap-retry", "job-3", at.Add(2*time.Minute)); !errors.Is(err, search.ErrNotRetryable) {
		t.Fatalf("second CreateRetryJob() error = %v, want ErrNotRetryable", err)
	}
}

func TestSearchRepositoryCreateRetryJobRefusesSucceededAndMissing(t *testing.T) {
	database := openMigratedDB(t)
	ctx := context.Background()
	at := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	seedSentenceCapture(t, database, "cap-done", "the bread went stale", at)
	execTestSQL(t, database,
		`INSERT INTO lookup_jobs(id, capture_id, status, created_at)
VALUES ('job-done', 'cap-done', 'done', ?)`, at)
	repo := NewSearchRepository(database)

	// Re-running a successful lookup would overwrite an explanation the user may
	// already have acted on.
	if _, err := repo.CreateRetryJob(ctx, "cap-done", "job-x", at); !errors.Is(err, search.ErrNotRetryable) {
		t.Fatalf("error = %v, want ErrNotRetryable", err)
	}
	if _, err := repo.CreateRetryJob(ctx, "nope", "job-y", at); !errors.Is(err, search.ErrCaptureNotFound) {
		t.Fatalf("error = %v, want ErrCaptureNotFound", err)
	}
}

func TestSearchRepositorySetLearnKindMissingCapture(t *testing.T) {
	repo := NewSearchRepository(openMigratedDB(t))
	err := repo.SetLearnKind(context.Background(), "nope", capture.LearnKindWord, capture.TriageUnseen, time.Now())
	if !errors.Is(err, search.ErrCaptureNotFound) {
		t.Fatalf("error = %v, want ErrCaptureNotFound", err)
	}
}
