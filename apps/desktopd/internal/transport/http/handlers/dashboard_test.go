package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"neulsang/desktopd/internal/domain/stats"
)

type fakeDashboardService struct {
	summary func(context.Context) (stats.Summary, error)
}

func (f fakeDashboardService) Summary(ctx context.Context) (stats.Summary, error) {
	return f.summary(ctx)
}

func TestDashboardSummaryOK(t *testing.T) {
	handler := NewDashboard(fakeDashboardService{summary: func(_ context.Context) (stats.Summary, error) {
		return stats.Summary{
			TodaySearchCount:      3,
			WeekSearchCount:       9,
			TodayCompletedReviews: 4,
			DueCardCount:          2,
			MostSearched:          []stats.WordStat{{KnowledgeItemID: "k1", SurfaceText: "stale", Count: 5}},
			MostWrong:             []stats.WordStat{{KnowledgeItemID: "k2", SurfaceText: "race", Count: 3}},
			CategoryWeakness:      []stats.CategoryWeakness{{Category: "backend", ItemCount: 2, WeaknessScore: 1.5}},
		}, nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/dashboard/summary", nil)

	handler.Summary(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body struct {
		TodaySearchCount      int `json:"today_search_count"`
		WeekSearchCount       int `json:"week_search_count"`
		TodayCompletedReviews int `json:"today_completed_reviews"`
		DueCardCount          int `json:"due_card_count"`
		MostSearched          []struct {
			SurfaceText string `json:"surface_text"`
			Count       int    `json:"count"`
		} `json:"most_searched"`
		CategoryWeakness []struct {
			Category      string  `json:"category"`
			WeaknessScore float64 `json:"weakness_score"`
		} `json:"category_weakness"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TodaySearchCount != 3 || body.WeekSearchCount != 9 || body.DueCardCount != 2 || body.TodayCompletedReviews != 4 {
		t.Fatalf("counts = %#v", body)
	}
	if len(body.MostSearched) != 1 || body.MostSearched[0].SurfaceText != "stale" || body.MostSearched[0].Count != 5 {
		t.Fatalf("most_searched = %#v", body.MostSearched)
	}
	if len(body.CategoryWeakness) != 1 || body.CategoryWeakness[0].Category != "backend" || body.CategoryWeakness[0].WeaknessScore != 1.5 {
		t.Fatalf("category_weakness = %#v", body.CategoryWeakness)
	}
}

func TestDashboardSummaryError(t *testing.T) {
	handler := NewDashboard(fakeDashboardService{summary: func(_ context.Context) (stats.Summary, error) {
		return stats.Summary{}, errors.New("boom")
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/dashboard/summary", nil)

	handler.Summary(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}
