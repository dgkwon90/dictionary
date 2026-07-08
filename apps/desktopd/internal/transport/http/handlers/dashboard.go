package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"neulsang/desktopd/internal/domain/stats"
)

type DashboardService interface {
	Summary(ctx context.Context) (stats.Summary, error)
}

type Dashboard struct {
	svc DashboardService
	log *slog.Logger
}

func NewDashboard(svc DashboardService, log *slog.Logger) *Dashboard {
	return &Dashboard{svc: svc, log: log}
}

func (h *Dashboard) Summary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.svc.Summary(r.Context())
	if err != nil {
		h.log.Error("dashboard summary", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, dashboardSummaryResponse{
		TodaySearchCount:      summary.TodaySearchCount,
		WeekSearchCount:       summary.WeekSearchCount,
		TodayCompletedReviews: summary.TodayCompletedReviews,
		DueCardCount:          summary.DueCardCount,
		MostSearched:          toWordStats(summary.MostSearched),
		MostWrong:             toWordStats(summary.MostWrong),
		CategoryWeakness:      toCategoryWeakness(summary.CategoryWeakness),
	})
}

func toWordStats(words []stats.WordStat) []wordStatResponse {
	out := make([]wordStatResponse, 0, len(words))
	for _, word := range words {
		out = append(out, wordStatResponse{
			KnowledgeItemID: word.KnowledgeItemID,
			SurfaceText:     word.SurfaceText,
			Count:           word.Count,
		})
	}
	return out
}

func toCategoryWeakness(categories []stats.CategoryWeakness) []categoryWeaknessResponse {
	out := make([]categoryWeaknessResponse, 0, len(categories))
	for _, category := range categories {
		out = append(out, categoryWeaknessResponse{
			Category:      category.Category,
			ItemCount:     category.ItemCount,
			WeaknessScore: category.WeaknessScore,
		})
	}
	return out
}

type dashboardSummaryResponse struct {
	TodaySearchCount      int                        `json:"today_search_count"`
	WeekSearchCount       int                        `json:"week_search_count"`
	TodayCompletedReviews int                        `json:"today_completed_reviews"`
	DueCardCount          int                        `json:"due_card_count"`
	MostSearched          []wordStatResponse         `json:"most_searched"`
	MostWrong             []wordStatResponse         `json:"most_wrong"`
	CategoryWeakness      []categoryWeaknessResponse `json:"category_weakness"`
}

type wordStatResponse struct {
	KnowledgeItemID string `json:"knowledge_item_id"`
	SurfaceText     string `json:"surface_text"`
	Count           int    `json:"count"`
}

type categoryWeaknessResponse struct {
	Category      string  `json:"category"`
	ItemCount     int     `json:"item_count"`
	WeaknessScore float64 `json:"weakness_score"`
}
