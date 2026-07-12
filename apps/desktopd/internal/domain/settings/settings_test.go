package settings

import (
	"context"
	"errors"
	"testing"
)

type fakeRepo struct {
	loaded Preferences
	saved  *Preferences
	err    error
}

func (f *fakeRepo) Load(context.Context) (Preferences, error) {
	return f.loaded, f.err
}

func (f *fakeRepo) Save(_ context.Context, prefs Preferences) error {
	if f.err != nil {
		return f.err
	}
	f.saved = &prefs
	return nil
}

func TestDefaultsAreValid(t *testing.T) {
	if err := Defaults().Validate(); err != nil {
		t.Fatalf("Defaults() invalid: %v", err)
	}
}

func TestPreferencesValidate(t *testing.T) {
	base := Defaults()
	cases := []struct {
		name    string
		morning string
		evening string
		wantErr bool
	}{
		{"ok", "09:00", "21:30", false},
		{"midnight and last minute", "00:00", "23:59", false},
		{"hour 24 invalid", "24:00", "21:00", true},
		{"minute 60 invalid", "09:60", "21:00", true},
		{"missing colon", "0900", "21:00", true},
		{"empty", "", "21:00", true},
		{"evening invalid", "09:00", "9:00", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			p.MorningReviewTime = tc.morning
			p.EveningReviewTime = tc.evening
			err := p.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate() = nil, want error")
				}
				if !errors.Is(err, ErrInvalidPreferences) {
					t.Fatalf("Validate() error = %v, want wraps ErrInvalidPreferences", err)
				}
			} else if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestServiceGet(t *testing.T) {
	want := Preferences{NotificationsEnabled: false, MorningReviewTime: "08:00", EveningReviewTime: "20:00"}
	svc := NewService(&fakeRepo{loaded: want})
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != want {
		t.Fatalf("Get() = %+v, want %+v", got, want)
	}
}

func TestServiceUpdateValidatesBeforeSaving(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	_, err := svc.Update(context.Background(), Preferences{MorningReviewTime: "bad", EveningReviewTime: "21:00"})
	if !errors.Is(err, ErrInvalidPreferences) {
		t.Fatalf("Update() error = %v, want ErrInvalidPreferences", err)
	}
	if repo.saved != nil {
		t.Fatalf("Update() persisted invalid preferences: %+v", *repo.saved)
	}
}

func TestServiceUpdatePersists(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	in := Preferences{NotificationsEnabled: true, MorningReviewTime: "07:15", EveningReviewTime: "22:45"}
	got, err := svc.Update(context.Background(), in)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got != in {
		t.Fatalf("Update() = %+v, want %+v", got, in)
	}
	if repo.saved == nil || *repo.saved != in {
		t.Fatalf("Update() saved = %+v, want %+v", repo.saved, in)
	}
}
