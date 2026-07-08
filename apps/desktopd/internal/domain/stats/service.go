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
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Summary builds the dashboard summary. "Today" and "this week" use UTC day and
// rolling-7-day boundaries; per-category weakness applies review.WeaknessScore to
// the category aggregates so the formula stays single-sourced (PRD §13.3).
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	now := s.now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	window := Window{
		Now:        now,
		TodayStart: todayStart,
		WeekStart:  now.AddDate(0, 0, -7),
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
