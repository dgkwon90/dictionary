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
	"neulsang/desktopd/internal/domain/knowledge"
	"neulsang/desktopd/internal/domain/review"
	"neulsang/desktopd/internal/domain/stats"
	"neulsang/desktopd/internal/domain/suggest"
	"neulsang/desktopd/internal/infra/llm/gemini"
	httptransport "neulsang/desktopd/internal/transport/http"
	"neulsang/desktopd/internal/transport/http/handlers"
)

const (
	shutdownTimeout       = 5 * time.Second
	explainProcessTimeout = 90 * time.Second
)

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
			Handler:           httptransport.NewRouter(log, nil, nil, nil, nil, nil, nil, nil),
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
	explainCtx, cancelExplain := context.WithCancel(context.Background())
	var explainWG sync.WaitGroup
	defer func() {
		cancelExplain()
		explainWG.Wait()
		if err := a.db.Close(); err != nil {
			a.log.Error("failed to close database", "error", err)
		}
	}()
	captureRepo := sqlite.NewCaptureRepository(sqlDB)
	captureService := capture.NewService(captureRepo)
	explainRepo := sqlite.NewExplainRepository(sqlDB)
	explainer := a.newExplainer()
	explainService := explain.NewService(explainer, explainRepo)
	inboxRepo := sqlite.NewInboxRepository(sqlDB)
	inboxService := inbox.NewService(inboxRepo)
	knowledgeRepo := sqlite.NewKnowledgeRepository(sqlDB)
	knowledgeService := knowledge.NewService(knowledgeRepo)
	reviewRepo := sqlite.NewReviewRepository(sqlDB)
	reviewService := review.NewService(reviewRepo)
	statsRepo := sqlite.NewStatsRepository(sqlDB)
	statsService := stats.NewService(statsRepo)
	suggestService := suggest.NewService(a.newSuggester())
	captureHandler := handlers.NewCapture(explainingCaptureCreator{
		captureService: captureService,
		explainService: explainService,
		log:            a.log,
		baseCtx:        explainCtx,
		wg:             &explainWG,
	}, a.log)
	explanationHandler := handlers.NewExplanation(explainRepo, a.log)
	inboxHandler := handlers.NewInbox(inboxService, a.log)
	knowledgeHandler := handlers.NewKnowledge(knowledgeService, a.log)
	reviewHandler := handlers.NewReview(reviewService, a.log)
	dashboardHandler := handlers.NewDashboard(statsService, a.log)
	suggestHandler := handlers.NewSuggest(suggestService, a.log)
	a.srv.Handler = httptransport.NewRouter(a.log, captureHandler, explanationHandler, inboxHandler, knowledgeHandler, reviewHandler, dashboardHandler, suggestHandler)
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
		cancelExplain()
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
	baseCtx        context.Context
	wg             *sync.WaitGroup
}

func (c explainingCaptureCreator) Create(ctx context.Context, input capture.CreateInput) (capture.CreateResult, error) {
	result, err := c.captureService.Create(ctx, input)
	if err != nil {
		return capture.CreateResult{}, err
	}
	if c.wg != nil {
		c.wg.Add(1)
	}
	go func() {
		if c.wg != nil {
			defer c.wg.Done()
		}
		baseCtx := c.baseCtx
		if baseCtx == nil {
			baseCtx = context.Background()
		}
		explainCtx, cancel := context.WithTimeout(baseCtx, explainProcessTimeout)
		defer cancel()
		if err := c.explainService.Process(explainCtx, result.LookupJobID, result.CaptureID, input.Text); err != nil {
			c.log.Error("process explanation", "capture_id", result.CaptureID, "lookup_job_id", result.LookupJobID, "error", err)
		}
	}()
	return result, nil
}

// newExplainer selects the AI provider behind the explain.Explainer interface.
// NEULSANG_AI_PROVIDER pins a provider explicitly ("mock"/"gemini"); when empty,
// it auto-selects gemini if an API key is present, otherwise mock. Adding a new
// provider (openai/claude) is a new internal/infra/llm/<name> package + one case here.
func (a *App) newExplainer() explain.Explainer {
	provider := a.cfg.AIProvider
	if provider == "" {
		if a.cfg.GeminiAPIKey != "" {
			provider = "gemini"
		} else {
			provider = "mock"
		}
	}

	switch provider {
	case "gemini":
		return a.newGeminiExplainer()
	case "mock":
		return explain.NewMockExplainer()
	default:
		a.log.Warn("unknown NEULSANG_AI_PROVIDER, falling back to mock explainer", "provider", provider)
		return explain.NewMockExplainer()
	}
}

func (a *App) newGeminiExplainer() explain.Explainer {
	if a.cfg.GeminiAPIKey == "" {
		a.log.Warn("AI provider is gemini but NEULSANG_GEMINI_API_KEY not set, using mock explainer")
		return explain.NewMockExplainer()
	}

	model := gemini.DefaultModel
	opts := []gemini.Option{}
	if a.cfg.GeminiModel != "" {
		model = a.cfg.GeminiModel
		opts = append(opts, gemini.WithModel(a.cfg.GeminiModel))
	}
	a.log.Info("using Gemini explainer", "model", model)
	return gemini.New(a.cfg.GeminiAPIKey, opts...)
}

// newSuggester selects the AI provider behind suggest.Suggester, mirroring
// newExplainer: gemini when an API key is present (the gemini client implements both
// interfaces), otherwise the mock.
func (a *App) newSuggester() suggest.Suggester {
	provider := a.cfg.AIProvider
	if provider == "" {
		if a.cfg.GeminiAPIKey != "" {
			provider = "gemini"
		} else {
			provider = "mock"
		}
	}

	if provider == "gemini" && a.cfg.GeminiAPIKey != "" {
		model := gemini.DefaultModel
		opts := []gemini.Option{}
		if a.cfg.GeminiModel != "" {
			model = a.cfg.GeminiModel
			opts = append(opts, gemini.WithModel(a.cfg.GeminiModel))
		}
		a.log.Info("using Gemini suggester", "model", model)
		return gemini.New(a.cfg.GeminiAPIKey, opts...)
	}
	a.log.Info("using mock suggester (no Gemini API key)")
	return suggest.NewMockSuggester()
}

func (a *App) setStartupError(err error) {
	a.addrMu.Lock()
	a.listenErr = err
	a.addrMu.Unlock()
	a.readyOnce.Do(func() { close(a.ready) })
}
