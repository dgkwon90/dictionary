package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"neulsang/desktopd/internal/domain/settings"
)

// app_settings key names (PRD §11.1). Only non-secret behavior policy is stored here.
const (
	settingNotificationsEnabled = "notifications_enabled"
	settingMorningReviewTime    = "morning_review_time"
	settingEveningReviewTime    = "evening_review_time"
)

// SettingsRepository reads/writes user preferences in the app_settings key-value table.
type SettingsRepository struct {
	db  *sql.DB
	now func() time.Time
}

func NewSettingsRepository(db *sql.DB) *SettingsRepository {
	return &SettingsRepository{db: db, now: time.Now}
}

// Load starts from Defaults() and overrides with any stored keys, so unset settings
// return their default rather than a zero value.
func (r *SettingsRepository) Load(ctx context.Context) (prefs settings.Preferences, resultErr error) {
	prefs = settings.Defaults()
	rows, err := r.db.QueryContext(ctx,
		`SELECT key, value FROM app_settings WHERE key IN (?, ?, ?)`,
		settingNotificationsEnabled, settingMorningReviewTime, settingEveningReviewTime)
	if err != nil {
		return settings.Preferences{}, fmt.Errorf("query app_settings: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close app_settings rows: %w", err)
		}
	}()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return settings.Preferences{}, fmt.Errorf("scan app_settings: %w", err)
		}
		switch key {
		case settingNotificationsEnabled:
			prefs.NotificationsEnabled = value == "true"
		case settingMorningReviewTime:
			prefs.MorningReviewTime = value
		case settingEveningReviewTime:
			prefs.EveningReviewTime = value
		}
	}
	if err := rows.Err(); err != nil {
		return settings.Preferences{}, fmt.Errorf("iterate app_settings: %w", err)
	}
	return prefs, nil
}

// Save upserts every preference key in one transaction (PUT = full replace).
func (r *SettingsRepository) Save(ctx context.Context, prefs settings.Preferences) (resultErr error) {
	now := r.now().UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin settings tx: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, tx.Rollback())
		}
	}()

	pairs := [...]struct{ key, value string }{
		{settingNotificationsEnabled, strconv.FormatBool(prefs.NotificationsEnabled)},
		{settingMorningReviewTime, prefs.MorningReviewTime},
		{settingEveningReviewTime, prefs.EveningReviewTime},
	}
	for _, p := range pairs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			p.key, p.value, now); err != nil {
			return fmt.Errorf("upsert %s: %w", p.key, err)
		}
	}
	return tx.Commit()
}
