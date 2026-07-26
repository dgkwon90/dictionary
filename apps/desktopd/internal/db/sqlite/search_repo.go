package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"neulsang/desktopd/internal/domain/capture"
	"neulsang/desktopd/internal/domain/review"
	"neulsang/desktopd/internal/domain/search"
	"neulsang/desktopd/internal/id"
)

type SearchRepository struct {
	db *sql.DB
}

var _ search.Repository = (*SearchRepository)(nil)

func NewSearchRepository(db *sql.DB) *SearchRepository {
	return &SearchRepository{db: db}
}

// searchListQuery is built once and filtered by appending predicates, rather than
// kept as two near-identical copies the way the old inbox queries were — those had
// drifted apart in the past because a change to one did not have to touch the other.
const searchListQuery = `WITH job_latest AS (
  SELECT capture_id, status,
         ROW_NUMBER() OVER (PARTITION BY capture_id ORDER BY created_at DESC) AS rn
  FROM lookup_jobs
)
SELECT c.id, c.selected_text, c.source_app, c.source_type, c.input_mode,
       c.learn_kind, c.triage_state, jl.status, e.brief_ko, c.created_at
FROM captures c
LEFT JOIN job_latest jl ON jl.capture_id = c.id AND jl.rn = 1
LEFT JOIN explanations e ON e.capture_id = c.id
WHERE c.parent_capture_id IS NULL
  AND c.triage_state <> 'discarded'`

func (r *SearchRepository) List(ctx context.Context, input search.ListInput) (items []search.Item, resultErr error) {
	query := searchListQuery
	args := []any{}
	// "미확인" includes a sentence whose words have not been picked: the user has seen
	// it but has not yet said why they did not understand it, so it is not finished.
	if input.View == search.ViewUnresolved {
		query += "\n  AND c.triage_state IN ('unseen', 'needs_selection')"
	}
	if input.LearnKind != "" {
		query += "\n  AND c.learn_kind = ?"
		args = append(args, input.LearnKind)
	}
	query += "\nORDER BY c.created_at DESC\nLIMIT ?"
	args = append(args, input.Limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select search items: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close search rows: %w", err)
		}
	}()

	for rows.Next() {
		var item search.Item
		var sourceApp, sourceType, learnKind, jobStatus, briefKo sql.NullString
		if err := rows.Scan(
			&item.CaptureID,
			&item.SelectedText,
			&sourceApp,
			&sourceType,
			&item.InputMode,
			&learnKind,
			&item.TriageState,
			&jobStatus,
			&briefKo,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan search item: %w", err)
		}
		item.SourceApp = nullString(sourceApp)
		item.SourceType = nullString(sourceType)
		item.LearnKind = nullString(learnKind)
		item.JobStatus = nullString(jobStatus)
		item.BriefKo = nullString(briefKo)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search items: %w", err)
	}
	return items, nil
}

func (r *SearchRepository) LoadTriage(ctx context.Context, captureID string) (search.Triage, error) {
	var learnKind sql.NullString
	result := search.Triage{CaptureID: captureID}
	err := r.db.QueryRowContext(
		ctx,
		`SELECT learn_kind, triage_state FROM captures WHERE id = ?`,
		captureID,
	).Scan(&learnKind, &result.TriageState)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return search.Triage{}, search.ErrCaptureNotFound
	case err != nil:
		return search.Triage{}, fmt.Errorf("load capture triage: %w", err)
	}
	result.LearnKind = nullString(learnKind)
	return result, nil
}

func (r *SearchRepository) SetTriageState(ctx context.Context, captureID, state string, at time.Time) error {
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE captures SET triage_state = ?, updated_at = ? WHERE id = ?`,
		state, utc(at), captureID,
	)
	if err != nil {
		return fmt.Errorf("update triage state: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read triage rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return search.ErrCaptureNotFound
	}
	return nil
}

// RegisterWordForLearning is the "학습할래요" path for a word capture. Searching a word
// already means the user did not know it, so the whole capture becomes one learning
// item: the word itself.
//
// Everything happens in one transaction because a learner row without its cards would
// show up in the learning list with nothing to review, and cards without a learner row
// would never be scheduled.
func (r *SearchRepository) RegisterWordForLearning(ctx context.Context, captureID string, at time.Time) (result search.TriageResult, resultErr error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return search.TriageResult{}, fmt.Errorf("begin register word transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, tx.Rollback())
		}
	}()

	// The word the user searched for is the primary sub_item of its own capture. Pick
	// the highest-importance one; ties broken by surface text so the choice is stable.
	var knowledgeItemID string
	err = tx.QueryRowContext(
		ctx,
		`SELECT ci.knowledge_item_id
FROM capture_items ci
JOIN knowledge_items ki ON ki.id = ci.knowledge_item_id
WHERE ci.capture_id = ? AND ci.role = ?
ORDER BY ci.confidence DESC, ki.surface_text ASC
LIMIT 1`,
		captureID, captureItemRoleSubItem,
	).Scan(&knowledgeItemID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// No extracted term yet: the lookup has not finished, or it produced nothing
		// usable. Refuse rather than create an empty learning entry.
		return search.TriageResult{}, fmt.Errorf("%w: 아직 배울 항목이 준비되지 않았어요", search.ErrInvalidInput)
	case err != nil:
		return search.TriageResult{}, fmt.Errorf("select capture primary item: %w", err)
	}

	if err := registerLearnerItem(ctx, tx, knowledgeItemID, at); err != nil {
		return search.TriageResult{}, err
	}
	cardsCreated, err := promoteCandidatesToCards(ctx, tx, knowledgeItemID, at)
	if err != nil {
		return search.TriageResult{}, err
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE captures SET triage_state = ?, updated_at = ? WHERE id = ?`,
		capture.TriageLearning, utc(at), captureID,
	); err != nil {
		return search.TriageResult{}, fmt.Errorf("mark capture learning: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return search.TriageResult{}, fmt.Errorf("commit register word transaction: %w", err)
	}
	return search.TriageResult{
		CaptureID:       captureID,
		TriageState:     capture.TriageLearning,
		LearningItemIDs: []string{knowledgeItemID},
		CardsCreated:    cardsCreated,
	}, nil
}

// registerLearnerItem puts a knowledge item into the learning list, or records another
// encounter if it is already there. This is the single place membership is granted.
//
// registered_at is preserved on repeat so "오늘 등록한 단어" keeps meaning the day the
// user first committed to it. status is forced back to active because re-declaring
// something unknown reverses an earlier "I know this".
func registerLearnerItem(ctx context.Context, tx *sql.Tx, knowledgeItemID string, at time.Time) error {
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO learner_items(
id, knowledge_item_id, ask_count, unknown_count, registered_at, last_asked_at, last_unknown_at, status, updated_at
) VALUES (?, ?, 1, 1, ?, ?, ?, 'active', ?)
ON CONFLICT(knowledge_item_id) DO UPDATE SET
  unknown_count = unknown_count + 1,
  last_unknown_at = excluded.last_unknown_at,
  status = 'active',
  updated_at = excluded.updated_at`,
		id.New(), knowledgeItemID, utc(at), utc(at), utc(at), utc(at),
	); err != nil {
		return fmt.Errorf("register learner item: %w", err)
	}
	return nil
}

// promoteCandidatesToCards turns this item's unconsumed AI card candidates into review
// cards, due immediately.
//
// ON CONFLICT DO NOTHING against ux_review_cards_identity is what makes repeat searches
// safe: previously, looking the same word up twice stored a second set of candidates and
// the next registration turned them into duplicate copies of cards the user already had.
func promoteCandidatesToCards(ctx context.Context, tx *sql.Tx, knowledgeItemID string, at time.Time) (int, error) {
	created, err := promoteCandidates(ctx, tx,
		`SELECT id, card_type, question, answer, explanation, context_knowledge_item_id
FROM review_card_candidates
WHERE knowledge_item_id = ? AND consumed_at IS NULL`,
		[]any{knowledgeItemID}, at)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE review_card_candidates SET consumed_at = ? WHERE knowledge_item_id = ? AND consumed_at IS NULL`,
		utc(at), knowledgeItemID,
	); err != nil {
		return 0, fmt.Errorf("consume review card candidates: %w", err)
	}
	return created, nil
}

type candidateRow struct {
	id          string
	cardType    string
	question    string
	answer      string
	explanation sql.NullString
	contextID   sql.NullString
}

// promoteCandidates reads candidate rows fully before writing. Holding a cursor open
// while inserting into the same transaction is what the single-connection pool makes
// unsafe, so the rows are drained and closed first.
func promoteCandidates(ctx context.Context, tx *sql.Tx, query string, args []any, at time.Time) (created int, resultErr error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("select review card candidates: %w", err)
	}
	candidates := []candidateRow{}
	for rows.Next() {
		var candidate candidateRow
		if err := rows.Scan(&candidate.id, &candidate.cardType, &candidate.question, &candidate.answer, &candidate.explanation, &candidate.contextID); err != nil {
			resultErr = errors.Join(fmt.Errorf("scan review card candidate: %w", err), rows.Close())
			return 0, resultErr
		}
		candidates = append(candidates, candidate)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return 0, fmt.Errorf("read review card candidates: %w", err)
	}

	for _, candidate := range candidates {
		cardType := strings.TrimSpace(candidate.cardType)
		if cardType == "" {
			cardType = review.DefaultCardType
		}
		var contextID any
		if candidate.contextID.Valid {
			contextID = candidate.contextID.String
		}
		var explanation any
		if candidate.explanation.Valid && candidate.explanation.String != "" {
			explanation = candidate.explanation.String
		}
		// The owner of a candidate's card is whichever item the candidate hangs off;
		// callers pass a query that already scopes that.
		result, err := tx.ExecContext(
			ctx,
			`INSERT INTO review_cards(
id, knowledge_item_id, context_knowledge_item_id, card_type, question, answer, explanation, state, due_at, created_at, updated_at
) SELECT ?, knowledge_item_id, ?, ?, ?, ?, ?, ?, ?, ?, ?
FROM review_card_candidates WHERE id = ?
ON CONFLICT DO NOTHING`,
			id.New(), contextID, cardType, candidate.question, candidate.answer, explanation,
			review.CardStateNew, utc(at), utc(at), utc(at), candidate.id,
		)
		if err != nil {
			return 0, fmt.Errorf("insert review card: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("read review card rows affected: %w", err)
		}
		created += int(affected)
	}
	return created, nil
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}
