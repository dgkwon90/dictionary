// Package stats computes the dashboard summary (PRD §14.3, §15.7): search volume,
// most-searched / most-wrong words, per-category weakness, and due-card counts.
// It reads aggregates from a Repository and applies the learner-score formulas.
package stats

import (
	"context"
	"time"
)

// TopN is how many words the most-searched / most-wrong lists return (PRD §13.3).
const TopN = 10

// WordStat is one entry in a most-searched / most-wrong ranking.
type WordStat struct {
	KnowledgeItemID string
	SurfaceText     string
	Count           int
}

// CategoryAggregate is the raw per-category rollup the repository returns; the
// service turns it into a weakness score.
type CategoryAggregate struct {
	Category   string
	ItemCount  int
	AskSum     int
	WrongSum   int
	MasterySum float64
}

// CategoryWeakness is a category's computed weakness for the dashboard.
type CategoryWeakness struct {
	Category      string
	ItemCount     int
	WeaknessScore float64
}

// RawSummary is what the repository gathers; the service derives CategoryWeakness
// from Categories and assembles the final Summary.
type RawSummary struct {
	TodaySearchCount      int
	WeekSearchCount       int
	TodayCompletedReviews int
	DueCardCount          int
	MostSearched          []WordStat
	MostWrong             []WordStat
	Categories            []CategoryAggregate
}

// Summary is the dashboard payload (PRD §10.6, §15.7).
type Summary struct {
	TodaySearchCount      int
	WeekSearchCount       int
	TodayCompletedReviews int
	DueCardCount          int
	MostSearched          []WordStat
	MostWrong             []WordStat
	CategoryWeakness      []CategoryWeakness
}

// Window carries the time boundaries the queries need, computed once by the service.
type Window struct {
	Now        time.Time
	TodayStart time.Time
	WeekStart  time.Time
}

type Repository interface {
	Summary(ctx context.Context, window Window, topN int) (RawSummary, error)
}
