package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"neulsang/desktopd/internal/config"
	httptransport "neulsang/desktopd/internal/transport/http"
)

const shutdownTimeout = 5 * time.Second

type App struct {
	cfg config.Config
	log *slog.Logger
	srv *http.Server

	ready      chan struct{}
	readyOnce  sync.Once
	addrMu     sync.RWMutex
	listenAddr string
	listenErr  error
}

func New(cfg config.Config, log *slog.Logger) *App {
	return &App{
		cfg: cfg,
		log: log,
		srv: &http.Server{
			Addr:    cfg.Addr,
			Handler: httptransport.NewRouter(log),
		},
		ready: make(chan struct{}),
	}
}

func (a *App) Addr() (string, error) {
	<-a.ready
	a.addrMu.RLock()
	defer a.addrMu.RUnlock()
	return a.listenAddr, a.listenErr
}

func (a *App) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", a.cfg.Addr)
	if err != nil {
		a.addrMu.Lock()
		a.listenErr = err
		a.addrMu.Unlock()
		a.readyOnce.Do(func() { close(a.ready) })
		return fmt.Errorf("listen on %s: %w", a.cfg.Addr, err)
	}

	a.addrMu.Lock()
	a.listenAddr = listener.Addr().String()
	a.addrMu.Unlock()
	a.readyOnce.Do(func() { close(a.ready) })
	a.log.Info("HTTP server listening", "addr", listener.Addr().String())

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- a.srv.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := a.srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}

	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}
