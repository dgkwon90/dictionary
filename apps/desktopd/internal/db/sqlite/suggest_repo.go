package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"neulsang/desktopd/internal/domain/suggest"
	"neulsang/desktopd/internal/id"
)

type SuggestRepository struct {
	db *sql.DB
}

var _ suggest.Repository = (*SuggestRepository)(nil)

func NewSuggestRepository(db *sql.DB) *SuggestRepository {
	return &SuggestRepository{db: db}
}

// Cached returns confirmed picks for a normalized query, most-reinforced first.
func (r *SuggestRepository) Cached(ctx context.Context, normalizedQuery string) (candidates []suggest.Candidate, resultErr error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT english, gloss_ko FROM suggest_cache
WHERE normalized_query = ?
ORDER BY hit_count DESC, last_used_at DESC`,
		normalizedQuery,
	)
	if err != nil {
		return nil, fmt.Errorf("select suggest cache: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close suggest cache rows: %w", err)
		}
	}()

	for rows.Next() {
		var candidate suggest.Candidate
		var glossKo sql.NullString
		if err := rows.Scan(&candidate.English, &glossKo); err != nil {
			return nil, fmt.Errorf("scan suggest cache: %w", err)
		}
		candidate.GlossKo = nullString(glossKo)
		candidate.Confidence = 1.0 // a confirmed pick is authoritative
		candidate.Source = suggest.SourceCache
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate suggest cache: %w", err)
	}
	return candidates, nil
}

// SavePick records or reinforces a confirmed query→english pick.
func (r *SuggestRepository) SavePick(ctx context.Context, normalizedQuery, english, glossKo string, at time.Time) error {
	if _, err := r.db.ExecContext(
		ctx,
		`INSERT INTO suggest_cache(id, normalized_query, english, gloss_ko, hit_count, created_at, last_used_at)
VALUES (?, ?, ?, NULLIF(?, ''), 1, ?, ?)
ON CONFLICT(normalized_query, english) DO UPDATE SET
  hit_count = hit_count + 1,
  last_used_at = excluded.last_used_at,
  gloss_ko = COALESCE(excluded.gloss_ko, suggest_cache.gloss_ko)`,
		id.New(), normalizedQuery, english, glossKo, at, at,
	); err != nil {
		return fmt.Errorf("save suggest pick: %w", err)
	}
	return nil
}
