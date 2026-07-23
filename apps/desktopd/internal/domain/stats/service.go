package stats

import (
	"context"
	"sort"
	"time"

	"neulsang/desktopd/internal/domain/review"
)

type Service struct {
	repo Repository
	now  func() time.Time
	loc  *time.Location
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now, loc: time.Local}
}

// Summary builds the dashboard summary. "Today" resets at local-timezone midnight
// (RW-07/review R-07: using UTC midnight reset "today" at 09:00 KST, mixing
// yesterday's data into "today" until then) — the boundary is computed in s.loc
// (OS-configured local zone by default) and converted to a UTC instant before
// binding to the DB query, since stored timestamps are UTC ([[modernc-time-utc]]).
// "This week" stays a rolling 7-day window (calendar-week vs rolling is a separate
// product question tracked in RW-07). Per-category weakness applies
// review.WeaknessScore to the category aggregates so the formula stays
// single-sourced (PRD §13.3).
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	now := s.now()
	local := now.In(s.loc)
	todayStartLocal := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, s.loc)
	window := Window{
		Now:        now.UTC(),
		TodayStart: todayStartLocal.UTC(),
		WeekStart:  now.UTC().AddDate(0, 0, -7),
	}

	raw, err := s.repo.Summary(ctx, window, TopN)
	if err != nil {
		return Summary{}, err
	}

	// Category weakness is the weakness of the *average* item in the category (PRD
	// §10.6 "카테고리별 약점"): averaging the per-item counts before applying the
	// formula avoids biasing toward categories that simply hold more items and keeps
	// avg mastery in [0,1] so a few weak items are not masked by many mastered ones.
	weakness := make([]CategoryWeakness, 0, len(raw.Categories))
	for _, category := range raw.Categories {
		n := float64(category.ItemCount)
		if n == 0 {
			n = 1
		}
		weakness = append(weakness, CategoryWeakness{
			Category:      category.Category,
			ItemCount:     category.ItemCount,
			WeaknessScore: review.WeaknessScore(float64(category.AskSum)/n, float64(category.WrongSum)/n, category.MasterySum/n, 0),
		})
	}
	// Weakest categories first; ties broken by name for a stable order.
	sort.SliceStable(weakness, func(i, j int) bool {
		if weakness[i].WeaknessScore != weakness[j].WeaknessScore {
			return weakness[i].WeaknessScore > weakness[j].WeaknessScore
		}
		return weakness[i].Category < weakness[j].Category
	})

	return Summary{
		TodaySearchCount:      raw.TodaySearchCount,
		WeekSearchCount:       raw.WeekSearchCount,
		TodayCompletedReviews: raw.TodayCompletedReviews,
		DueCardCount:          raw.DueCardCount,
		MostSearched:          raw.MostSearched,
		MostWrong:             raw.MostWrong,
		CategoryWeakness:      weakness,
	}, nil
}
