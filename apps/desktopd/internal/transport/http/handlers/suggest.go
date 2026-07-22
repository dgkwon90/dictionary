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
	ConfirmPick(ctx context.Context, query, english, glossKo string) error
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
			Source:     candidate.Source,
		})
	}
	writeJSON(w, http.StatusOK, suggestResponse{Candidates: responseCandidates})
}

// Confirm handles POST /v1/suggest/confirm {query, english, gloss_ko}: records the
// user's chosen candidate so the same query is answered from cache next time (#21 P2).
func (h *Suggest) Confirm(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Query   string `json:"query"`
		English string `json:"english"`
		GlossKo string `json:"gloss_ko"`
	}
	if err := decodeJSONBody(w, r, &request, 1<<20, h.log); err != nil {
		writeJSONDecodeError(w, err)
		return
	}

	if err := h.svc.ConfirmPick(r.Context(), request.Query, request.English, request.GlossKo); err != nil {
		switch {
		case errors.Is(err, suggest.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, suggest.ErrUnavailable):
			writeError(w, http.StatusServiceUnavailable, "suggestion cache is unavailable")
		default:
			h.log.Error("confirm suggest pick", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type suggestResponse struct {
	Candidates []suggestCandidateResponse `json:"candidates"`
}

type suggestCandidateResponse struct {
	English    string  `json:"english"`
	Confidence float64 `json:"confidence"`
	GlossKo    string  `json:"gloss_ko"`
	Source     string  `json:"source"`
}
