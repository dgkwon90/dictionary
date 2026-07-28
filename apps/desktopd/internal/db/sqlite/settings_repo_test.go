package sqlite

import (
	"context"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/explain"
	"neulsang/desktopd/internal/domain/review"
	"neulsang/desktopd/internal/domain/settings"
)

func TestSettingsRepositoryLoadDefaultsWhenEmpty(t *testing.T) {
	repo := NewSettingsRepository(openMigratedDB(t))
	got, err := repo.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != settings.Defaults() {
		t.Fatalf("Load() = %+v, want defaults %+v", got, settings.Defaults())
	}
}

func TestSettingsRepositorySaveThenLoadRoundTrips(t *testing.T) {
	repo := NewSettingsRepository(openMigratedDB(t))
	ctx := context.Background()

	want := settings.Defaults()
	want.NotificationsEnabled = false
	want.MorningReviewTime = "07:30"
	want.EveningReviewTime = "22:15"
	want.ReviewIntervals = review.Intervals{
		AgainMinutes: 5, FirstHardDays: 0.5, FirstGoodDays: 2, FirstEasyDays: 4,
		HardMultiplier: 1.1, GoodMultiplier: 2, EasyMultiplier: 3.5,
	}
	want.AIFormat = explain.Format{PromptStyle: "짧게, 백엔드 문맥으로", ExamplesCount: 4}
	if err := repo.Save(ctx, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := repo.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestSettingsRepositorySaveOverwrites(t *testing.T) {
	repo := NewSettingsRepository(openMigratedDB(t))
	ctx := context.Background()

	first := settings.Defaults()
	first.MorningReviewTime = "06:00"
	first.EveningReviewTime = "18:00"
	first.AIFormat.PromptStyle = "처음 스타일"
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	want := settings.Defaults()
	want.NotificationsEnabled = false
	want.MorningReviewTime = "10:00"
	want.EveningReviewTime = "20:00"
	want.ReviewIntervals.GoodMultiplier = 1.8
	// Clearing the style must actually clear it: an empty string is a real answer
	// ("no extra guidance"), not an unset key to fall back from.
	want.AIFormat = explain.Format{PromptStyle: "", ExamplesCount: 0}
	if err := repo.Save(ctx, want); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	got, err := repo.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() after overwrite = %+v, want %+v", got, want)
	}
}

// The schedule reaches grading through review.IntervalSource, not through the whole
// preference set — this is the wiring the review service actually depends on.
func TestSettingsRepositoryReviewIntervalsReadsSavedSchedule(t *testing.T) {
	repo := NewSettingsRepository(openMigratedDB(t))
	ctx := context.Background()

	prefs := settings.Defaults()
	prefs.ReviewIntervals = review.Intervals{
		AgainMinutes: 3, FirstHardDays: 0.25, FirstGoodDays: 1.5, FirstEasyDays: 5,
		HardMultiplier: 1.05, GoodMultiplier: 2.2, EasyMultiplier: 6,
	}
	if err := repo.Save(ctx, prefs); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := repo.ReviewIntervals(ctx)
	if err != nil {
		t.Fatalf("ReviewIntervals() error = %v", err)
	}
	if got != prefs.ReviewIntervals {
		t.Fatalf("ReviewIntervals() = %+v, want %+v", got, prefs.ReviewIntervals)
	}
}

func TestSettingsRepositoryReviewIntervalsDefaultsWhenUnset(t *testing.T) {
	repo := NewSettingsRepository(openMigratedDB(t))
	got, err := repo.ReviewIntervals(context.Background())
	if err != nil {
		t.Fatalf("ReviewIntervals() error = %v", err)
	}
	if got != review.DefaultIntervals() {
		t.Fatalf("ReviewIntervals() = %+v, want the default schedule", got)
	}
}

func TestSettingsRepositoryExplainFormatReadsSavedStyle(t *testing.T) {
	repo := NewSettingsRepository(openMigratedDB(t))
	ctx := context.Background()

	prefs := settings.Defaults()
	prefs.AIFormat = explain.Format{PromptStyle: "예문은 Go 코드로", ExamplesCount: 1}
	if err := repo.Save(ctx, prefs); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := repo.ExplainFormat(ctx)
	if err != nil {
		t.Fatalf("ExplainFormat() error = %v", err)
	}
	if got != prefs.AIFormat {
		t.Fatalf("ExplainFormat() = %+v, want %+v", got, prefs.AIFormat)
	}
}

// A row that cannot be read must cost the user that one setting, not the feature. A
// garbage schedule value that propagated into NextSchedule would make every card due
// the moment it was graded.
func TestSettingsRepositoryLoadFallsBackOnUnreadableValues(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewSettingsRepository(database)
	ctx := context.Background()

	for _, row := range []struct{ key, value string }{
		{settingReviewGoodMultiplier, "빠르게"},
		{settingReviewAgainMinutes, ""},
		{settingReviewFirstGoodDays, "-4"},
		{settingAIExamplesCount, "many"},
	} {
		if _, err := database.ExecContext(ctx,
			`INSERT INTO app_settings(key, value, updated_at) VALUES (?, ?, ?)`,
			row.key, row.value, utc(time.Now()),
		); err != nil {
			t.Fatalf("seed %s: %v", row.key, err)
		}
	}

	got, err := repo.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != settings.Defaults() {
		t.Fatalf("Load() = %+v, want defaults for every unreadable value", got)
	}
}

// Out-of-range values can arrive from an imported snapshot or an older build. Reading
// is not where they get refused — clamping keeps a lookup working.
func TestSettingsRepositoryLoadClampsStoredFormat(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewSettingsRepository(database)
	ctx := context.Background()

	if _, err := database.ExecContext(ctx,
		`INSERT INTO app_settings(key, value, updated_at) VALUES (?, ?, ?)`,
		settingAIExamplesCount, "99", utc(time.Now()),
	); err != nil {
		t.Fatalf("seed examples count: %v", err)
	}

	got, err := repo.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.AIFormat.ExamplesCount != explain.MaxExamplesCount {
		t.Fatalf("examples count = %d, want clamped to %d", got.AIFormat.ExamplesCount, explain.MaxExamplesCount)
	}
}
