package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/explain"
	"neulsang/desktopd/internal/domain/search"
)

const sentenceCaptureText = "The cache became stale after we deploy."

// insertSentenceCaptureFixture is insertCaptureFixture with a real sentence as the
// captured text, which the word fixture cannot be: sub_item offsets and cloze blanks
// are measured against it.
func insertSentenceCaptureFixture(t *testing.T, database *sql.DB, captureID, jobID string) {
	t.Helper()
	createdAt := time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(
		context.Background(),
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at, updated_at, triage_state) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		captureID, sentenceCaptureText, "manual", captureID+"-hash", createdAt, createdAt, "unseen",
	); err != nil {
		t.Fatalf("insert sentence capture fixture: %v", err)
	}
	if _, err := database.ExecContext(
		context.Background(),
		`INSERT INTO lookup_jobs(id, capture_id, status, created_at) VALUES (?, ?, ?, ?)`,
		jobID, captureID, "running", createdAt,
	); err != nil {
		t.Fatalf("insert lookup job fixture: %v", err)
	}
}

func sentenceExplainResult() explain.ExplainResult {
	return explain.ExplainResult{
		InputType:        "sentence",
		DetectedLanguage: "en",
		BriefKo:          "캐시가 오래됐다는 뜻입니다.",
		DetailedKo:       "become + 형용사 구문입니다.",
		DomainCategory:   "backend",
		Difficulty:       0.5,
		Sentence: &explain.Sentence{
			TranslationKo: "배포하고 나서 캐시가 오래된 상태가 되었다.",
			StructureKo:   "became가 상태 변화를 나타냅니다.",
		},
		SubItems: []explain.SubItem{
			{
				SurfaceText: "stale", SurfaceInText: "stale", NormalizedKey: "stale",
				ItemType: "word", MeaningKo: "오래된", Importance: 0.9,
				CardCandidates: []explain.ReviewCardCandidate{{CardType: "meaning", Question: "stale?", Answer: "오래된"}},
			},
			{
				// The dictionary form is not what is in the sentence ("deploy" is, but
				// pretend the AI returned the noun): surface_in_text is what makes the
				// blank land in the right place.
				SurfaceText: "deployment", SurfaceInText: "deploy", NormalizedKey: "deployment",
				ItemType: "term", MeaningKo: "배포", Importance: 0.6,
				CardCandidates: []explain.ReviewCardCandidate{{CardType: "meaning", Question: "deployment?", Answer: "배포"}},
			},
		},
	}
}

func TestExplainRepositorySaveSuccessRecordsSentenceItem(t *testing.T) {
	database := openMigratedDB(t)
	insertSentenceCaptureFixture(t, database, "capture-1", "job-1")
	repo := NewExplainRepository(database)
	now := time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)

	if err := repo.SaveSuccess(context.Background(), "job-1", "capture-1", sentenceExplainResult(), `{"raw":true}`, now); err != nil {
		t.Fatalf("SaveSuccess() error = %v", err)
	}

	var sentenceItemID, surfaceText string
	var meaningKo, descriptionKo sql.NullString
	if err := database.QueryRowContext(context.Background(),
		`SELECT id, surface_text, meaning_ko, description_ko
FROM knowledge_items WHERE learn_kind = 'sentence'`,
	).Scan(&sentenceItemID, &surfaceText, &meaningKo, &descriptionKo); err != nil {
		t.Fatalf("query sentence knowledge item: %v", err)
	}
	if surfaceText != sentenceCaptureText {
		t.Errorf("surface_text = %q, want the captured sentence", surfaceText)
	}
	// The sentence's meaning is the AI's translation, not brief_ko — brief_ko describes
	// the sentence, and grading a user against a description is grading them against
	// text they were never shown as the answer.
	if meaningKo.String != "배포하고 나서 캐시가 오래된 상태가 되었다." {
		t.Errorf("meaning_ko = %q, want the translation", meaningKo.String)
	}
	if descriptionKo.String != "became가 상태 변화를 나타냅니다." {
		t.Errorf("description_ko = %q, want structure_ko", descriptionKo.String)
	}

	// The sentence is linked to its capture, but is NOT in the learning list: only the
	// user completing word selection puts it there.
	var role string
	if err := database.QueryRowContext(context.Background(),
		`SELECT role FROM capture_items WHERE capture_id = ? AND knowledge_item_id = ?`,
		"capture-1", sentenceItemID).Scan(&role); err != nil {
		t.Fatalf("query sentence capture_item: %v", err)
	}
	if role != "sentence_self" {
		t.Errorf("role = %q, want sentence_self", role)
	}
	var learnerCount int
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM learner_items`).Scan(&learnerCount); err != nil {
		t.Fatalf("count learner_items: %v", err)
	}
	if learnerCount != 0 {
		t.Errorf("learner_items = %d, want 0 (explaining is not registering)", learnerCount)
	}

	// The sentence's own card candidate is a translation card owned by the sentence.
	var question, answer string
	var contextID sql.NullString
	if err := database.QueryRowContext(context.Background(),
		`SELECT question, answer, context_knowledge_item_id FROM review_card_candidates
WHERE knowledge_item_id = ? AND card_type = 'sentence_translation'`, sentenceItemID,
	).Scan(&question, &answer, &contextID); err != nil {
		t.Fatalf("query sentence translation candidate: %v", err)
	}
	if question != sentenceCaptureText {
		t.Errorf("question = %q, want the sentence itself", question)
	}
	if answer != "배포하고 나서 캐시가 오래된 상태가 되었다." {
		t.Errorf("answer = %q, want the translation", answer)
	}
	if contextID.Valid {
		// A sentence's own card has no outer context; only cloze cards do.
		t.Errorf("context_knowledge_item_id = %q, want NULL", contextID.String)
	}
}

func TestExplainRepositorySaveSuccessBuildsClozeFromCapturedText(t *testing.T) {
	database := openMigratedDB(t)
	insertSentenceCaptureFixture(t, database, "capture-1", "job-1")
	repo := NewExplainRepository(database)
	now := time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)

	if err := repo.SaveSuccess(context.Background(), "job-1", "capture-1", sentenceExplainResult(), `{"raw":true}`, now); err != nil {
		t.Fatalf("SaveSuccess() error = %v", err)
	}

	var sentenceItemID string
	if err := database.QueryRowContext(context.Background(),
		`SELECT id FROM knowledge_items WHERE learn_kind = 'sentence'`).Scan(&sentenceItemID); err != nil {
		t.Fatalf("query sentence item: %v", err)
	}

	tests := []struct {
		normalizedKey string
		wantQuestion  string
		wantAnswer    string
	}{
		{"stale", "The cache became ____ after we deploy.", "stale"},
		// surface_in_text ("deploy") locates the word even though surface_text is the
		// dictionary form ("deployment") that never appears in the sentence.
		{"deployment", "The cache became stale after we ____.", "deploy"},
	}
	for _, tt := range tests {
		t.Run(tt.normalizedKey, func(t *testing.T) {
			var question, answer, contextItemID string
			if err := database.QueryRowContext(context.Background(),
				`SELECT c.question, c.answer, c.context_knowledge_item_id
FROM review_card_candidates c JOIN knowledge_items ki ON ki.id = c.knowledge_item_id
WHERE ki.normalized_key = ? AND c.card_type = 'cloze'`, tt.normalizedKey,
			).Scan(&question, &answer, &contextItemID); err != nil {
				t.Fatalf("query cloze candidate: %v", err)
			}
			if question != tt.wantQuestion {
				t.Errorf("question = %q, want %q", question, tt.wantQuestion)
			}
			if answer != tt.wantAnswer {
				t.Errorf("answer = %q, want %q", answer, tt.wantAnswer)
			}
			// The card's owner is the word (its accuracy is what a blank tests) and the
			// sentence is only its context. Owning it the other way round would file
			// every word's mistakes under the sentence.
			if contextItemID != sentenceItemID {
				t.Errorf("context_knowledge_item_id = %q, want the sentence item %q", contextItemID, sentenceItemID)
			}
		})
	}
}

// A word the AI names but that is not in the text at all gets its meaning card and
// nothing else. Inventing a blank for it is the failure mode server-side cloze exists
// to rule out.
func TestExplainRepositorySaveSuccessSkipsClozeForAbsentWord(t *testing.T) {
	database := openMigratedDB(t)
	insertSentenceCaptureFixture(t, database, "capture-1", "job-1")
	repo := NewExplainRepository(database)
	result := sentenceExplainResult()
	result.SubItems = []explain.SubItem{{
		SurfaceText: "idempotent", NormalizedKey: "idempotent", ItemType: "term",
		MeaningKo: "멱등한", Importance: 0.8,
		CardCandidates: []explain.ReviewCardCandidate{{CardType: "meaning", Question: "idempotent?", Answer: "멱등한"}},
	}}
	now := time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)

	if err := repo.SaveSuccess(context.Background(), "job-1", "capture-1", result, `{"raw":true}`, now); err != nil {
		t.Fatalf("SaveSuccess() error = %v", err)
	}

	var clozeCount int
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM review_card_candidates WHERE card_type = 'cloze'`).Scan(&clozeCount); err != nil {
		t.Fatalf("count cloze candidates: %v", err)
	}
	if clozeCount != 0 {
		t.Errorf("cloze candidates = %d, want 0 for a word that is not in the text", clozeCount)
	}
	var meaningCount int
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM review_card_candidates WHERE card_type = 'meaning'`).Scan(&meaningCount); err != nil {
		t.Fatalf("count meaning candidates: %v", err)
	}
	if meaningCount != 1 {
		t.Errorf("meaning candidates = %d, want 1", meaningCount)
	}
}

// The whole sentence path end to end: what the AI returned has to survive all the way
// into the cards the user reviews. Each half is covered above; this checks the seam,
// which is where the previous contract lost input_type entirely.
func TestSentenceLookupThroughCompletionProducesTranslationAndClozeCards(t *testing.T) {
	database := openMigratedDB(t)
	ctx := context.Background()
	insertSentenceCaptureFixture(t, database, "capture-1", "job-1")
	now := time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)

	if err := NewExplainRepository(database).SaveSuccess(ctx, "job-1", "capture-1", sentenceExplainResult(), `{"raw":true}`, now); err != nil {
		t.Fatalf("SaveSuccess() error = %v", err)
	}

	var staleItemID string
	if err := database.QueryRowContext(ctx,
		`SELECT id FROM knowledge_items WHERE normalized_key = 'stale'`).Scan(&staleItemID); err != nil {
		t.Fatalf("query stale item: %v", err)
	}
	searchRepo := NewSearchRepository(database)
	if err := searchRepo.SetSelected(ctx, "capture-1", staleItemID, true, now); err != nil {
		t.Fatalf("SetSelected() error = %v", err)
	}
	result, err := searchRepo.CompleteSelection(ctx, search.CompleteInput{CaptureID: "capture-1"}, now)
	if err != nil {
		t.Fatalf("CompleteSelection() error = %v", err)
	}

	// The sentence's translation card and the chosen word's meaning + cloze cards.
	if result.CardsCreated != 3 {
		t.Fatalf("cards created = %d, want 3 (sentence translation + word meaning + cloze)", result.CardsCreated)
	}

	var sentenceAnswer string
	if err := database.QueryRowContext(ctx,
		`SELECT answer FROM review_cards WHERE card_type = 'sentence_translation'`).Scan(&sentenceAnswer); err != nil {
		t.Fatalf("query sentence card: %v", err)
	}
	if sentenceAnswer != "배포하고 나서 캐시가 오래된 상태가 되었다." {
		t.Errorf("sentence card answer = %q, want the AI translation rather than the brief_ko fallback", sentenceAnswer)
	}

	var clozeQuestion, clozeAnswer, clozeOwner string
	if err := database.QueryRowContext(ctx,
		`SELECT question, answer, knowledge_item_id FROM review_cards WHERE card_type = 'cloze'`,
	).Scan(&clozeQuestion, &clozeAnswer, &clozeOwner); err != nil {
		t.Fatalf("query cloze card: %v", err)
	}
	if clozeQuestion != "The cache became ____ after we deploy." || clozeAnswer != "stale" {
		t.Errorf("cloze card = %q / %q", clozeQuestion, clozeAnswer)
	}
	if clozeOwner != staleItemID {
		t.Errorf("cloze owner = %q, want the word %q", clozeOwner, staleItemID)
	}

	// The word the user did not pick keeps its candidates unconsumed: nothing it owns
	// becomes a card until they say they did not know it.
	var unpickedCards int
	if err := database.QueryRowContext(ctx,
		`SELECT count(*) FROM review_cards rc JOIN knowledge_items ki ON ki.id = rc.knowledge_item_id
WHERE ki.normalized_key = 'deployment'`).Scan(&unpickedCards); err != nil {
		t.Fatalf("count unpicked cards: %v", err)
	}
	if unpickedCards != 0 {
		t.Errorf("unpicked word has %d cards, want 0", unpickedCards)
	}
}

// A word lookup has no sentence, so it must not gain a sentence item or a cloze card.
func TestExplainRepositorySaveSuccessWordLookupHasNoSentenceItem(t *testing.T) {
	database := openMigratedDB(t)
	insertCaptureFixture(t, database, "capture-1", "job-1", "running")
	repo := NewExplainRepository(database)
	now := time.Date(2026, 7, 7, 2, 0, 0, 0, time.UTC)

	if err := repo.SaveSuccess(context.Background(), "job-1", "capture-1", repositoryExplainResult(), `{"raw":true}`, now); err != nil {
		t.Fatalf("SaveSuccess() error = %v", err)
	}

	var sentenceItems, clozeCandidates int
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM knowledge_items WHERE learn_kind = 'sentence'`).Scan(&sentenceItems); err != nil {
		t.Fatalf("count sentence items: %v", err)
	}
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM review_card_candidates WHERE card_type = 'cloze'`).Scan(&clozeCandidates); err != nil {
		t.Fatalf("count cloze candidates: %v", err)
	}
	if sentenceItems != 0 || clozeCandidates != 0 {
		t.Errorf("word lookup produced %d sentence items and %d cloze candidates, want 0 and 0", sentenceItems, clozeCandidates)
	}
}
