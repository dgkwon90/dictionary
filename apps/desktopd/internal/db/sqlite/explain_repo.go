package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"neulsang/desktopd/internal/domain/explain"
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

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO explanations(
id, capture_id, brief_ko, detailed_ko, pronunciation, examples_json, terms_json, difficulty_estimate, category, raw_response_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.New(), captureID, result.BriefKo, result.DetailedKo, result.PronunciationKo, string(examplesJSON), string(termsJSON), result.Difficulty, result.DomainCategory, rawResponseJSON, finishedAt,
	); err != nil {
		return fmt.Errorf("insert explanation: %w", err)
	}
	if err := extractKnowledge(ctx, tx, captureID, result, finishedAt); err != nil {
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

// captureItemRoleSubItem marks a capture_items link derived from an AI sub_item
// (as opposed to the capture's primary term, used by later issues).
const captureItemRoleSubItem = "sub_item"

// extractKnowledge persists PRD Task05: each AI sub_item is upserted into
// knowledge_items (merged by normalized_key+item_type), linked to the capture via
// capture_items, and reflected in learner_items (ask_count/last_asked_at). Each
// sub_item's nested card candidates (#22) are stored against that sub_item's own
// knowledge item, so marking ANY extracted term unknown finds its candidates (not
// just the capture's primary term). It runs inside the SaveSuccess transaction so it
// commits atomically with the explanation.
func extractKnowledge(ctx context.Context, tx *sql.Tx, captureID string, result explain.ExplainResult, seenAt time.Time) error {
	// One lookup counts as a single "ask" per distinct item, so collapse sub_items
	// that repeat the same (normalized_key, item_type) within one result.
	seen := make(map[string]struct{}, len(result.SubItems))
	for _, item := range result.SubItems {
		if item.NormalizedKey == "" || item.ItemType == "" {
			continue
		}
		dedupKey := item.NormalizedKey + "\x00" + item.ItemType
		if _, ok := seen[dedupKey]; ok {
			// Duplicate term: its candidates are intentionally dropped — the first
			// occurrence already stored candidates against the same knowledge item,
			// so the term is never left without cards.
			continue
		}
		seen[dedupKey] = struct{}{}
		// confidence is derived from the sub_item's importance; there is no separate
		// AI confidence signal yet (revisit if the JSON contract adds one).
		knowledgeItemID, err := upsertKnowledgeItem(ctx, tx, item, result.DetectedLanguage, result.DomainCategory, seenAt)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO capture_items(id, capture_id, knowledge_item_id, role, confidence, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			id.New(), captureID, knowledgeItemID, captureItemRoleSubItem, item.Importance, seenAt,
		); err != nil {
			return fmt.Errorf("insert capture item: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO learner_items(id, knowledge_item_id, ask_count, last_asked_at)
VALUES (?, ?, 1, ?)
ON CONFLICT(knowledge_item_id) DO UPDATE SET
  ask_count = ask_count + 1,
  last_asked_at = excluded.last_asked_at`,
			id.New(), knowledgeItemID, seenAt,
		); err != nil {
			return fmt.Errorf("upsert learner item: %w", err)
		}
		if err := insertReviewCardCandidates(ctx, tx, captureID, knowledgeItemID, item.CardCandidates, seenAt); err != nil {
			return err
		}
	}
	return nil
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
			id.New(), captureID, knowledgeItemID, candidate.CardType, candidate.Question, candidate.Answer, candidate.Explanation, createdAt,
		); err != nil {
			return fmt.Errorf("insert review card candidate: %w", err)
		}
	}
	return nil
}

// upsertKnowledgeItem merges a sub_item into knowledge_items keyed by
// (normalized_key, item_type), returning the row id. first_seen_at is preserved on
// merge; the latest explanation refreshes surface_text/pronunciation/meaning and
// last_seen_at. The select-then-insert is safe because db.Open pins the pool to a
// single connection (see db.go); widening it would require an atomic upsert here.
func upsertKnowledgeItem(ctx context.Context, tx *sql.Tx, item explain.SubItem, language, domainCategory string, seenAt time.Time) (string, error) {
	var existingID string
	err := tx.QueryRowContext(
		ctx,
		`SELECT id FROM knowledge_items WHERE normalized_key = ? AND item_type = ?`,
		item.NormalizedKey, item.ItemType,
	).Scan(&existingID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		newID := id.New()
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO knowledge_items(
id, normalized_key, surface_text, item_type, language, pronunciation, meaning_ko, domain_category, first_seen_at, last_seen_at
) VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
			newID, item.NormalizedKey, item.SurfaceText, item.ItemType, language, item.PronunciationKo, item.MeaningKo, domainCategory, seenAt, seenAt,
		); err != nil {
			return "", fmt.Errorf("insert knowledge item: %w", err)
		}
		return newID, nil
	case err != nil:
		return "", fmt.Errorf("select knowledge item: %w", err)
	default:
		// COALESCE keeps a previously stored pronunciation/meaning when the latest
		// explanation omits it (these sub_item fields are not validated as non-empty).
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE knowledge_items SET
surface_text = ?, language = ?,
pronunciation = COALESCE(NULLIF(?, ''), pronunciation),
meaning_ko = COALESCE(NULLIF(?, ''), meaning_ko),
domain_category = NULLIF(?, ''), last_seen_at = ?
WHERE id = ?`,
			item.SurfaceText, language, item.PronunciationKo, item.MeaningKo, domainCategory, seenAt, existingID,
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
