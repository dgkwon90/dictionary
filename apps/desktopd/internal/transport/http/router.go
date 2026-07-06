package http

import (
	"log/slog"
	nethttp "net/http"
)

func NewRouter(log *slog.Logger) *nethttp.ServeMux {
	mux := nethttp.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(nethttp.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			log.Error("write health response", "error", err)
		}
	})
	return mux
}
