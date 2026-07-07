package http

import (
	"log/slog"
	nethttp "net/http"

	"neulsang/desktopd/internal/transport/http/handlers"
)

func NewRouter(log *slog.Logger, captureHandler *handlers.Capture) *nethttp.ServeMux {
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
	return mux
}
