package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"neulsang/desktopd/internal/config"
	"neulsang/desktopd/internal/db"
	"neulsang/desktopd/internal/db/sqlite"
	"neulsang/desktopd/internal/domain/capture"
	"neulsang/desktopd/internal/domain/explain"
	"neulsang/desktopd/internal/domain/inbox"
	httptransport "neulsang/desktopd/internal/transport/http"
	"neulsang/desktopd/internal/transport/http/handlers"
)

const shutdownTimeout = 5 * time.Second
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
)

type App struct {
	cfg config.Config
	log *slog.Logger
	srv *http.Server
	db  *sql.DB

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
			Addr:              cfg.Addr,
			Handler:           httptransport.NewRouter(log, nil, nil, nil),
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
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
	sqlDB, err := db.Open(a.cfg.DBPath)
	if err != nil {
		a.setStartupError(err)
		return fmt.Errorf("open database: %w", err)
	}
	if err := db.Migrate(ctx, sqlDB, a.log); err != nil {
		if closeErr := sqlDB.Close(); closeErr != nil {
			a.log.Error("failed to close database", "error", closeErr)
		}
		a.setStartupError(err)
		return fmt.Errorf("migrate database: %w", err)
	}
	a.db = sqlDB
	captureRepo := sqlite.NewCaptureRepository(sqlDB)
	captureService := capture.NewService(captureRepo)
	explainRepo := sqlite.NewExplainRepository(sqlDB)
	mockExplainer := explain.NewMockExplainer()
	explainService := explain.NewService(mockExplainer, explainRepo)
	inboxRepo := sqlite.NewInboxRepository(sqlDB)
	inboxService := inbox.NewService(inboxRepo)
	captureHandler := handlers.NewCapture(explainingCaptureCreator{
		captureService: captureService,
		explainService: explainService,
		log:            a.log,
	}, a.log)
	explanationHandler := handlers.NewExplanation(explainRepo, a.log)
	inboxHandler := handlers.NewInbox(inboxService, a.log)
	a.srv.Handler = httptransport.NewRouter(a.log, captureHandler, explanationHandler, inboxHandler)
	defer func() {
		if err := a.db.Close(); err != nil {
			a.log.Error("failed to close database", "error", err)
		}
	}()

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

type explainingCaptureCreator struct {
	captureService *capture.Service
	explainService *explain.Service
	log            *slog.Logger
}

func (c explainingCaptureCreator) Create(ctx context.Context, input capture.CreateInput) (capture.CreateResult, error) {
	result, err := c.captureService.Create(ctx, input)
	if err != nil {
		return capture.CreateResult{}, err
	}
	// TODO(#6): The mock provider returns immediately, so synchronous execution
	// keeps response latency negligible; revisit async workers with real provider latency and retries.
	if err := c.explainService.Process(ctx, result.LookupJobID, result.CaptureID, input.Text); err != nil {
		c.log.Error("process explanation", "capture_id", result.CaptureID, "lookup_job_id", result.LookupJobID, "error", err)
	}
	return result, nil
}

func (a *App) setStartupError(err error) {
	a.addrMu.Lock()
	a.listenErr = err
	a.addrMu.Unlock()
	a.readyOnce.Do(func() { close(a.ready) })
}
