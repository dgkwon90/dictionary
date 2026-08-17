package http

import (
	"log/slog"
	nethttp "net/http"

	"neulsang/desktopd/internal/transport/http/handlers"
)

// Set holds every HTTP handler the router can mount. A nil field means that
// handler's routes are not registered at all (the request 404s), which is how
// bootstrap builds a healthz-only server before the real dependencies exist.
//
// This is a struct rather than a positional argument list on purpose: the router
// mounts a dozen handler groups, and with positional arguments every new domain
// shifted the meaning of every call site — including the nil placeholders in
// bootstrap and the tests — with nothing but argument count to catch a mistake.
type Set struct {
	Capture      *handlers.Capture
	Explanation  *handlers.Explanation
	Search       *handlers.Search
	Learning     *handlers.Learning
	Review       *handlers.Review
	Dashboard    *handlers.Dashboard
	Suggest      *handlers.Suggest
	Settings     *handlers.Settings
	Notification *handlers.Notification
	Backup       *handlers.Backup
	Sync         *handlers.Sync
}

func NewRouter(log *slog.Logger, h Set) *nethttp.ServeMux {
	mux := nethttp.NewServeMux()
	mux.HandleFunc("GET /healthz", writeHealth(log))
	// 인증이 필요한 liveness. `/healthz`는 Secure에서 면제라 토큰 없이도 200이므로
	// "이 포트에 뭔가 떠 있다"까지만 알려준다 — 셸에겐 그것으로 부족하다. 사이드카가
	// 포트를 못 잡고 죽고 그 포트를 **다른 Neulsang 인스턴스**가 쥐고 있는 경우에도
	// `/healthz`는 200을 주고, 그 뒤 모든 /v1/*가 401로 깨지기 때문이다. 이 경로는
	// Secure를 통과하므로 200은 "이 서버가 내 토큰을 받아준다"까지 보증한다.
	mux.HandleFunc("GET /v1/healthz", writeHealth(log))
	if h.Capture != nil {
		mux.HandleFunc("POST /v1/captures", h.Capture.Create)
	}
	if h.Explanation != nil {
		mux.HandleFunc("GET /v1/captures/{id}/explanation", h.Explanation.Get)
	}
	if h.Search != nil {
		mux.HandleFunc("GET /v1/searches", h.Search.List)
		mux.HandleFunc("GET /v1/searches/{id}", h.Search.Get)
		mux.HandleFunc("POST /v1/searches/{id}/open", h.Search.Open)
		mux.HandleFunc("POST /v1/searches/{id}/learn", h.Search.Learn)
		mux.HandleFunc("POST /v1/searches/{id}/discard", h.Search.Discard)
		mux.HandleFunc("POST /v1/searches/{id}/kind", h.Search.SetKind)
		mux.HandleFunc("POST /v1/searches/{id}/retry", h.Search.Retry)
		mux.HandleFunc("POST /v1/searches/{id}/selections", h.Search.Select)
		mux.HandleFunc("DELETE /v1/searches/{id}/selections/{knowledgeItemId}", h.Search.Deselect)
		mux.HandleFunc("POST /v1/searches/{id}/selections/complete", h.Search.CompleteSelection)
	}
	if h.Learning != nil {
		mux.HandleFunc("GET /v1/learning", h.Learning.List)
		mux.HandleFunc("POST /v1/learning/{id}/retire", h.Learning.Retire)
		mux.HandleFunc("DELETE /v1/learning/{id}", h.Learning.Remove)
		mux.HandleFunc("POST /v1/learning/{id}/restore", h.Learning.Restore)
	}
	if h.Review != nil {
		mux.HandleFunc("GET /v1/reviews/due", h.Review.Due)
		mux.HandleFunc("GET /v1/practice/cards", h.Review.PracticeCards)
		mux.HandleFunc("POST /v1/practice/{id}/grade", h.Review.GradePractice)
		mux.HandleFunc("POST /v1/reviews/{id}/grade", h.Review.Grade)
	}
	if h.Dashboard != nil {
		mux.HandleFunc("GET /v1/dashboard/summary", h.Dashboard.Summary)
	}
	if h.Suggest != nil {
		mux.HandleFunc("GET /v1/suggest", h.Suggest.Get)
		mux.HandleFunc("POST /v1/suggest/confirm", h.Suggest.Confirm)
	}
	if h.Settings != nil {
		mux.HandleFunc("GET /v1/settings", h.Settings.Get)
		mux.HandleFunc("PUT /v1/settings", h.Settings.Update)
		mux.HandleFunc("GET /v1/settings/ai-format/sample", h.Settings.AIFormatSample)
	}
	if h.Notification != nil {
		mux.HandleFunc("GET /v1/notifications", h.Notification.List)
		mux.HandleFunc("GET /v1/notifications/history", h.Notification.History)
		mux.HandleFunc("POST /v1/notifications/{id}/ack", h.Notification.Ack)
		mux.HandleFunc("DELETE /v1/notifications/{id}", h.Notification.Delete)
		mux.HandleFunc("DELETE /v1/notifications", h.Notification.DeleteAll)
		mux.HandleFunc("POST /v1/captures/{id}/notification-ack", h.Notification.AckByCapture)
	}
	if h.Backup != nil {
		mux.HandleFunc("GET /v1/export", h.Backup.Export)
		mux.HandleFunc("POST /v1/import", h.Backup.Import)
		mux.HandleFunc("POST /v1/backup", h.Backup.BackupFile)
	}
	if h.Sync != nil {
		mux.HandleFunc("GET /v1/sync/status", h.Sync.Status)
	}
	return mux
}

// writeHealth builds the handler both liveness paths share. They differ only in
// whether Secure lets an unauthenticated request through, not in what they say.
func writeHealth(log *slog.Logger) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(nethttp.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			log.Error("write health response", "error", err)
		}
	}
}
