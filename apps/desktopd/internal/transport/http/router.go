package http

import (
	"log/slog"
	nethttp "net/http"

	"neulsang/desktopd/internal/transport/http/handlers"
)

func NewRouter(log *slog.Logger, captureHandler *handlers.Capture, explanationHandler *handlers.Explanation, inboxHandler *handlers.Inbox, knowledgeHandler *handlers.Knowledge, reviewHandler *handlers.Review, dashboardHandler *handlers.Dashboard, suggestHandler *handlers.Suggest, settingsHandler *handlers.Settings, notificationHandler *handlers.Notification, backupHandler *handlers.Backup, syncHandler *handlers.Sync) *nethttp.ServeMux {
	mux := nethttp.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(nethttp.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			log.Error("write health response", "error", err)
		}
	})
	if captureHandler != nil {
		mux.HandleFunc("POST /v1/captures", captureHandler.Create)
	}
	if explanationHandler != nil {
		mux.HandleFunc("GET /v1/captures/{id}/explanation", explanationHandler.Get)
	}
	if inboxHandler != nil {
		mux.HandleFunc("GET /v1/inbox", inboxHandler.List)
		mux.HandleFunc("POST /v1/inbox/{id}/save", inboxHandler.Save)
		mux.HandleFunc("POST /v1/inbox/{id}/archive", inboxHandler.Archive)
	}
	if knowledgeHandler != nil {
		mux.HandleFunc("GET /v1/captures/{id}/knowledge", knowledgeHandler.ListByCapture)
		mux.HandleFunc("POST /v1/knowledge/{id}/mark-unknown", knowledgeHandler.MarkUnknown)
		mux.HandleFunc("POST /v1/knowledge/{id}/mark-known", knowledgeHandler.MarkKnown)
	}
	if reviewHandler != nil {
		mux.HandleFunc("GET /v1/reviews/due", reviewHandler.Due)
		mux.HandleFunc("POST /v1/reviews/session/start", reviewHandler.StartSession)
		mux.HandleFunc("POST /v1/reviews/{id}/grade", reviewHandler.Grade)
	}
	if dashboardHandler != nil {
		mux.HandleFunc("GET /v1/dashboard/summary", dashboardHandler.Summary)
	}
	if suggestHandler != nil {
		mux.HandleFunc("GET /v1/suggest", suggestHandler.Get)
		mux.HandleFunc("POST /v1/suggest/confirm", suggestHandler.Confirm)
	}
	if settingsHandler != nil {
		mux.HandleFunc("GET /v1/settings", settingsHandler.Get)
		mux.HandleFunc("PUT /v1/settings", settingsHandler.Update)
	}
	if notificationHandler != nil {
		mux.HandleFunc("GET /v1/notifications", notificationHandler.List)
		mux.HandleFunc("POST /v1/notifications/{id}/ack", notificationHandler.Ack)
		mux.HandleFunc("POST /v1/captures/{id}/notification-ack", notificationHandler.AckByCapture)
	}
	if backupHandler != nil {
		mux.HandleFunc("GET /v1/export", backupHandler.Export)
		mux.HandleFunc("POST /v1/import", backupHandler.Import)
		mux.HandleFunc("POST /v1/backup", backupHandler.BackupFile)
	}
	if syncHandler != nil {
		mux.HandleFunc("GET /v1/sync/status", syncHandler.Status)
	}
	return mux
}
