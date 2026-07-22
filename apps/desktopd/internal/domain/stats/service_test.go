package stats

import (
	"context"
	"testing"
	"time"

	// Guarantees IANA zone data is available for LoadLocation regardless of the
	// host OS (Windows CI runners don't ship system tzdata) — test-only, does not
	// bloat the production binary since it's imported from a _test.go file only.
	_ "time/tzdata"
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
	svc.loc = time.UTC // pin explicitly so the test doesn't depend on the runner's system timezone
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

// TestServiceSummaryTodayBoundaryUsesLocalTimezone reproduces review R-07: a
// search made at 01:00 KST (2026-07-08 local day) is still 2026-07-07 16:00 UTC —
// under the old UTC-midnight boundary it would have been counted as "yesterday"
// until 09:00 KST. TodayStart must reflect the *local* day's midnight, converted
// to the equivalent UTC instant for the DB query.
func TestServiceSummaryTodayBoundaryUsesLocalTimezone(t *testing.T) {
	seoul, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatalf("load Asia/Seoul: %v", err)
	}
	repo := &fakeRepo{}
	svc := NewService(repo)
	svc.loc = seoul
	fixed := time.Date(2026, 7, 7, 16, 0, 0, 0, time.UTC) // 2026-07-08 01:00 KST
	svc.now = func() time.Time { return fixed }

	if _, err := svc.Summary(context.Background()); err != nil {
		t.Fatalf("Summary() error = %v", err)
	}

	wantToday := time.Date(2026, 7, 7, 15, 0, 0, 0, time.UTC) // 2026-07-08 00:00 KST
	if !repo.gotWindow.TodayStart.Equal(wantToday) {
		t.Errorf("TodayStart = %v, want %v (local KST midnight)", repo.gotWindow.TodayStart, wantToday)
	}
	if !repo.gotWindow.Now.Equal(fixed) {
		t.Errorf("Now = %v, want %v", repo.gotWindow.Now, fixed)
	}
}

// TestServiceSummaryTodayBoundaryAcrossDSTTransition guards against a location
// with DST regressing the local-midnight computation: time.Date must still
// resolve to a valid, correct wall-clock instant across a spring-forward
// transition (America/New_York, 2026-03-08 02:00 -> 03:00 local).
func TestServiceSummaryTodayBoundaryAcrossDSTTransition(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load America/New_York: %v", err)
	}
	repo := &fakeRepo{}
	svc := NewService(repo)
	svc.loc = newYork
	// 2026-03-09 12:00 UTC = 2026-03-09 08:00 EDT (UTC-4), the day after the
	// spring-forward transition, so the host is already observing DST.
	fixed := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }

	if _, err := svc.Summary(context.Background()); err != nil {
		t.Fatalf("Summary() error = %v", err)
	}

	wantToday := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC) // 2026-03-09 00:00 EDT
	if !repo.gotWindow.TodayStart.Equal(wantToday) {
		t.Errorf("TodayStart = %v, want %v (EDT midnight)", repo.gotWindow.TodayStart, wantToday)
	}
}

func approx(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}
