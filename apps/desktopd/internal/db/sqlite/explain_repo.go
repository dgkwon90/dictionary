package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"neulsang/desktopd/internal/domain/explain"
	"neulsang/desktopd/internal/domain/knowledge"
	"neulsang/desktopd/internal/domain/notification"
	"neulsang/desktopd/internal/id"
)

type ExplainRepository struct {
	db *sql.DB
}

func NewExplainRepository(db *sql.DB) *ExplainRepository {
	return &ExplainRepository{db: db}
}

func (r *ExplainRepository) MarkRunning(ctx context.Context, jobID string, startedAt time.Time) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE lookup_jobs SET status = 'running', started_at = ? WHERE id = ?`, startedAt, jobID); err != nil {
		return fmt.Errorf("mark explain job running: %w", err)
	}
	return nil
}

// RecoverStaleJobs marks any lookup_jobs still queued/running as failed. Called
// once at startup (review R-03): a queued/running row can only mean the
// goroutine that would have processed it belonged to a previous process — this
// one just started, so nothing is actually in flight for it. Left alone, such a
// row stays running forever (Quick Search's explanation poll never terminates,
// and the Inbox item never leaves "processing"). This is a safety net for
// non-graceful termination (crash, force-kill); a graceful Quit already lets the
// in-flight goroutine record its own failure via Process's saveFailure path
// before this ever runs. Returns the number of rows recovered.
//
// Assumes at most one desktopd process is active against a given DB file at a
// time — there is no single-instance guard (codex review, RW-03). If a second
// process somehow started while a first genuinely still had a job in flight,
// this could stomp that live row to "failed" prematurely; it self-heals,
// because the real goroutine's eventual SaveSuccess/saveFailure unconditionally
// overwrites the same row by jobID once it finishes — worst case is a
// transient wrong status, not permanent corruption or data loss.
func (r *ExplainRepository) RecoverStaleJobs(ctx context.Context, now time.Time) (int64, error) {
	const staleJobMessage = "interrupted by shutdown or crash before a previous run could finish"
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE lookup_jobs SET status = 'failed', error_message = ?, finished_at = ? WHERE status IN ('queued', 'running')`,
		staleJobMessage, now,
	)
	if err != nil {
		return 0, fmt.Errorf("recover stale lookup jobs: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("recover stale lookup jobs: rows affected: %w", err)
	}
	return n, nil
}

func (r *ExplainRepository) SaveSuccess(ctx context.Context, jobID, captureID string, result explain.ExplainResult, rawResponseJSON string, finishedAt time.Time) (resultErr error) {
	examplesJSON, err := json.Marshal(result.Examples)
	if err != nil {
		return fmt.Errorf("marshal examples: %w", err)
	}
	termsJSON, err := json.Marshal(result.SubItems)
	if err != nil {
		return fmt.Errorf("marshal sub items: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin explain success transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, tx.Rollback())
		}
	}()

	// The capture's own text decides word-vs-sentence together with the AI's label,
	// and it is also what sub_item character offsets are measured against.
	var captureText string
	if err := tx.QueryRowContext(ctx, `SELECT selected_text FROM captures WHERE id = ?`, captureID).Scan(&captureText); err != nil {
		return fmt.Errorf("select capture text: %w", err)
	}
	learnKind := explain.LearnKind(result.InputType, captureText)

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO explanations(
id, capture_id, brief_ko, detailed_ko, pronunciation, examples_json, terms_json, difficulty_estimate, category, raw_response_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.New(), captureID, result.BriefKo, result.DetailedKo, result.PronunciationKo, string(examplesJSON), string(termsJSON), result.Difficulty, result.DomainCategory, rawResponseJSON, utc(finishedAt),
	); err != nil {
		return fmt.Errorf("insert explanation: %w", err)
	}
	// Record the classification on the capture itself. Previously input_type was parsed,
	// validated, and then dropped — it survived only inside raw_response_json, so nothing
	// downstream could branch on word vs sentence.
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE captures SET input_type = ?, learn_kind = ?, detected_lang = NULLIF(?, ''), updated_at = ? WHERE id = ?`,
		result.InputType, learnKind, result.DetectedLanguage, utc(finishedAt), captureID,
	); err != nil {
		return fmt.Errorf("record capture classification: %w", err)
	}
	if err := extractKnowledge(ctx, tx, captureID, captureText, result, finishedAt); err != nil {
		return err
	}
	// Enqueue the "result ready" notification atomically with the explanation
	// (ADR-0008): one per capture (dedup_key = captureID), short TTL so a stale result
	// from a previous session does not notify after a restart.
	if err := insertNotification(ctx, tx, notification.Notification{
		Kind:      notification.KindResultReady,
		DedupKey:  captureID,
		Title:     "검색 결과 준비 완료",
		Body:      result.BriefKo,
		Route:     "Inbox",
		PayloadID: captureID,
		CreatedAt: finishedAt,
		ExpiresAt: finishedAt.Add(notification.ResultReadyTTL),
	}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE lookup_jobs SET status = 'done', finished_at = ? WHERE id = ?`, finishedAt, jobID); err != nil {
		return fmt.Errorf("mark explain job done: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit explain success transaction: %w", err)
	}
	return nil
}

// capture_items roles. sub_item is a term the AI found inside the capture;
// sentence_self links a sentence capture to the knowledge item for the sentence
// itself (created when the user completes word selection).
const (
	captureItemRoleSubItem      = "sub_item"
	captureItemRoleSentenceSelf = "sentence_self"
)

// extractKnowledge records what the AI found in a capture: each sub_item becomes (or
// merges into) a knowledge_items row and is linked to the capture via capture_items,
// along with its card candidates.
//
// What it deliberately does NOT do is create learner_items. In the previous design a
// term became part of the learning list the moment the AI mentioned it, which meant
// the list filled up with words the user had never said they did not know. Learning
// registration now happens only when the user acts on the result — "학습할래요" for a
// word, or completing word selection for a sentence. Existing learner rows still get
// their ask_count bumped here, because for something already being learned, seeing it
// again is genuinely another encounter.
func extractKnowledge(ctx context.Context, tx *sql.Tx, captureID, captureText string, result explain.ExplainResult, seenAt time.Time) error {
	// Collapse sub_items that normalize to the same key within one result so a single
	// lookup never counts as two encounters.
	seen := make(map[string]struct{}, len(result.SubItems))
	for _, item := range result.SubItems {
		// The key is derived from the surface text rather than taken from the AI —
		// see knowledge.NormalizeKey for why the AI's own key is not trustworthy as
		// an identity.
		normalizedKey := knowledge.NormalizeKey(item.SurfaceText)
		if normalizedKey == "" {
			continue
		}
		if _, ok := seen[normalizedKey]; ok {
			// Duplicate term: its candidates are intentionally dropped — the first
			// occurrence already stored candidates against the same knowledge item,
			// so the term is never left without cards.
			continue
		}
		seen[normalizedKey] = struct{}{}
		// confidence is derived from the sub_item's importance; there is no separate
		// AI confidence signal yet (revisit if the JSON contract adds one).
		knowledgeItemID, err := upsertKnowledgeItem(ctx, tx, item, normalizedKey, result.DetectedLanguage, result.DomainCategory, seenAt)
		if err != nil {
			return err
		}
		charStart, charEnd, found := findSpan(captureText, item.SurfaceText)
		if err := upsertCaptureItem(ctx, tx, captureID, knowledgeItemID, captureItemRoleSubItem, item.Importance, charStart, charEnd, found, seenAt); err != nil {
			return err
		}
		// Update-only: a learner row exists exactly when the user has committed to
		// learning this item, and re-encountering it must not create one.
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE learner_items SET ask_count = ask_count + 1, last_asked_at = ?, updated_at = ?
WHERE knowledge_item_id = ?`,
			utc(seenAt), utc(seenAt), knowledgeItemID,
		); err != nil {
			return fmt.Errorf("bump learner item ask count: %w", err)
		}
		if err := insertReviewCardCandidates(ctx, tx, captureID, knowledgeItemID, item.CardCandidates, seenAt); err != nil {
			return err
		}
	}
	return nil
}

// upsertCaptureItem links a capture to a knowledge item, or refreshes that link if the
// same capture is explained again. UNIQUE(capture_id, knowledge_item_id, role) makes the
// link idempotent; selected_at is left alone so re-running a lookup never erases the
// user's word choices.
func upsertCaptureItem(ctx context.Context, tx *sql.Tx, captureID, knowledgeItemID, role string, confidence float64, charStart, charEnd int, hasSpan bool, at time.Time) error {
	var start, end any
	if hasSpan {
		start, end = charStart, charEnd
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO capture_items(id, capture_id, knowledge_item_id, role, confidence, char_start, char_end, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(capture_id, knowledge_item_id, role) DO UPDATE SET
  confidence = excluded.confidence,
  char_start = COALESCE(excluded.char_start, char_start),
  char_end = COALESCE(excluded.char_end, char_end),
  updated_at = excluded.updated_at`,
		id.New(), captureID, knowledgeItemID, role, confidence, start, end, utc(at), utc(at),
	); err != nil {
		return fmt.Errorf("upsert capture item: %w", err)
	}
	return nil
}

// findSpan locates a term inside the capture text so the UI can highlight it and a
// cloze card can blank it out. Offsets are in runes, not bytes, because the frontend
// indexes strings by character.
//
// The match is case-insensitive because the AI returns terms in dictionary form
// ("stale") while the sentence may capitalize them. Only the first occurrence is
// recorded; a user who means a later occurrence selects it explicitly, which stores
// its own offsets.
func findSpan(text, surface string) (start, end int, ok bool) {
	if text == "" || surface == "" {
		return 0, 0, false
	}
	haystack := []rune(strings.ToLower(text))
	needle := []rune(strings.ToLower(surface))
	if len(needle) > len(haystack) {
		return 0, 0, false
	}
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if string(haystack[index:index+len(needle)]) == string(needle) {
			return index, index + len(needle), true
		}
	}
	return 0, 0, false
}

// insertReviewCardCandidates persists a sub_item's nested review_card_candidates
// (PRD §12.1, #22) against that sub_item's knowledge item, so #9 can build review
// cards from them when the term is marked unknown.
func insertReviewCardCandidates(ctx context.Context, tx *sql.Tx, captureID, knowledgeItemID string, candidates []explain.ReviewCardCandidate, createdAt time.Time) error {
	for _, candidate := range candidates {
		if candidate.Question == "" || candidate.Answer == "" {
			continue
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO review_card_candidates(id, capture_id, knowledge_item_id, card_type, question, answer, explanation, created_at)
VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
			id.New(), captureID, knowledgeItemID, candidate.CardType, candidate.Question, candidate.Answer, candidate.Explanation, utc(createdAt),
		); err != nil {
			return fmt.Errorf("insert review card candidate: %w", err)
		}
	}
	return nil
}

// upsertKnowledgeItem merges a sub_item into knowledge_items keyed by
// (normalized_key, learn_kind), returning the row id. first_seen_at is preserved on
// merge; the latest explanation refreshes surface_text/pronunciation/meaning and
// last_seen_at. The select-then-insert is safe because db.Open pins the pool to a
// single connection (see db.go); widening it would require an atomic upsert here.
//
// The AI's item_type is stored but is not part of the key: it is a five-way guess
// that can come back as "word" for one lookup and "term" for the next, which would
// split one term across two rows and scatter its counts.
func upsertKnowledgeItem(ctx context.Context, tx *sql.Tx, item explain.SubItem, normalizedKey, language, domainCategory string, seenAt time.Time) (string, error) {
	var existingID string
	err := tx.QueryRowContext(
		ctx,
		`SELECT id FROM knowledge_items WHERE normalized_key = ? AND learn_kind = ?`,
		normalizedKey, explain.LearnKindWord,
	).Scan(&existingID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		newID := id.New()
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO knowledge_items(
id, normalized_key, surface_text, learn_kind, item_type, language, pronunciation, meaning_ko, description_ko, domain_category, first_seen_at, last_seen_at, updated_at
) VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?)`,
			newID, normalizedKey, item.SurfaceText, explain.LearnKindWord, item.ItemType, language,
			item.PronunciationKo, item.MeaningKo, item.DescriptionKo, domainCategory, utc(seenAt), utc(seenAt), utc(seenAt),
		); err != nil {
			return "", fmt.Errorf("insert knowledge item: %w", err)
		}
		return newID, nil
	case err != nil:
		return "", fmt.Errorf("select knowledge item: %w", err)
	default:
		// COALESCE keeps a previously stored pronunciation/meaning/description when the
		// latest explanation omits it (these sub_item fields are not validated as non-empty).
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE knowledge_items SET
surface_text = ?, language = ?,
item_type = COALESCE(NULLIF(?, ''), item_type),
pronunciation = COALESCE(NULLIF(?, ''), pronunciation),
meaning_ko = COALESCE(NULLIF(?, ''), meaning_ko),
description_ko = COALESCE(NULLIF(?, ''), description_ko),
domain_category = NULLIF(?, ''), last_seen_at = ?, updated_at = ?
WHERE id = ?`,
			item.SurfaceText, language, item.ItemType, item.PronunciationKo, item.MeaningKo, item.DescriptionKo,
			domainCategory, utc(seenAt), utc(seenAt), existingID,
		); err != nil {
			return "", fmt.Errorf("update knowledge item: %w", err)
		}
		return existingID, nil
	}
}

func (r *ExplainRepository) SaveFailure(ctx context.Context, jobID string, errMessage string, finishedAt time.Time) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE lookup_jobs SET status = 'failed', error_message = ?, finished_at = ? WHERE id = ?`, errMessage, finishedAt, jobID); err != nil {
		return fmt.Errorf("mark explain job failed: %w", err)
	}
	return nil
}

func (r *ExplainRepository) GetSnapshot(ctx context.Context, captureID string) (explain.Snapshot, error) {
	var status string
	var errorMessage sql.NullString
	err := r.db.QueryRowContext(
		ctx,
		`SELECT status, error_message FROM lookup_jobs WHERE capture_id = ? ORDER BY created_at DESC LIMIT 1`,
		captureID,
	).Scan(&status, &errorMessage)
	switch {
	case err == nil:
	case errors.Is(err, sql.ErrNoRows):
		return explain.Snapshot{}, explain.ErrCaptureNotFound
	default:
		return explain.Snapshot{}, fmt.Errorf("select latest lookup job: %w", err)
	}

	snapshot := explain.Snapshot{Status: status}
	if errorMessage.Valid {
		snapshot.ErrorMessage = errorMessage.String
	}
	if status != "done" {
		return snapshot, nil
	}

	var result explain.ExplainResult
	var examplesJSON string
	var termsJSON string
	if err := r.db.QueryRowContext(
		ctx,
		`SELECT brief_ko, detailed_ko, pronunciation, examples_json, terms_json, difficulty_estimate, category
FROM explanations WHERE capture_id = ?`,
		captureID,
	).Scan(&result.BriefKo, &result.DetailedKo, &result.PronunciationKo, &examplesJSON, &termsJSON, &result.Difficulty, &result.DomainCategory); err != nil {
		return explain.Snapshot{}, fmt.Errorf("select explanation: %w", err)
	}
	if err := json.Unmarshal([]byte(examplesJSON), &result.Examples); err != nil {
		return explain.Snapshot{}, fmt.Errorf("unmarshal examples: %w", err)
	}
	if err := json.Unmarshal([]byte(termsJSON), &result.SubItems); err != nil {
		return explain.Snapshot{}, fmt.Errorf("unmarshal sub items: %w", err)
	}
	// InputType, DetectedLanguage, and ReviewCardCandidates are preserved only
	// in raw_response_json in backlog #4 and are not projected into this snapshot.
	snapshot.Result = &result
	return snapshot, nil
}
