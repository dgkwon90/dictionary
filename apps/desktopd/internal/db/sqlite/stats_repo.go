package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"neulsang/desktopd/internal/domain/stats"
)

type StatsRepository struct {
	db *sql.DB
}

var _ stats.Repository = (*StatsRepository)(nil)

func NewStatsRepository(db *sql.DB) *StatsRepository {
	return &StatsRepository{db: db}
}

func (r *StatsRepository) Summary(ctx context.Context, window stats.Window, topN int) (stats.RawSummary, error) {
	var summary stats.RawSummary

	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM captures WHERE created_at >= ?`, window.TodayStart).
		Scan(&summary.TodaySearchCount); err != nil {
		return stats.RawSummary{}, fmt.Errorf("count today searches: %w", err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM captures WHERE created_at >= ?`, window.WeekStart).
		Scan(&summary.WeekSearchCount); err != nil {
		return stats.RawSummary{}, fmt.Errorf("count week searches: %w", err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM review_logs WHERE reviewed_at >= ?`, window.TodayStart).
		Scan(&summary.TodayCompletedReviews); err != nil {
		return stats.RawSummary{}, fmt.Errorf("count today reviews: %w", err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM review_cards WHERE due_at IS NOT NULL AND due_at <= ?`, window.Now).
		Scan(&summary.DueCardCount); err != nil {
		return stats.RawSummary{}, fmt.Errorf("count due cards: %w", err)
	}

	mostSearched, err := r.topWords(ctx, "ask_count", topN)
	if err != nil {
		return stats.RawSummary{}, err
	}
	summary.MostSearched = mostSearched

	mostWrong, err := r.topWords(ctx, "wrong_count", topN)
	if err != nil {
		return stats.RawSummary{}, err
	}
	summary.MostWrong = mostWrong

	categories, err := r.categoryAggregates(ctx)
	if err != nil {
		return stats.RawSummary{}, err
	}
	summary.Categories = categories

	return summary, nil
}

// topWords ranks knowledge items by a learner_items counter column. The column is a
// fixed internal identifier (never user input), so interpolating it is safe.
func (r *StatsRepository) topWords(ctx context.Context, column string, topN int) (words []stats.WordStat, resultErr error) {
	query := fmt.Sprintf(`SELECT ki.id, ki.surface_text, li.%[1]s
FROM learner_items li
JOIN knowledge_items ki ON ki.id = li.knowledge_item_id
WHERE li.%[1]s > 0
ORDER BY li.%[1]s DESC, ki.surface_text ASC
LIMIT ?`, column)

	rows, err := r.db.QueryContext(ctx, query, topN)
	if err != nil {
		return nil, fmt.Errorf("select top %s: %w", column, err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close top %s rows: %w", column, err)
		}
	}()

	for rows.Next() {
		var word stats.WordStat
		if err := rows.Scan(&word.KnowledgeItemID, &word.SurfaceText, &word.Count); err != nil {
			return nil, fmt.Errorf("scan top %s: %w", column, err)
		}
		words = append(words, word)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate top %s: %w", column, err)
	}
	return words, nil
}

func (r *StatsRepository) categoryAggregates(ctx context.Context) (categories []stats.CategoryAggregate, resultErr error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
  COALESCE(ki.domain_category, 'general') AS category,
  count(*),
  COALESCE(sum(li.ask_count), 0),
  COALESCE(sum(li.wrong_count), 0),
  COALESCE(sum(li.mastery_score), 0)
FROM learner_items li
JOIN knowledge_items ki ON ki.id = li.knowledge_item_id
GROUP BY category`)
	if err != nil {
		return nil, fmt.Errorf("select category aggregates: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close category rows: %w", err)
		}
	}()

	for rows.Next() {
		var category stats.CategoryAggregate
		if err := rows.Scan(&category.Category, &category.ItemCount, &category.AskSum, &category.WrongSum, &category.MasterySum); err != nil {
			return nil, fmt.Errorf("scan category aggregate: %w", err)
		}
		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate category aggregates: %w", err)
	}
	return categories, nil
}
