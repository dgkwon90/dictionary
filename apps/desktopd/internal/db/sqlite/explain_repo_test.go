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
