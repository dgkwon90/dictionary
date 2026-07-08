package sqlite

import (
	"context"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/suggest"
)

func TestSuggestRepositorySavePickAndCached(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewSuggestRepository(database)
	ctx := context.Background()
	at := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)

	if err := repo.SavePick(ctx, "스테일", "stale", "오래된", at); err != nil {
		t.Fatalf("SavePick() error = %v", err)
	}

	cached, err := repo.Cached(ctx, "스테일")
	if err != nil {
		t.Fatalf("Cached() error = %v", err)
	}
	if len(cached) != 1 || cached[0].English != "stale" || cached[0].GlossKo != "오래된" {
		t.Fatalf("cached = %#v", cached)
	}
	if cached[0].Source != suggest.SourceCache || cached[0].Confidence != 1.0 {
		t.Fatalf("cached source/conf = %#v", cached[0])
	}
}

func TestSuggestRepositorySavePickReinforcesAndOrders(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewSuggestRepository(database)
	ctx := context.Background()
	base := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)

	// "style" picked once, "stale" picked twice → stale ranks first by hit_count.
	if err := repo.SavePick(ctx, "스테일", "style", "", base); err != nil {
		t.Fatalf("save style: %v", err)
	}
	if err := repo.SavePick(ctx, "스테일", "stale", "오래된", base.Add(time.Minute)); err != nil {
		t.Fatalf("save stale 1: %v", err)
	}
	if err := repo.SavePick(ctx, "스테일", "stale", "", base.Add(2*time.Minute)); err != nil {
		t.Fatalf("save stale 2: %v", err)
	}

	cached, err := repo.Cached(ctx, "스테일")
	if err != nil {
		t.Fatalf("Cached() error = %v", err)
	}
	if len(cached) != 2 || cached[0].English != "stale" {
		t.Fatalf("cached order = %#v, want stale first", cached)
	}

	// gloss preserved from the first non-empty pick (COALESCE keeps 오래된).
	var hitCount int
	var gloss string
	if err := database.QueryRowContext(ctx,
		`SELECT hit_count, gloss_ko FROM suggest_cache WHERE normalized_query=? AND english=?`, "스테일", "stale").
		Scan(&hitCount, &gloss); err != nil {
		t.Fatalf("query row: %v", err)
	}
	if hitCount != 2 || gloss != "오래된" {
		t.Fatalf("hit_count=%d gloss=%q, want 2/오래된", hitCount, gloss)
	}
}

func TestSuggestRepositoryCachedMiss(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewSuggestRepository(database)
	cached, err := repo.Cached(context.Background(), "없는쿼리")
	if err != nil {
		t.Fatalf("Cached() error = %v", err)
	}
	if len(cached) != 0 {
		t.Fatalf("cached = %#v, want empty", cached)
	}
}
