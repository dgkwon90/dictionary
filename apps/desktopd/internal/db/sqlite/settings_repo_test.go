package sqlite

import (
	"context"
	"testing"

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

	want := settings.Preferences{NotificationsEnabled: false, MorningReviewTime: "07:30", EveningReviewTime: "22:15"}
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

	if err := repo.Save(ctx, settings.Preferences{NotificationsEnabled: true, MorningReviewTime: "06:00", EveningReviewTime: "18:00"}); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	want := settings.Preferences{NotificationsEnabled: false, MorningReviewTime: "10:00", EveningReviewTime: "20:00"}
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
