package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"neulsang/desktopd/internal/domain/explain"
	"neulsang/desktopd/internal/domain/review"
	"neulsang/desktopd/internal/domain/settings"
)

// app_settings key names (PRD §11.1). Only non-secret behavior policy is stored here.
const (
	settingNotificationsEnabled = "notifications_enabled"
	settingMorningReviewTime    = "morning_review_time"
	settingEveningReviewTime    = "evening_review_time"
	settingReviewAgainMinutes   = "review_again_minutes"
	settingReviewFirstHardDays  = "review_first_hard_days"
	settingReviewFirstGoodDays  = "review_first_good_days"
	settingReviewFirstEasyDays  = "review_first_easy_days"
	settingReviewHardMultiplier = "review_hard_multiplier"
	settingReviewGoodMultiplier = "review_good_multiplier"
	settingReviewEasyMultiplier = "review_easy_multiplier"
	settingAIPromptStyle        = "ai_prompt_style"
	settingAIExamplesCount      = "ai_examples_count"
)

// settingKeys is every key Load reads and Save writes. One list so the two can never
// disagree about what a stored preference set consists of.
var settingKeys = []string{
	settingNotificationsEnabled,
	settingMorningReviewTime,
	settingEveningReviewTime,
	settingReviewAgainMinutes,
	settingReviewFirstHardDays,
	settingReviewFirstGoodDays,
	settingReviewFirstEasyDays,
	settingReviewHardMultiplier,
	settingReviewGoodMultiplier,
	settingReviewEasyMultiplier,
	settingAIPromptStyle,
	settingAIExamplesCount,
}

// SettingsRepository reads/writes user preferences in the app_settings key-value table.
//
// It is the single storage for user policy, so the domains that consume one slice of
// it each get a narrow reader interface rather than the whole preference set.
type SettingsRepository struct {
	db  *sql.DB
	now func() time.Time
}

var (
	_ settings.Repository   = (*SettingsRepository)(nil)
	_ review.IntervalSource = (*SettingsRepository)(nil)
	_ explain.FormatSource  = (*SettingsRepository)(nil)
)

func NewSettingsRepository(db *sql.DB) *SettingsRepository {
	return &SettingsRepository{db: db, now: time.Now}
}

// Load starts from Defaults() and overrides with any stored keys, so unset settings
// return their default rather than a zero value.
func (r *SettingsRepository) Load(ctx context.Context) (prefs settings.Preferences, resultErr error) {
	prefs = settings.Defaults()
	rows, err := r.db.QueryContext(ctx,
		`SELECT key, value FROM app_settings WHERE key IN (`+placeholders(len(settingKeys))+`)`,
		anySlice(settingKeys)...)
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
		case settingReviewAgainMinutes:
			applyFloat(&prefs.ReviewIntervals.AgainMinutes, value)
		case settingReviewFirstHardDays:
			applyFloat(&prefs.ReviewIntervals.FirstHardDays, value)
		case settingReviewFirstGoodDays:
			applyFloat(&prefs.ReviewIntervals.FirstGoodDays, value)
		case settingReviewFirstEasyDays:
			applyFloat(&prefs.ReviewIntervals.FirstEasyDays, value)
		case settingReviewHardMultiplier:
			applyFloat(&prefs.ReviewIntervals.HardMultiplier, value)
		case settingReviewGoodMultiplier:
			applyFloat(&prefs.ReviewIntervals.GoodMultiplier, value)
		case settingReviewEasyMultiplier:
			applyFloat(&prefs.ReviewIntervals.EasyMultiplier, value)
		case settingAIPromptStyle:
			prefs.AIFormat.PromptStyle = value
		case settingAIExamplesCount:
			if count, err := strconv.Atoi(value); err == nil {
				prefs.AIFormat.ExamplesCount = count
			}
		}
	}
	if err := rows.Err(); err != nil {
		return settings.Preferences{}, fmt.Errorf("iterate app_settings: %w", err)
	}
	// The stored format is clamped on the way out. A value can predate a change to the
	// limits, or arrive from an imported snapshot, and neither should be able to fail a
	// lookup — the write boundary is where bad input is refused (explain.Format.Validate).
	prefs.AIFormat = prefs.AIFormat.Normalized()
	return prefs, nil
}

// applyFloat overwrites target only with a value that parses and is positive.
//
// Everything else — a truncated number, someone's hand edit, a key written by a future
// version in a format this one does not read — leaves the default in place. A settings
// row is policy, not data: the app answering with its default schedule is always better
// than it refusing to schedule at all because one row is unreadable.
func applyFloat(target *float64, value string) {
	if parsed, err := strconv.ParseFloat(value, 64); err == nil && parsed > 0 {
		*target = parsed
	}
}

// ReviewIntervals implements review.IntervalSource: grading reads the schedule through
// this rather than the whole preference set.
func (r *SettingsRepository) ReviewIntervals(ctx context.Context) (review.Intervals, error) {
	prefs, err := r.Load(ctx)
	if err != nil {
		return review.Intervals{}, err
	}
	return prefs.ReviewIntervals, nil
}

// ExplainFormat implements explain.FormatSource: a lookup reads the user's style
// through this rather than the whole preference set.
func (r *SettingsRepository) ExplainFormat(ctx context.Context) (explain.Format, error) {
	prefs, err := r.Load(ctx)
	if err != nil {
		return explain.Format{}, err
	}
	return prefs.AIFormat, nil
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

	intervals := prefs.ReviewIntervals
	pairs := [...]struct{ key, value string }{
		{settingNotificationsEnabled, strconv.FormatBool(prefs.NotificationsEnabled)},
		{settingMorningReviewTime, prefs.MorningReviewTime},
		{settingEveningReviewTime, prefs.EveningReviewTime},
		{settingReviewAgainMinutes, formatFloat(intervals.AgainMinutes)},
		{settingReviewFirstHardDays, formatFloat(intervals.FirstHardDays)},
		{settingReviewFirstGoodDays, formatFloat(intervals.FirstGoodDays)},
		{settingReviewFirstEasyDays, formatFloat(intervals.FirstEasyDays)},
		{settingReviewHardMultiplier, formatFloat(intervals.HardMultiplier)},
		{settingReviewGoodMultiplier, formatFloat(intervals.GoodMultiplier)},
		{settingReviewEasyMultiplier, formatFloat(intervals.EasyMultiplier)},
		{settingAIPromptStyle, prefs.AIFormat.PromptStyle},
		{settingAIExamplesCount, strconv.Itoa(prefs.AIFormat.ExamplesCount)},
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

// formatFloat writes the shortest text that reads back as the same number, so 3 stores
// as "3" and 1.2 as "1.2" rather than "3.000000".
func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func anySlice(values []string) []any {
	args := make([]any, len(values))
	for i, value := range values {
		args[i] = value
	}
	return args
}
