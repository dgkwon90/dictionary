package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"neulsang/desktopd/internal/domain/backup"
)

type BackupRepository struct {
	db *sql.DB
}

var _ backup.Repository = (*BackupRepository)(nil)

func NewBackupRepository(db *sql.DB) *BackupRepository {
	return &BackupRepository{db: db}
}

func (r *BackupRepository) Export(ctx context.Context) (*backup.Snapshot, error) {
	snapshot := &backup.Snapshot{}
	var err error
	if snapshot.KnowledgeItems, err = r.exportKnowledgeItems(ctx); err != nil {
		return nil, err
	}
	if snapshot.Captures, err = r.exportCaptures(ctx); err != nil {
		return nil, err
	}
	if snapshot.Explanations, err = r.exportExplanations(ctx); err != nil {
		return nil, err
	}
	if snapshot.CaptureItems, err = r.exportCaptureItems(ctx); err != nil {
		return nil, err
	}
	if snapshot.LearnerItems, err = r.exportLearnerItems(ctx); err != nil {
		return nil, err
	}
	if snapshot.ReviewCards, err = r.exportReviewCards(ctx); err != nil {
		return nil, err
	}
	if snapshot.ReviewLogs, err = r.exportReviewLogs(ctx); err != nil {
		return nil, err
	}
	if snapshot.LookupJobs, err = r.exportLookupJobs(ctx); err != nil {
		return nil, err
	}
	if snapshot.ReviewCardCandidates, err = r.exportReviewCardCandidates(ctx); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (r *BackupRepository) exportKnowledgeItems(ctx context.Context) (items []backup.KnowledgeItemRow, resultErr error) {
	items = make([]backup.KnowledgeItemRow, 0)
	rows, err := r.db.QueryContext(ctx, `SELECT
id, normalized_key, surface_text, item_type, language, pos, pronunciation, meaning_ko, description_ko, domain_category, first_seen_at, last_seen_at
FROM knowledge_items
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("select backup knowledge_items: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close backup knowledge_items rows: %w", err)
		}
	}()

	for rows.Next() {
		var row backup.KnowledgeItemRow
		if err := rows.Scan(
			&row.ID, &row.NormalizedKey, &row.SurfaceText, &row.ItemType, &row.Language,
			&row.Pos, &row.Pronunciation, &row.MeaningKo, &row.DescriptionKo, &row.DomainCategory,
			&row.FirstSeenAt, &row.LastSeenAt,
		); err != nil {
			return nil, fmt.Errorf("scan backup knowledge_item: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup knowledge_items: %w", err)
	}
	return items, nil
}

func (r *BackupRepository) exportCaptures(ctx context.Context) (captures []backup.CaptureRow, resultErr error) {
	captures = make([]backup.CaptureRow, 0)
	rows, err := r.db.QueryContext(ctx, `SELECT
id, source_app, source_type, source_title, source_url, selected_text, detected_lang, input_mode, text_hash, created_at, inbox_status
FROM captures
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("select backup captures: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close backup captures rows: %w", err)
		}
	}()

	for rows.Next() {
		var row backup.CaptureRow
		if err := rows.Scan(
			&row.ID, &row.SourceApp, &row.SourceType, &row.SourceTitle, &row.SourceURL,
			&row.SelectedText, &row.DetectedLang, &row.InputMode, &row.TextHash, &row.CreatedAt, &row.InboxStatus,
		); err != nil {
			return nil, fmt.Errorf("scan backup capture: %w", err)
		}
		captures = append(captures, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup captures: %w", err)
	}
	return captures, nil
}

func (r *BackupRepository) exportExplanations(ctx context.Context) (explanations []backup.ExplanationRow, resultErr error) {
	explanations = make([]backup.ExplanationRow, 0)
	rows, err := r.db.QueryContext(ctx, `SELECT
id, capture_id, brief_ko, detailed_ko, pronunciation, examples_json, terms_json, difficulty_estimate, category, raw_response_json, created_at
FROM explanations
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("select backup explanations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close backup explanations rows: %w", err)
		}
	}()

	for rows.Next() {
		var row backup.ExplanationRow
		if err := rows.Scan(
			&row.ID, &row.CaptureID, &row.BriefKo, &row.DetailedKo, &row.Pronunciation,
			&row.ExamplesJSON, &row.TermsJSON, &row.DifficultyEstimate, &row.Category, &row.RawResponseJSON, &row.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan backup explanation: %w", err)
		}
		explanations = append(explanations, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup explanations: %w", err)
	}
	return explanations, nil
}

func (r *BackupRepository) exportCaptureItems(ctx context.Context) (items []backup.CaptureItemRow, resultErr error) {
	items = make([]backup.CaptureItemRow, 0)
	rows, err := r.db.QueryContext(ctx, `SELECT
id, capture_id, knowledge_item_id, role, confidence, created_at
FROM capture_items
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("select backup capture_items: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close backup capture_items rows: %w", err)
		}
	}()

	for rows.Next() {
		var row backup.CaptureItemRow
		if err := rows.Scan(&row.ID, &row.CaptureID, &row.KnowledgeItemID, &row.Role, &row.Confidence, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan backup capture_item: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup capture_items: %w", err)
	}
	return items, nil
}

func (r *BackupRepository) exportLearnerItems(ctx context.Context) (items []backup.LearnerItemRow, resultErr error) {
	items = make([]backup.LearnerItemRow, 0)
	rows, err := r.db.QueryContext(ctx, `SELECT
id, knowledge_item_id, familiarity_score, mastery_score, ask_count, wrong_count, review_count, last_asked_at, last_wrong_at, last_reviewed_at, status
FROM learner_items
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("select backup learner_items: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close backup learner_items rows: %w", err)
		}
	}()

	for rows.Next() {
		var row backup.LearnerItemRow
		if err := rows.Scan(
			&row.ID, &row.KnowledgeItemID, &row.FamiliarityScore, &row.MasteryScore,
			&row.AskCount, &row.WrongCount, &row.ReviewCount, &row.LastAskedAt, &row.LastWrongAt, &row.LastReviewedAt, &row.Status,
		); err != nil {
			return nil, fmt.Errorf("scan backup learner_item: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup learner_items: %w", err)
	}
	return items, nil
}

func (r *BackupRepository) exportReviewCards(ctx context.Context) (cards []backup.ReviewCardRow, resultErr error) {
	cards = make([]backup.ReviewCardRow, 0)
	rows, err := r.db.QueryContext(ctx, `SELECT
id, knowledge_item_id, card_type, question, answer, explanation, state, due_at, stability, difficulty, retrievability, reps, lapses, last_review_at, created_at, updated_at
FROM review_cards
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("select backup review_cards: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close backup review_cards rows: %w", err)
		}
	}()

	for rows.Next() {
		var row backup.ReviewCardRow
		if err := rows.Scan(
			&row.ID, &row.KnowledgeItemID, &row.CardType, &row.Question, &row.Answer,
			&row.Explanation, &row.State, &row.DueAt, &row.Stability, &row.Difficulty, &row.Retrievability,
			&row.Reps, &row.Lapses, &row.LastReviewAt, &row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan backup review_card: %w", err)
		}
		cards = append(cards, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup review_cards: %w", err)
	}
	return cards, nil
}

func (r *BackupRepository) exportReviewLogs(ctx context.Context) (logs []backup.ReviewLogRow, resultErr error) {
	logs = make([]backup.ReviewLogRow, 0)
	rows, err := r.db.QueryContext(ctx, `SELECT
id, review_card_id, source, rating, elapsed_ms, reviewed_at
FROM review_logs
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("select backup review_logs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close backup review_logs rows: %w", err)
		}
	}()

	for rows.Next() {
		var row backup.ReviewLogRow
		if err := rows.Scan(&row.ID, &row.ReviewCardID, &row.Source, &row.Rating, &row.ElapsedMs, &row.ReviewedAt); err != nil {
			return nil, fmt.Errorf("scan backup review_log: %w", err)
		}
		logs = append(logs, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup review_logs: %w", err)
	}
	return logs, nil
}

func (r *BackupRepository) exportLookupJobs(ctx context.Context) (jobs []backup.LookupJobRow, resultErr error) {
	jobs = make([]backup.LookupJobRow, 0)
	rows, err := r.db.QueryContext(ctx, `SELECT
id, capture_id, status, provider, model, prompt_version, error_message, started_at, finished_at, created_at
FROM lookup_jobs
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("select backup lookup_jobs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close backup lookup_jobs rows: %w", err)
		}
	}()

	for rows.Next() {
		var row backup.LookupJobRow
		if err := rows.Scan(
			&row.ID, &row.CaptureID, &row.Status, &row.Provider, &row.Model, &row.PromptVersion,
			&row.ErrorMessage, &row.StartedAt, &row.FinishedAt, &row.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan backup lookup_job: %w", err)
		}
		jobs = append(jobs, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup lookup_jobs: %w", err)
	}
	return jobs, nil
}

func (r *BackupRepository) exportReviewCardCandidates(ctx context.Context) (candidates []backup.ReviewCardCandidateRow, resultErr error) {
	candidates = make([]backup.ReviewCardCandidateRow, 0)
	rows, err := r.db.QueryContext(ctx, `SELECT
id, capture_id, knowledge_item_id, card_type, question, answer, explanation, created_at, consumed_at
FROM review_card_candidates
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("select backup review_card_candidates: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close backup review_card_candidates rows: %w", err)
		}
	}()

	for rows.Next() {
		var row backup.ReviewCardCandidateRow
		if err := rows.Scan(
			&row.ID, &row.CaptureID, &row.KnowledgeItemID, &row.CardType, &row.Question,
			&row.Answer, &row.Explanation, &row.CreatedAt, &row.ConsumedAt,
		); err != nil {
			return nil, fmt.Errorf("scan backup review_card_candidate: %w", err)
		}
		candidates = append(candidates, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backup review_card_candidates: %w", err)
	}
	return candidates, nil
}

func (r *BackupRepository) Import(ctx context.Context, snapshot *backup.Snapshot) (result *backup.ImportResult, resultErr error) {
	if snapshot == nil {
		return nil, fmt.Errorf("import backup snapshot: nil snapshot")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin backup import transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, tx.Rollback())
		}
	}()

	result = &backup.ImportResult{}
	kmap := make(map[string]string, len(snapshot.KnowledgeItems))
	rcmap := make(map[string]string, len(snapshot.ReviewCards))

	if err := importKnowledgeItems(ctx, tx, snapshot.KnowledgeItems, result, kmap); err != nil {
		return nil, err
	}
	if err := importCaptures(ctx, tx, snapshot.Captures, result); err != nil {
		return nil, err
	}
	if err := importExplanations(ctx, tx, snapshot.Explanations, result); err != nil {
		return nil, err
	}
	if err := importLookupJobs(ctx, tx, snapshot.LookupJobs, result); err != nil {
		return nil, err
	}
	if err := importCaptureItems(ctx, tx, snapshot.CaptureItems, result, kmap); err != nil {
		return nil, err
	}
	if err := importLearnerItems(ctx, tx, snapshot.LearnerItems, result, kmap); err != nil {
		return nil, err
	}
	if err := importReviewCardCandidates(ctx, tx, snapshot.ReviewCardCandidates, result, kmap); err != nil {
		return nil, err
	}
	if err := importReviewCards(ctx, tx, snapshot.ReviewCards, result, kmap, rcmap); err != nil {
		return nil, err
	}
	if err := importReviewLogs(ctx, tx, snapshot.ReviewLogs, result, rcmap); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit backup import transaction: %w", err)
	}
	return result, nil
}

func importKnowledgeItems(ctx context.Context, tx *sql.Tx, rows []backup.KnowledgeItemRow, result *backup.ImportResult, kmap map[string]string) error {
	for _, row := range rows {
		var existingID string
		var existingFirstSeenAt, existingLastSeenAt time.Time
		err := tx.QueryRowContext(ctx,
			`SELECT id, first_seen_at, last_seen_at FROM knowledge_items WHERE normalized_key = ? AND item_type = ?`,
			row.NormalizedKey, row.ItemType,
		).Scan(&existingID, &existingFirstSeenAt, &existingLastSeenAt)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_items(
id, normalized_key, surface_text, item_type, language, pos, pronunciation, meaning_ko, description_ko, domain_category, first_seen_at, last_seen_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				row.ID, row.NormalizedKey, row.SurfaceText, row.ItemType, row.Language, row.Pos, row.Pronunciation,
				row.MeaningKo, row.DescriptionKo, row.DomainCategory, row.FirstSeenAt.UTC(), row.LastSeenAt.UTC(),
			); err != nil {
				return fmt.Errorf("insert backup knowledge_item %q: %w", row.ID, err)
			}
			kmap[row.ID] = row.ID
			result.KnowledgeItems.Inserted++
		case err != nil:
			return fmt.Errorf("select backup knowledge_item %q: %w", row.ID, err)
		default:
			kmap[row.ID] = existingID
			firstSeenAt := existingFirstSeenAt
			if row.FirstSeenAt.Before(existingFirstSeenAt) {
				firstSeenAt = row.FirstSeenAt
			}
			if row.LastSeenAt.After(existingLastSeenAt) {
				if _, err := tx.ExecContext(ctx, `UPDATE knowledge_items SET
surface_text = ?, language = ?, pos = ?, pronunciation = ?, meaning_ko = ?, description_ko = ?, domain_category = ?, first_seen_at = ?, last_seen_at = ?
WHERE id = ?`,
					row.SurfaceText, row.Language, row.Pos, row.Pronunciation, row.MeaningKo, row.DescriptionKo, row.DomainCategory,
					firstSeenAt.UTC(), row.LastSeenAt.UTC(), existingID,
				); err != nil {
					return fmt.Errorf("merge backup knowledge_item %q: %w", row.ID, err)
				}
			} else if firstSeenAt.Before(existingFirstSeenAt) {
				if _, err := tx.ExecContext(ctx, `UPDATE knowledge_items SET first_seen_at = ? WHERE id = ?`, firstSeenAt.UTC(), existingID); err != nil {
					return fmt.Errorf("merge backup knowledge_item first_seen_at %q: %w", row.ID, err)
				}
			}
			result.KnowledgeItems.Merged++
		}
	}
	return nil
}

func importCaptures(ctx context.Context, tx *sql.Tx, rows []backup.CaptureRow, result *backup.ImportResult) error {
	for _, row := range rows {
		exists, err := rowExists(ctx, tx, `SELECT 1 FROM captures WHERE id = ?`, row.ID)
		if err != nil {
			return fmt.Errorf("select backup capture %q: %w", row.ID, err)
		}
		if exists {
			result.Captures.Skipped++
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO captures(
id, source_app, source_type, source_title, source_url, selected_text, detected_lang, input_mode, text_hash, created_at, inbox_status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.SourceApp, row.SourceType, row.SourceTitle, row.SourceURL, row.SelectedText, row.DetectedLang,
			row.InputMode, row.TextHash, row.CreatedAt.UTC(), row.InboxStatus,
		); err != nil {
			return fmt.Errorf("insert backup capture %q: %w", row.ID, err)
		}
		result.Captures.Inserted++
	}
	return nil
}

func importExplanations(ctx context.Context, tx *sql.Tx, rows []backup.ExplanationRow, result *backup.ImportResult) error {
	for _, row := range rows {
		exists, err := rowExists(ctx, tx, `SELECT 1 FROM explanations WHERE capture_id = ?`, row.CaptureID)
		if err != nil {
			return fmt.Errorf("select backup explanation %q: %w", row.ID, err)
		}
		if exists {
			result.Explanations.Skipped++
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO explanations(
id, capture_id, brief_ko, detailed_ko, pronunciation, examples_json, terms_json, difficulty_estimate, category, raw_response_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.CaptureID, row.BriefKo, row.DetailedKo, row.Pronunciation, row.ExamplesJSON, row.TermsJSON,
			row.DifficultyEstimate, row.Category, row.RawResponseJSON, row.CreatedAt.UTC(),
		); err != nil {
			return fmt.Errorf("insert backup explanation %q: %w", row.ID, err)
		}
		result.Explanations.Inserted++
	}
	return nil
}

// importLookupJobs dedups by id, like captures/explanations: capture_id isn't
// remapped (captures are restored under their original id, never merged), so
// the row's own capture_id is used as-is.
func importLookupJobs(ctx context.Context, tx *sql.Tx, rows []backup.LookupJobRow, result *backup.ImportResult) error {
	for _, row := range rows {
		exists, err := rowExists(ctx, tx, `SELECT 1 FROM lookup_jobs WHERE id = ?`, row.ID)
		if err != nil {
			return fmt.Errorf("select backup lookup_job %q: %w", row.ID, err)
		}
		if exists {
			result.LookupJobs.Skipped++
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO lookup_jobs(
id, capture_id, status, provider, model, prompt_version, error_message, started_at, finished_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.CaptureID, row.Status, row.Provider, row.Model, row.PromptVersion,
			row.ErrorMessage, timePtrValue(row.StartedAt), timePtrValue(row.FinishedAt), row.CreatedAt.UTC(),
		); err != nil {
			return fmt.Errorf("insert backup lookup_job %q: %w", row.ID, err)
		}
		result.LookupJobs.Inserted++
	}
	return nil
}

func importCaptureItems(ctx context.Context, tx *sql.Tx, rows []backup.CaptureItemRow, result *backup.ImportResult, kmap map[string]string) error {
	for _, row := range rows {
		knowledgeItemID, err := resolvedID(kmap, row.KnowledgeItemID, "knowledge_item")
		if err != nil {
			return fmt.Errorf("import backup capture_item %q: %w", row.ID, err)
		}
		exists, err := rowExists(ctx, tx,
			`SELECT 1 FROM capture_items WHERE capture_id = ? AND knowledge_item_id = ? AND role = ?`,
			row.CaptureID, knowledgeItemID, row.Role,
		)
		if err != nil {
			return fmt.Errorf("select backup capture_item %q: %w", row.ID, err)
		}
		if exists {
			result.CaptureItems.Skipped++
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO capture_items(id, capture_id, knowledge_item_id, role, confidence, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
			row.ID, row.CaptureID, knowledgeItemID, row.Role, row.Confidence, row.CreatedAt.UTC(),
		); err != nil {
			return fmt.Errorf("insert backup capture_item %q: %w", row.ID, err)
		}
		result.CaptureItems.Inserted++
	}
	return nil
}

func importLearnerItems(ctx context.Context, tx *sql.Tx, rows []backup.LearnerItemRow, result *backup.ImportResult, kmap map[string]string) error {
	for _, row := range rows {
		knowledgeItemID, err := resolvedID(kmap, row.KnowledgeItemID, "knowledge_item")
		if err != nil {
			return fmt.Errorf("import backup learner_item %q: %w", row.ID, err)
		}

		var existing existingLearnerItem
		err = tx.QueryRowContext(ctx, `SELECT
id, familiarity_score, mastery_score, ask_count, wrong_count, review_count, last_asked_at, last_wrong_at, last_reviewed_at, status
FROM learner_items
WHERE knowledge_item_id = ?`, knowledgeItemID).Scan(
			&existing.id, &existing.familiarityScore, &existing.masteryScore, &existing.askCount, &existing.wrongCount, &existing.reviewCount,
			&existing.lastAskedAt, &existing.lastWrongAt, &existing.lastReviewedAt, &existing.status,
		)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, `INSERT INTO learner_items(
id, knowledge_item_id, familiarity_score, mastery_score, ask_count, wrong_count, review_count, last_asked_at, last_wrong_at, last_reviewed_at, status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				row.ID, knowledgeItemID, row.FamiliarityScore, row.MasteryScore, row.AskCount, row.WrongCount, row.ReviewCount,
				timePtrValue(row.LastAskedAt), timePtrValue(row.LastWrongAt), timePtrValue(row.LastReviewedAt), row.Status,
			); err != nil {
				return fmt.Errorf("insert backup learner_item %q: %w", row.ID, err)
			}
			result.LearnerItems.Inserted++
		case err != nil:
			return fmt.Errorf("select backup learner_item %q: %w", row.ID, err)
		default:
			status := existing.status
			if importedLearnerStatusIsNewer(row, existing) {
				status = row.Status
			}
			if _, err := tx.ExecContext(ctx, `UPDATE learner_items SET
familiarity_score = ?, mastery_score = ?, ask_count = ?, wrong_count = ?, review_count = ?,
last_asked_at = ?, last_wrong_at = ?, last_reviewed_at = ?, status = ?
WHERE id = ?`,
				maxFloat64(existing.familiarityScore, row.FamiliarityScore),
				maxFloat64(existing.masteryScore, row.MasteryScore),
				maxInt64(existing.askCount, row.AskCount),
				maxInt64(existing.wrongCount, row.WrongCount),
				maxInt64(existing.reviewCount, row.ReviewCount),
				timePtrValue(maxTimePtr(nullTimePtr(existing.lastAskedAt), row.LastAskedAt)),
				timePtrValue(maxTimePtr(nullTimePtr(existing.lastWrongAt), row.LastWrongAt)),
				timePtrValue(maxTimePtr(nullTimePtr(existing.lastReviewedAt), row.LastReviewedAt)),
				status,
				existing.id,
			); err != nil {
				return fmt.Errorf("update backup learner_item %q: %w", row.ID, err)
			}
			result.LearnerItems.Updated++
		}
	}
	return nil
}

// importReviewCardCandidates dedups by id (same reasoning as review_cards: no
// natural uniqueness beyond id — the same knowledge item can pick up more than
// one candidate across captures). knowledge_item_id is nullable and, when
// present, needs kmap remapping like every other knowledge_item_id reference.
func importReviewCardCandidates(ctx context.Context, tx *sql.Tx, rows []backup.ReviewCardCandidateRow, result *backup.ImportResult, kmap map[string]string) error {
	for _, row := range rows {
		knowledgeItemID, err := resolvedNullableID(kmap, row.KnowledgeItemID, "knowledge_item")
		if err != nil {
			return fmt.Errorf("import backup review_card_candidate %q: %w", row.ID, err)
		}
		exists, err := rowExists(ctx, tx, `SELECT 1 FROM review_card_candidates WHERE id = ?`, row.ID)
		if err != nil {
			return fmt.Errorf("select backup review_card_candidate %q: %w", row.ID, err)
		}
		if exists {
			result.ReviewCardCandidates.Skipped++
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO review_card_candidates(
id, capture_id, knowledge_item_id, card_type, question, answer, explanation, created_at, consumed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, row.CaptureID, knowledgeItemID, row.CardType, row.Question, row.Answer,
			row.Explanation, row.CreatedAt.UTC(), timePtrValue(row.ConsumedAt),
		); err != nil {
			return fmt.Errorf("insert backup review_card_candidate %q: %w", row.ID, err)
		}
		result.ReviewCardCandidates.Inserted++
	}
	return nil
}

func importReviewCards(ctx context.Context, tx *sql.Tx, rows []backup.ReviewCardRow, result *backup.ImportResult, kmap, rcmap map[string]string) error {
	for _, row := range rows {
		knowledgeItemID, err := resolvedID(kmap, row.KnowledgeItemID, "knowledge_item")
		if err != nil {
			return fmt.Errorf("import backup review_card %q: %w", row.ID, err)
		}
		// A review_card's identity is its id (there is no UNIQUE(knowledge_item_id, card_type):
		// re-marking a word unknown across captures legitimately yields multiple same-type cards).
		// Dedup by id so restore is lossless — re-importing skips an existing card (preserving its
		// live SRS state), and every distinct card is preserved. rcmap is identity-only.
		rcmap[row.ID] = row.ID
		exists, err := rowExists(ctx, tx, `SELECT 1 FROM review_cards WHERE id = ?`, row.ID)
		if err != nil {
			return fmt.Errorf("select backup review_card %q: %w", row.ID, err)
		}
		if exists {
			result.ReviewCards.Skipped++
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO review_cards(
id, knowledge_item_id, card_type, question, answer, explanation, state, due_at, stability, difficulty, retrievability, reps, lapses, last_review_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.ID, knowledgeItemID, row.CardType, row.Question, row.Answer, row.Explanation, row.State, timePtrValue(row.DueAt),
			row.Stability, row.Difficulty, row.Retrievability, row.Reps, row.Lapses, timePtrValue(row.LastReviewAt), row.CreatedAt.UTC(), row.UpdatedAt.UTC(),
		); err != nil {
			return fmt.Errorf("insert backup review_card %q: %w", row.ID, err)
		}
		result.ReviewCards.Inserted++
	}
	return nil
}

func importReviewLogs(ctx context.Context, tx *sql.Tx, rows []backup.ReviewLogRow, result *backup.ImportResult, rcmap map[string]string) error {
	for _, row := range rows {
		reviewCardID, err := resolvedID(rcmap, row.ReviewCardID, "review_card")
		if err != nil {
			return fmt.Errorf("import backup review_log %q: %w", row.ID, err)
		}
		exists, err := rowExists(ctx, tx,
			`SELECT 1 FROM review_logs WHERE review_card_id = ? AND reviewed_at = ? AND rating = ?`,
			reviewCardID, row.ReviewedAt.UTC(), row.Rating,
		)
		if err != nil {
			return fmt.Errorf("select backup review_log %q: %w", row.ID, err)
		}
		if exists {
			result.ReviewLogs.Skipped++
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO review_logs(id, review_card_id, source, rating, elapsed_ms, reviewed_at)
VALUES (?, ?, ?, ?, ?, ?)`,
			row.ID, reviewCardID, row.Source, row.Rating, row.ElapsedMs, row.ReviewedAt.UTC(),
		); err != nil {
			return fmt.Errorf("insert backup review_log %q: %w", row.ID, err)
		}
		result.ReviewLogs.Inserted++
	}
	return nil
}

func (r *BackupRepository) BackupFile(ctx context.Context, path string) (*backup.BackupResult, error) {
	if _, err := r.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return nil, fmt.Errorf("vacuum into backup file: %w", err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat backup file: %w", err)
	}
	return &backup.BackupResult{Path: path, SizeBytes: stat.Size()}, nil
}

type existingLearnerItem struct {
	id               string
	familiarityScore float64
	masteryScore     float64
	askCount         int64
	wrongCount       int64
	reviewCount      int64
	lastAskedAt      sql.NullTime
	lastWrongAt      sql.NullTime
	lastReviewedAt   sql.NullTime
	status           string
}

func rowExists(ctx context.Context, tx *sql.Tx, query string, args ...any) (bool, error) {
	var one int
	err := tx.QueryRowContext(ctx, query, args...).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, err
	default:
		return true, nil
	}
}

func resolvedID(mapping map[string]string, oldID, label string) (string, error) {
	resolved, ok := mapping[oldID]
	if !ok {
		return "", fmt.Errorf("missing %s mapping for %q", label, oldID)
	}
	return resolved, nil
}

// resolvedNullableID is resolvedID for an optional foreign key: nil in, nil out.
func resolvedNullableID(mapping map[string]string, oldID *string, label string) (*string, error) {
	if oldID == nil {
		return nil, nil
	}
	resolved, err := resolvedID(mapping, *oldID, label)
	if err != nil {
		return nil, err
	}
	return &resolved, nil
}

func maxFloat64(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

func maxInt64(a, b int64) int64 {
	if b > a {
		return b
	}
	return a
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	utc := value.Time.UTC()
	return &utc
}

func maxTimePtr(a, b *time.Time) *time.Time {
	switch {
	case a == nil && b == nil:
		return nil
	case a == nil:
		utc := b.UTC()
		return &utc
	case b == nil:
		utc := a.UTC()
		return &utc
	case b.After(*a):
		utc := b.UTC()
		return &utc
	default:
		utc := a.UTC()
		return &utc
	}
}

func timePtrValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func importedLearnerStatusIsNewer(row backup.LearnerItemRow, existing existingLearnerItem) bool {
	importedNewest, importedOK := newestTime(row.LastAskedAt, row.LastWrongAt, row.LastReviewedAt)
	existingNewest, existingOK := newestTime(nullTimePtr(existing.lastAskedAt), nullTimePtr(existing.lastWrongAt), nullTimePtr(existing.lastReviewedAt))
	return importedOK && (!existingOK || importedNewest.After(existingNewest))
}

func newestTime(values ...*time.Time) (time.Time, bool) {
	var newest time.Time
	ok := false
	for _, value := range values {
		if value == nil {
			continue
		}
		if !ok || value.After(newest) {
			newest = value.UTC()
			ok = true
		}
	}
	return newest, ok
}
