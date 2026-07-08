package stats

import (
	"context"
	"testing"
	"time"
)

type fakeRepo struct {
	gotWindow Window
	gotTopN   int
	raw       RawSummary
	err       error
}

func (f *fakeRepo) Summary(_ context.Context, window Window, topN int) (RawSummary, error) {
	f.gotWindow = window
	f.gotTopN = topN
	return f.raw, f.err
}

func TestServiceSummaryComputesWindowAndWeakness(t *testing.T) {
	repo := &fakeRepo{raw: RawSummary{
		TodaySearchCount: 3,
		WeekSearchCount:  9,
		DueCardCount:     2,
		MostSearched:     []WordStat{{KnowledgeItemID: "k1", SurfaceText: "stale", Count: 5}},
		Categories: []CategoryAggregate{
			// averaged per item: general avg (1,0,0.9) → 0.2-0.63<0 → 0
			{Category: "general", ItemCount: 1, AskSum: 1, WrongSum: 0, MasterySum: 0.9},
			// backend avg (2,1.5,0.25) → 0.4+0.75-0.175 = 0.975
			{Category: "backend", ItemCount: 2, AskSum: 4, WrongSum: 3, MasterySum: 0.5},
		},
	}}
	svc := NewService(repo)
	fixed := time.Date(2026, 7, 8, 15, 30, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }

	summary, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}

	if repo.gotTopN != TopN {
		t.Errorf("topN = %d, want %d", repo.gotTopN, TopN)
	}
	wantToday := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	if !repo.gotWindow.TodayStart.Equal(wantToday) || !repo.gotWindow.WeekStart.Equal(fixed.AddDate(0, 0, -7)) {
		t.Errorf("window = %#v", repo.gotWindow)
	}

	if summary.TodaySearchCount != 3 || summary.WeekSearchCount != 9 || summary.DueCardCount != 2 {
		t.Errorf("summary counts = %#v", summary)
	}
	// backend (1.95) should sort before general (0)
	if len(summary.CategoryWeakness) != 2 || summary.CategoryWeakness[0].Category != "backend" {
		t.Fatalf("category weakness order = %#v", summary.CategoryWeakness)
	}
	if !approx(summary.CategoryWeakness[0].WeaknessScore, 0.975) || summary.CategoryWeakness[1].WeaknessScore != 0 {
		t.Errorf("weakness scores = %#v", summary.CategoryWeakness)
	}
}

func approx(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}
