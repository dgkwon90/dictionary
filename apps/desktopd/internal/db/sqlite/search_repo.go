package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"neulsang/desktopd/internal/domain/capture"
	"neulsang/desktopd/internal/domain/knowledge"
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

// CreateRetryJob queues a second lookup for a capture whose last one failed.
//
// The check and the insert share one transaction so two clicks cannot both see the
// failed job and both queue a run — that would spend two AI calls and leave two jobs
// racing to write the same explanation.
func (r *SearchRepository) CreateRetryJob(ctx context.Context, captureID, jobID string, at time.Time) (text string, resultErr error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin retry job transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				resultErr = errors.Join(resultErr, fmt.Errorf("rollback retry job: %w", err))
			}
		}
	}()

	var triageState string
	if err := tx.QueryRowContext(ctx,
		`SELECT selected_text, triage_state FROM captures WHERE id = ?`, captureID,
	).Scan(&text, &triageState); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", search.ErrCaptureNotFound
		}
		return "", fmt.Errorf("read capture text: %w", err)
	}
	// A thrown-away search is not worth an AI call, and the explanation it produced
	// would write knowledge items and card candidates for something the user already
	// said they do not want. The screen never offers this, but the endpoint must not
	// depend on the screen for that.
	if triageState == capture.TriageDiscarded {
		return "", search.ErrNotRetryable
	}

	var latestStatus string
	switch err := tx.QueryRowContext(ctx,
		`SELECT status FROM lookup_jobs WHERE capture_id = ? ORDER BY created_at DESC LIMIT 1`,
		captureID,
	).Scan(&latestStatus); {
	case errors.Is(err, sql.ErrNoRows):
		// A capture with no job at all never got as far as failing, but it is just as
		// stuck as one that did, and queueing the lookup is exactly what it needs.
	case err != nil:
		return "", fmt.Errorf("read latest lookup job: %w", err)
	case latestStatus != "failed":
		return "", search.ErrNotRetryable
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO lookup_jobs(id, capture_id, status, created_at) VALUES (?, ?, 'queued', ?)`,
		jobID, captureID, utc(at),
	); err != nil {
		return "", fmt.Errorf("insert retry lookup job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit retry job transaction: %w", err)
	}
	return text, nil
}

// SetLearnKind rewrites the classification and the triage state together.
//
// One statement, not two: the schema forbids needs_selection on a word
// (CHECK(triage_state <> 'needs_selection' OR learn_kind = 'sentence')), so writing the
// kind first would break that constraint in the moment between the two updates.
func (r *SearchRepository) SetLearnKind(ctx context.Context, captureID, learnKind, triageState string, at time.Time) error {
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE captures SET learn_kind = ?, triage_state = ?, updated_at = ? WHERE id = ?`,
		learnKind, triageState, utc(at), captureID,
	)
	if err != nil {
		return fmt.Errorf("update learn kind: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read learn kind rows affected: %w", err)
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

// Get loads one search with the terms the AI found in it and which the user has
// picked. The detail panel needs all of it at once, so this is one round trip plus
// one for the items rather than a call per section.
func (r *SearchRepository) Get(ctx context.Context, captureID string) (search.Detail, error) {
	var detail search.Detail
	var learnKind, jobStatus, briefKo, detailedKo, sourceApp, sourceType sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT c.id, c.selected_text, c.source_app, c.source_type, c.input_mode,
       c.learn_kind, c.triage_state, c.created_at,
       (SELECT status FROM lookup_jobs WHERE capture_id = c.id ORDER BY created_at DESC LIMIT 1),
       e.brief_ko, e.detailed_ko
FROM captures c
LEFT JOIN explanations e ON e.capture_id = c.id
WHERE c.id = ?`, captureID).Scan(
		&detail.CaptureID, &detail.SelectedText, &sourceApp, &sourceType, &detail.InputMode,
		&learnKind, &detail.TriageState, &detail.CreatedAt, &jobStatus, &briefKo, &detailedKo,
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return search.Detail{}, search.ErrCaptureNotFound
	case err != nil:
		return search.Detail{}, fmt.Errorf("select search detail: %w", err)
	}
	detail.SourceApp = nullString(sourceApp)
	detail.SourceType = nullString(sourceType)
	detail.LearnKind = nullString(learnKind)
	detail.JobStatus = nullString(jobStatus)
	detail.BriefKo = nullString(briefKo)
	detail.DetailedKo = nullString(detailedKo)

	items, err := r.detailItems(ctx, captureID)
	if err != nil {
		return search.Detail{}, err
	}
	detail.Items = items
	return detail, nil
}

func (r *SearchRepository) detailItems(ctx context.Context, captureID string) (items []search.DetailItem, resultErr error) {
	rows, err := r.db.QueryContext(ctx, `SELECT ki.id, ki.surface_text, ki.meaning_ko, ki.description_ko, ki.pronunciation,
       ci.char_start, ci.char_end, ci.selected_at
FROM capture_items ci
JOIN knowledge_items ki ON ki.id = ci.knowledge_item_id
WHERE ci.capture_id = ? AND ci.role = ?
ORDER BY ci.char_start IS NULL, ci.char_start, ci.confidence DESC, ki.surface_text`,
		captureID, captureItemRoleSubItem)
	if err != nil {
		return nil, fmt.Errorf("select search detail items: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close search detail item rows: %w", err)
		}
	}()

	items = []search.DetailItem{}
	for rows.Next() {
		var item search.DetailItem
		var meaning, description, pronunciation sql.NullString
		var charStart, charEnd sql.NullInt64
		var selectedAt sql.NullTime
		if err := rows.Scan(&item.KnowledgeItemID, &item.SurfaceText, &meaning, &description, &pronunciation,
			&charStart, &charEnd, &selectedAt); err != nil {
			return nil, fmt.Errorf("scan search detail item: %w", err)
		}
		item.MeaningKo = nullString(meaning)
		item.DescriptionKo = nullString(description)
		item.PronunciationKo = nullString(pronunciation)
		// -1 rather than 0 for "not located": 0 is a real offset (the first character).
		item.CharStart, item.CharEnd = -1, -1
		if charStart.Valid && charEnd.Valid {
			item.CharStart, item.CharEnd = int(charStart.Int64), int(charEnd.Int64)
		}
		item.Selected = selectedAt.Valid
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search detail items: %w", err)
	}
	return items, nil
}

func (r *SearchRepository) SetSelected(ctx context.Context, captureID, knowledgeItemID string, selected bool, at time.Time) error {
	var selectedAt any
	if selected {
		selectedAt = utc(at)
	}
	result, err := r.db.ExecContext(ctx,
		`UPDATE capture_items SET selected_at = ?, updated_at = ?
WHERE capture_id = ? AND knowledge_item_id = ? AND role = ?`,
		selectedAt, utc(at), captureID, knowledgeItemID, captureItemRoleSubItem)
	if err != nil {
		return fmt.Errorf("update capture item selection: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read selection rows affected: %w", err)
	}
	if rowsAffected == 0 {
		// The term is not one of this capture's extracted items. Reporting "not found"
		// on the capture is the closest honest answer for the caller.
		return search.ErrCaptureNotFound
	}
	return nil
}

// CompleteSelection is the sentence counterpart to RegisterWordForLearning. It runs as
// one transaction because the sentence and the words picked out of it are a single
// decision: a sentence registered without its words would review as a bare translation
// with no record of why it was hard, and words registered without their sentence would
// lose the context that made them worth learning.
func (r *SearchRepository) CompleteSelection(ctx context.Context, input search.CompleteInput, at time.Time) (result search.TriageResult, resultErr error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return search.TriageResult{}, fmt.Errorf("begin complete selection transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, tx.Rollback())
		}
	}()

	var selectedText, language string
	var briefKo sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT c.selected_text, COALESCE(c.detected_lang, 'und'), e.brief_ko
FROM captures c LEFT JOIN explanations e ON e.capture_id = c.id
WHERE c.id = ?`, input.CaptureID).Scan(&selectedText, &language, &briefKo)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return search.TriageResult{}, search.ErrCaptureNotFound
	case err != nil:
		return search.TriageResult{}, fmt.Errorf("select capture for completion: %w", err)
	}

	selectedIDs, err := selectedKnowledgeItemIDs(ctx, tx, input.CaptureID)
	if err != nil {
		return search.TriageResult{}, err
	}
	if len(selectedIDs) == 0 && !input.NoUnknownWords {
		return search.TriageResult{}, capture.ErrSelectionRequired
	}

	// The sentence item normally already exists: the explanation created it, with the
	// AI's translation. Calling this again only fills a gap (a result that arrived
	// without a sentence object) and refreshes last_seen_at.
	sentenceItemID, err := upsertSentenceItem(ctx, tx, sentenceItem{
		text:      selectedText,
		language:  language,
		meaningKo: nullString(briefKo),
	}, at)
	if err != nil {
		return search.TriageResult{}, err
	}
	if err := upsertCaptureItem(ctx, tx, input.CaptureID, sentenceItemID, captureItemRoleSentenceSelf, 1, 0, 0, false, at); err != nil {
		return search.TriageResult{}, err
	}

	learningIDs := make([]string, 0, len(selectedIDs)+1)
	cardsCreated := 0

	// The sentence itself is a learning item: "이 문장이 무슨 뜻이었지?".
	if err := registerLearnerItem(ctx, tx, sentenceItemID, at); err != nil {
		return search.TriageResult{}, err
	}
	learningIDs = append(learningIDs, sentenceItemID)
	created, err := promoteCandidatesToCards(ctx, tx, sentenceItemID, at)
	if err != nil {
		return search.TriageResult{}, err
	}
	cardsCreated += created
	// Fallback for a sentence whose explanation carried no translation candidate: the
	// one-line summary is a worse answer than a real translation, but registering a
	// sentence with nothing to review at all is worse still. A re-completion lands here
	// too and inserts nothing, because the card already exists.
	if created == 0 && nullString(briefKo) != "" {
		fallbackCreated, err := insertSentenceCard(ctx, tx, sentenceItemID, selectedText, nullString(briefKo), at)
		if err != nil {
			return search.TriageResult{}, err
		}
		cardsCreated += fallbackCreated
	}

	for _, knowledgeItemID := range selectedIDs {
		if err := registerLearnerItem(ctx, tx, knowledgeItemID, at); err != nil {
			return search.TriageResult{}, err
		}
		created, err := promoteCandidatesToCards(ctx, tx, knowledgeItemID, at)
		if err != nil {
			return search.TriageResult{}, err
		}
		cardsCreated += created
		learningIDs = append(learningIDs, knowledgeItemID)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE captures SET triage_state = ?, updated_at = ? WHERE id = ?`,
		capture.TriageLearning, utc(at), input.CaptureID); err != nil {
		return search.TriageResult{}, fmt.Errorf("mark sentence capture learning: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return search.TriageResult{}, fmt.Errorf("commit complete selection transaction: %w", err)
	}
	return search.TriageResult{
		CaptureID:       input.CaptureID,
		TriageState:     capture.TriageLearning,
		LearningItemIDs: learningIDs,
		CardsCreated:    cardsCreated,
	}, nil
}

func selectedKnowledgeItemIDs(ctx context.Context, tx *sql.Tx, captureID string) (ids []string, resultErr error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT knowledge_item_id FROM capture_items
WHERE capture_id = ? AND role = ? AND selected_at IS NOT NULL
ORDER BY char_start IS NULL, char_start`, captureID, captureItemRoleSubItem)
	if err != nil {
		return nil, fmt.Errorf("select chosen words: %w", err)
	}
	// Drain before writing: the pool is pinned to a single connection, so holding this
	// cursor open across the inserts below would deadlock.
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, errors.Join(fmt.Errorf("scan chosen word: %w", err), rows.Close())
		}
		ids = append(ids, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("read chosen words: %w", err)
	}
	return ids, nil
}

// sentenceItem is a captured sentence about to be stored as a learning target.
type sentenceItem struct {
	text          string
	language      string
	meaningKo     string
	descriptionKo string
	// overwriteMeaning decides who wins when the row already has a meaning. The
	// explanation path sets it: a fresh translation supersedes an older one. The
	// completion path does not, because all it has is a fallback and overwriting a real
	// translation with it would be a downgrade.
	overwriteMeaning bool
}

// upsertSentenceItem stores the sentence as a knowledge item so it can own cards and a
// learner row exactly like a word does. Merging by the same (normalized_key, learn_kind)
// rule means looking the same sentence up twice does not create a second entry.
func upsertSentenceItem(ctx context.Context, tx *sql.Tx, item sentenceItem, at time.Time) (string, error) {
	normalizedKey := knowledge.NormalizeKey(item.text)
	if normalizedKey == "" {
		return "", fmt.Errorf("%w: sentence text is empty", search.ErrInvalidInput)
	}
	language := item.language
	if language == "" {
		language = "und"
	}
	var existingID string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM knowledge_items WHERE normalized_key = ? AND learn_kind = ?`,
		normalizedKey, capture.LearnKindSentence).Scan(&existingID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		newID := id.New()
		if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_items(
id, normalized_key, surface_text, learn_kind, language, meaning_ko, description_ko, first_seen_at, last_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?)`,
			newID, normalizedKey, item.text, capture.LearnKindSentence, language, item.meaningKo, item.descriptionKo,
			utc(at), utc(at), utc(at)); err != nil {
			return "", fmt.Errorf("insert sentence knowledge item: %w", err)
		}
		return newID, nil
	case err != nil:
		return "", fmt.Errorf("select sentence knowledge item: %w", err)
	default:
		// Both forms leave an existing value alone when the incoming one is empty; they
		// differ only in which side wins when both are present.
		update := `UPDATE knowledge_items SET
meaning_ko = COALESCE(meaning_ko, NULLIF(?, '')),
description_ko = COALESCE(description_ko, NULLIF(?, '')),
last_seen_at = ?, updated_at = ?
WHERE id = ?`
		if item.overwriteMeaning {
			update = `UPDATE knowledge_items SET
meaning_ko = COALESCE(NULLIF(?, ''), meaning_ko),
description_ko = COALESCE(NULLIF(?, ''), description_ko),
last_seen_at = ?, updated_at = ?
WHERE id = ?`
		}
		if _, err := tx.ExecContext(ctx, update,
			item.meaningKo, item.descriptionKo, utc(at), utc(at), existingID); err != nil {
			return "", fmt.Errorf("update sentence knowledge item: %w", err)
		}
		return existingID, nil
	}
}

// insertSentenceCard creates the "what did this sentence mean?" card. Until the AI
// contract carries a dedicated sentence translation, the explanation's one-line summary
// is the answer — it is what the user already read on screen.
func insertSentenceCard(ctx context.Context, tx *sql.Tx, sentenceItemID, question, answer string, at time.Time) (int, error) {
	result, err := tx.ExecContext(ctx, `INSERT INTO review_cards(
id, knowledge_item_id, card_type, question, answer, state, due_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING`,
		id.New(), sentenceItemID, cardTypeSentenceTranslation, question, answer,
		review.CardStateNew, utc(at), utc(at), utc(at))
	if err != nil {
		return 0, fmt.Errorf("insert sentence review card: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read sentence card rows affected: %w", err)
	}
	return int(affected), nil
}

const (
	// cardTypeSentenceTranslation is the card that asks for a whole sentence's meaning.
	cardTypeSentenceTranslation = "sentence_translation"
	// cardTypeCloze is the fill-in-the-blank card cut out of a captured sentence.
	cardTypeCloze = "cloze"
)
