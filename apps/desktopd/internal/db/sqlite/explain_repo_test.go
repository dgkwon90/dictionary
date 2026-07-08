package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/explain"
)

func TestExplainRepositoryMarkRunning(t *testing.T) {
	database := openMigratedDB(t)
	insertCaptureFixture(t, database, "capture-1", "job-1", "queued")
	repo := NewExplainRepository(database)

	if err := repo.MarkRunning(context.Background(), "job-1", time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}

	var status string
	var startedAt sql.NullTime
	if err := database.QueryRowContext(context.Background(), `SELECT status, started_at FROM lookup_jobs WHERE id = ?`, "job-1").Scan(&status, &startedAt); err != nil {
		t.Fatalf("query lookup job: %v", err)
	}
	if status != "running" || !startedAt.Valid {
		t.Fatalf("status=%q startedAt=%#v", status, startedAt)
	}
}

func TestExplainRepositorySaveSuccess(t *testing.T) {
	database := openMigratedDB(t)
	insertCaptureFixture(t, database, "capture-1", "job-1", "running")
	repo := NewExplainRepository(database)
	result := repositoryExplainResult()

	if err := repo.SaveSuccess(context.Background(), "job-1", "capture-1", result, `{"raw":true}`, time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SaveSuccess() error = %v", err)
	}

	var count int
	var briefKo string
	var pronunciation string
	var examplesJSON string
	var termsJSON string
	var difficulty float64
	var category string
	var rawResponseJSON string
	if err := database.QueryRowContext(
		context.Background(),
		`SELECT count(*), brief_ko, pronunciation, examples_json, terms_json, difficulty_estimate, category, raw_response_json FROM explanations WHERE capture_id = ?`,
		"capture-1",
	).Scan(&count, &briefKo, &pronunciation, &examplesJSON, &termsJSON, &difficulty, &category, &rawResponseJSON); err != nil {
		t.Fatalf("query explanations: %v", err)
	}
	if count != 1 || briefKo != result.BriefKo || pronunciation != result.PronunciationKo || difficulty != result.Difficulty || category != result.DomainCategory || rawResponseJSON != `{"raw":true}` {
		t.Fatalf("explanation row mismatch count=%d brief=%q pronunciation=%q difficulty=%f category=%q raw=%q", count, briefKo, pronunciation, difficulty, category, rawResponseJSON)
	}
	if examplesJSON == "" || termsJSON == "" {
		t.Fatalf("json columns examples=%q terms=%q", examplesJSON, termsJSON)
	}

	var status string
	var finishedAt sql.NullTime
	if err := database.QueryRowContext(context.Background(), `SELECT status, finished_at FROM lookup_jobs WHERE id = ?`, "job-1").Scan(&status, &finishedAt); err != nil {
		t.Fatalf("query lookup job: %v", err)
	}
	if status != "done" || !finishedAt.Valid {
		t.Fatalf("status=%q finishedAt=%#v", status, finishedAt)
	}
}

func TestExplainRepositorySaveSuccessExtractsKnowledge(t *testing.T) {
	database := openMigratedDB(t)
	insertCaptureFixture(t, database, "capture-1", "job-1", "running")
	repo := NewExplainRepository(database)
	result := repositoryExplainResult()
	now := time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)

	if err := repo.SaveSuccess(context.Background(), "job-1", "capture-1", result, `{"raw":true}`, now); err != nil {
		t.Fatalf("SaveSuccess() error = %v", err)
	}

	var knowledgeID string
	var surface, itemType, language, meaningKo, domainCategory string
	var pronunciation sql.NullString
	var firstSeen, lastSeen time.Time
	if err := database.QueryRowContext(
		context.Background(),
		`SELECT id, surface_text, item_type, language, meaning_ko, domain_category, pronunciation, first_seen_at, last_seen_at
FROM knowledge_items WHERE normalized_key = ? AND item_type = ?`,
		"hello", "word",
	).Scan(&knowledgeID, &surface, &itemType, &language, &meaningKo, &domainCategory, &pronunciation, &firstSeen, &lastSeen); err != nil {
		t.Fatalf("query knowledge_items: %v", err)
	}
	if surface != "hello" || itemType != "word" || language != "en" || meaningKo != "meaning" || domainCategory != "general" {
		t.Fatalf("knowledge row mismatch surface=%q item_type=%q language=%q meaning=%q domain=%q", surface, itemType, language, meaningKo, domainCategory)
	}
	if !pronunciation.Valid || pronunciation.String != "pronunciation" {
		t.Fatalf("knowledge pronunciation = %#v", pronunciation)
	}
	if !firstSeen.Equal(now) || !lastSeen.Equal(now) {
		t.Fatalf("knowledge seen firstSeen=%v lastSeen=%v want %v", firstSeen, lastSeen, now)
	}

	var captureItemKnowledgeID, role string
	var confidence float64
	if err := database.QueryRowContext(
		context.Background(),
		`SELECT knowledge_item_id, role, confidence FROM capture_items WHERE capture_id = ?`,
		"capture-1",
	).Scan(&captureItemKnowledgeID, &role, &confidence); err != nil {
		t.Fatalf("query capture_items: %v", err)
	}
	if captureItemKnowledgeID != knowledgeID || role != "sub_item" || confidence != 0.5 {
		t.Fatalf("capture_item mismatch knowledge_id=%q role=%q confidence=%f", captureItemKnowledgeID, role, confidence)
	}

	var askCount int
	var lastAsked sql.NullTime
	if err := database.QueryRowContext(
		context.Background(),
		`SELECT ask_count, last_asked_at FROM learner_items WHERE knowledge_item_id = ?`,
		knowledgeID,
	).Scan(&askCount, &lastAsked); err != nil {
		t.Fatalf("query learner_items: %v", err)
	}
	if askCount != 1 || !lastAsked.Valid || !lastAsked.Time.Equal(now) {
		t.Fatalf("learner row mismatch ask_count=%d last_asked=%#v", askCount, lastAsked)
	}
}

func TestExplainRepositorySaveSuccessMergesKnowledge(t *testing.T) {
	database := openMigratedDB(t)
	insertCaptureFixture(t, database, "capture-1", "job-1", "running")
	insertCaptureFixture(t, database, "capture-2", "job-2", "running")
	repo := NewExplainRepository(database)
	result := repositoryExplainResult()
	firstAt := time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)
	secondAt := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)

	if err := repo.SaveSuccess(context.Background(), "job-1", "capture-1", result, `{"raw":1}`, firstAt); err != nil {
		t.Fatalf("first SaveSuccess() error = %v", err)
	}
	if err := repo.SaveSuccess(context.Background(), "job-2", "capture-2", result, `{"raw":2}`, secondAt); err != nil {
		t.Fatalf("second SaveSuccess() error = %v", err)
	}

	var knowledgeCount int
	if err := database.QueryRowContext(context.Background(), `SELECT count(*) FROM knowledge_items`).Scan(&knowledgeCount); err != nil {
		t.Fatalf("count knowledge_items: %v", err)
	}
	if knowledgeCount != 1 {
		t.Fatalf("knowledge_items count = %d, want 1 (merged)", knowledgeCount)
	}

	var knowledgeID string
	var firstSeen, lastSeen time.Time
	if err := database.QueryRowContext(
		context.Background(),
		`SELECT id, first_seen_at, last_seen_at FROM knowledge_items WHERE normalized_key = ? AND item_type = ?`,
		"hello", "word",
	).Scan(&knowledgeID, &firstSeen, &lastSeen); err != nil {
		t.Fatalf("query knowledge_items: %v", err)
	}
	if !firstSeen.Equal(firstAt) {
		t.Fatalf("first_seen_at = %v, want unchanged %v", firstSeen, firstAt)
	}
	if !lastSeen.Equal(secondAt) {
		t.Fatalf("last_seen_at = %v, want advanced %v", lastSeen, secondAt)
	}

	var askCount int
	if err := database.QueryRowContext(context.Background(), `SELECT ask_count FROM learner_items WHERE knowledge_item_id = ?`, knowledgeID).Scan(&askCount); err != nil {
		t.Fatalf("query learner_items: %v", err)
	}
	if askCount != 2 {
		t.Fatalf("ask_count = %d, want 2", askCount)
	}

	var captureItemCount int
	if err := database.QueryRowContext(context.Background(), `SELECT count(*) FROM capture_items WHERE knowledge_item_id = ?`, knowledgeID).Scan(&captureItemCount); err != nil {
		t.Fatalf("count capture_items: %v", err)
	}
	if captureItemCount != 2 {
		t.Fatalf("capture_items count = %d, want 2", captureItemCount)
	}
}

func TestExplainRepositorySaveSuccessPersistsCandidates(t *testing.T) {
	database := openMigratedDB(t)
	insertCaptureFixture(t, database, "capture-1", "job-1", "running")
	repo := NewExplainRepository(database)
	result := repositoryExplainResult()
	// A higher-importance sub_item should become the candidate's anchor.
	result.SubItems = append(result.SubItems, explain.SubItem{
		SurfaceText: "world", NormalizedKey: "world", ItemType: "word", MeaningKo: "세계", Importance: 0.9,
	})
	now := time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)

	if err := repo.SaveSuccess(context.Background(), "job-1", "capture-1", result, `{"raw":true}`, now); err != nil {
		t.Fatalf("SaveSuccess() error = %v", err)
	}

	var primaryID string
	if err := database.QueryRowContext(context.Background(), `SELECT id FROM knowledge_items WHERE normalized_key = ?`, "world").Scan(&primaryID); err != nil {
		t.Fatalf("query primary knowledge item: %v", err)
	}

	var captureID, cardType, question, answer string
	var knowledgeItemID sql.NullString
	var explanation sql.NullString
	if err := database.QueryRowContext(
		context.Background(),
		`SELECT capture_id, knowledge_item_id, card_type, question, answer, explanation FROM review_card_candidates`,
	).Scan(&captureID, &knowledgeItemID, &cardType, &question, &answer, &explanation); err != nil {
		t.Fatalf("query review_card_candidates: %v", err)
	}
	if captureID != "capture-1" || cardType != "meaning" || question == "" || answer == "" {
		t.Fatalf("candidate row mismatch capture=%q type=%q q=%q a=%q", captureID, cardType, question, answer)
	}
	if !knowledgeItemID.Valid || knowledgeItemID.String != primaryID {
		t.Fatalf("candidate anchored to %#v, want primary %q", knowledgeItemID, primaryID)
	}
}

func TestExplainRepositorySaveSuccessDeduplicatesSubItems(t *testing.T) {
	database := openMigratedDB(t)
	insertCaptureFixture(t, database, "capture-1", "job-1", "running")
	repo := NewExplainRepository(database)
	result := repositoryExplainResult()
	// Same (normalized_key, item_type) repeated within one lookup must count once.
	result.SubItems = append(result.SubItems, result.SubItems[0])
	now := time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)

	if err := repo.SaveSuccess(context.Background(), "job-1", "capture-1", result, `{"raw":true}`, now); err != nil {
		t.Fatalf("SaveSuccess() error = %v", err)
	}

	var knowledgeCount, captureItemCount, askCount int
	if err := database.QueryRowContext(context.Background(), `SELECT count(*) FROM knowledge_items`).Scan(&knowledgeCount); err != nil {
		t.Fatalf("count knowledge_items: %v", err)
	}
	if err := database.QueryRowContext(context.Background(), `SELECT count(*) FROM capture_items WHERE capture_id = ?`, "capture-1").Scan(&captureItemCount); err != nil {
		t.Fatalf("count capture_items: %v", err)
	}
	if err := database.QueryRowContext(context.Background(), `SELECT ask_count FROM learner_items`).Scan(&askCount); err != nil {
		t.Fatalf("query learner_items: %v", err)
	}
	if knowledgeCount != 1 || captureItemCount != 1 || askCount != 1 {
		t.Fatalf("dedup failed knowledge=%d capture_items=%d ask_count=%d, want 1/1/1", knowledgeCount, captureItemCount, askCount)
	}
}

func TestExplainRepositorySaveSuccessMergeKeepsExistingPronunciation(t *testing.T) {
	database := openMigratedDB(t)
	insertCaptureFixture(t, database, "capture-1", "job-1", "running")
	insertCaptureFixture(t, database, "capture-2", "job-2", "running")
	repo := NewExplainRepository(database)

	first := repositoryExplainResult()
	if err := repo.SaveSuccess(context.Background(), "job-1", "capture-1", first, `{"raw":1}`, time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("first SaveSuccess() error = %v", err)
	}

	// Second lookup of the same term returns no pronunciation/meaning; the stored
	// values must be preserved, not wiped.
	second := repositoryExplainResult()
	second.SubItems[0].PronunciationKo = ""
	second.SubItems[0].MeaningKo = ""
	if err := repo.SaveSuccess(context.Background(), "job-2", "capture-2", second, `{"raw":2}`, time.Date(2026, 7, 8, 2, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("second SaveSuccess() error = %v", err)
	}

	var pronunciation, meaningKo sql.NullString
	if err := database.QueryRowContext(
		context.Background(),
		`SELECT pronunciation, meaning_ko FROM knowledge_items WHERE normalized_key = ? AND item_type = ?`,
		"hello", "word",
	).Scan(&pronunciation, &meaningKo); err != nil {
		t.Fatalf("query knowledge_items: %v", err)
	}
	if !pronunciation.Valid || pronunciation.String != "pronunciation" || !meaningKo.Valid || meaningKo.String != "meaning" {
		t.Fatalf("merge wiped data pronunciation=%#v meaning=%#v", pronunciation, meaningKo)
	}
}

func TestExplainRepositorySaveFailure(t *testing.T) {
	database := openMigratedDB(t)
	insertCaptureFixture(t, database, "capture-1", "job-1", "running")
	repo := NewExplainRepository(database)

	if err := repo.SaveFailure(context.Background(), "job-1", "provider failed", time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SaveFailure() error = %v", err)
	}

	var status string
	var errorMessage string
	var finishedAt sql.NullTime
	if err := database.QueryRowContext(context.Background(), `SELECT status, error_message, finished_at FROM lookup_jobs WHERE id = ?`, "job-1").Scan(&status, &errorMessage, &finishedAt); err != nil {
		t.Fatalf("query lookup job: %v", err)
	}
	if status != "failed" || errorMessage != "provider failed" || !finishedAt.Valid {
		t.Fatalf("status=%q error=%q finishedAt=%#v", status, errorMessage, finishedAt)
	}
}

func TestExplainRepositoryGetSnapshot(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		database := openMigratedDB(t)
		repo := NewExplainRepository(database)
		_, err := repo.GetSnapshot(context.Background(), "missing")
		if !errors.Is(err, explain.ErrCaptureNotFound) {
			t.Fatalf("GetSnapshot() error = %v, want ErrCaptureNotFound", err)
		}
	})

	t.Run("queued", func(t *testing.T) {
		database := openMigratedDB(t)
		insertCaptureFixture(t, database, "capture-1", "job-1", "queued")
		repo := NewExplainRepository(database)
		snapshot, err := repo.GetSnapshot(context.Background(), "capture-1")
		if err != nil {
			t.Fatalf("GetSnapshot() error = %v", err)
		}
		if snapshot.Status != "queued" || snapshot.Result != nil {
			t.Fatalf("snapshot = %#v", snapshot)
		}
	})

	t.Run("done", func(t *testing.T) {
		database := openMigratedDB(t)
		insertCaptureFixture(t, database, "capture-1", "job-1", "running")
		repo := NewExplainRepository(database)
		result := repositoryExplainResult()
		if err := repo.SaveSuccess(context.Background(), "job-1", "capture-1", result, `{"raw":true}`, time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)); err != nil {
			t.Fatalf("SaveSuccess() error = %v", err)
		}
		snapshot, err := repo.GetSnapshot(context.Background(), "capture-1")
		if err != nil {
			t.Fatalf("GetSnapshot() error = %v", err)
		}
		if snapshot.Status != "done" || snapshot.Result == nil {
			t.Fatalf("snapshot = %#v", snapshot)
		}
		if snapshot.Result.BriefKo != result.BriefKo || len(snapshot.Result.Examples) != 1 || snapshot.Result.Examples[0].English != "hello" || len(snapshot.Result.SubItems) != 1 || snapshot.Result.SubItems[0].SurfaceText != "hello" {
			t.Fatalf("result = %#v", snapshot.Result)
		}
	})
}

func TestExplainRepositorySaveSuccessDuplicateRollsBack(t *testing.T) {
	database := openMigratedDB(t)
	insertCaptureFixture(t, database, "capture-1", "job-1", "running")
	insertCaptureFixture(t, database, "capture-2", "job-2", "running")
	repo := NewExplainRepository(database)
	result := repositoryExplainResult()
	if err := repo.SaveSuccess(context.Background(), "job-1", "capture-1", result, `{"raw":true}`, time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("first SaveSuccess() error = %v", err)
	}

	err := repo.SaveSuccess(context.Background(), "job-2", "capture-1", result, `{"raw":true}`, time.Date(2026, 7, 7, 3, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("second SaveSuccess() error = nil, want duplicate error")
	}

	var status string
	if err := database.QueryRowContext(context.Background(), `SELECT status FROM lookup_jobs WHERE id = ?`, "job-2").Scan(&status); err != nil {
		t.Fatalf("query job-2: %v", err)
	}
	if status != "running" {
		t.Fatalf("job-2 status = %q, want running", status)
	}
}

func insertCaptureFixture(t *testing.T, database *sql.DB, captureID, jobID, status string) {
	t.Helper()
	createdAt := time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(
		context.Background(),
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at, inbox_status) VALUES (?, ?, ?, ?, ?, ?)`,
		captureID, "hello", "manual", captureID+"-hash", createdAt, "new",
	); err != nil {
		t.Fatalf("insert capture fixture: %v", err)
	}
	if _, err := database.ExecContext(
		context.Background(),
		`INSERT INTO lookup_jobs(id, capture_id, status, created_at) VALUES (?, ?, ?, ?)`,
		jobID, captureID, status, createdAt,
	); err != nil {
		t.Fatalf("insert lookup job fixture: %v", err)
	}
}

func repositoryExplainResult() explain.ExplainResult {
	return explain.ExplainResult{
		InputType:        "word",
		DetectedLanguage: "en",
		BriefKo:          "brief",
		DetailedKo:       "detailed",
		PronunciationKo:  "pronunciation",
		DomainCategory:   "general",
		Difficulty:       0.5,
		Examples:         []explain.Example{{English: "hello", Korean: "안녕", Note: "note"}},
		SubItems: []explain.SubItem{{
			SurfaceText:     "hello",
			NormalizedKey:   "hello",
			ItemType:        "word",
			MeaningKo:       "meaning",
			PronunciationKo: "pronunciation",
			Importance:      0.5,
		}},
		ReviewCardCandidates: []explain.ReviewCardCandidate{{CardType: "meaning", Question: "q", Answer: "a", Explanation: "e"}},
	}
}
