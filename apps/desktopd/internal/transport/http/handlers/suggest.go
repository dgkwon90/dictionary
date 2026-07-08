package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"neulsang/desktopd/internal/domain/suggest"
)

type SuggestService interface {
	Suggest(ctx context.Context, query string) ([]suggest.Candidate, error)
}

type Suggest struct {
	svc SuggestService
	log *slog.Logger
}

func NewSuggest(svc SuggestService, log *slog.Logger) *Suggest {
	return &Suggest{svc: svc, log: log}
}

// Get handles GET /v1/suggest?q=<hangul> (backlog #21): infer English candidates
// from a Korean phonetic query for the user to pick from.
func (h *Suggest) Get(w http.ResponseWriter, r *http.Request) {
	candidates, err := h.svc.Suggest(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		switch {
		case errors.Is(err, suggest.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, suggest.ErrUnavailable):
			writeError(w, http.StatusServiceUnavailable, "suggestion is unavailable")
		default:
			h.log.Error("suggest candidates", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	responseCandidates := make([]suggestCandidateResponse, 0, len(candidates))
	for _, candidate := range candidates {
		responseCandidates = append(responseCandidates, suggestCandidateResponse{
			English:    candidate.English,
			Confidence: candidate.Confidence,
			GlossKo:    candidate.GlossKo,
		})
	}
	writeJSON(w, http.StatusOK, suggestResponse{Candidates: responseCandidates})
}

type suggestResponse struct {
	Candidates []suggestCandidateResponse `json:"candidates"`
}

type suggestCandidateResponse struct {
	English    string  `json:"english"`
	Confidence float64 `json:"confidence"`
	GlossKo    string  `json:"gloss_ko"`
}
