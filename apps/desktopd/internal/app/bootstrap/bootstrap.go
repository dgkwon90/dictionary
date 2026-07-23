package bootstrap

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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
	"neulsang/desktopd/internal/domain/backup"
	"neulsang/desktopd/internal/domain/capture"
	"neulsang/desktopd/internal/domain/explain"
	"neulsang/desktopd/internal/domain/inbox"
	"neulsang/desktopd/internal/domain/knowledge"
	"neulsang/desktopd/internal/domain/notification"
	"neulsang/desktopd/internal/domain/outbox"
	"neulsang/desktopd/internal/domain/review"
	"neulsang/desktopd/internal/domain/settings"
	"neulsang/desktopd/internal/domain/stats"
	"neulsang/desktopd/internal/domain/suggest"
	"neulsang/desktopd/internal/infra/llm/gemini"
	"neulsang/desktopd/internal/infra/phonetic"
	"neulsang/desktopd/internal/infra/syncpush"
	httptransport "neulsang/desktopd/internal/transport/http"
	"neulsang/desktopd/internal/transport/http/handlers"
)

const (
	shutdownTimeout       = 5 * time.Second
	explainProcessTimeout = 90 * time.Second
	// maxConcurrentExplains bounds how many explain (AI provider) calls run at once.
	// Without this, a burst of captures (bug, or an unauthenticated caller before
	// R-01's trust boundary lands) can spawn unbounded goroutines each holding a
	// Gemini call open for up to explainProcessTimeout — review R-01/R-08, RW-02.
	// Excess requests still succeed (capture + queued lookup_job are saved
	// immediately) and simply wait for a free slot before their explain call starts.
	maxConcurrentExplains = 3
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
	apiToken   string
}

// APIToken returns the bearer token every /v1/* request must present (review
// R-01), blocking until Run() has resolved it (generated one, if
// NEULSANG_API_TOKEN was unset) or failed to start. In production Tauri always
// sets NEULSANG_API_TOKEN before spawning this process and reads that same
// value back on its side — this accessor exists so in-process Go tests can
// learn the token when they build an App with an empty config.APIToken and
// exercise the auto-generate path.
func (a *App) APIToken() (string, error) {
	<-a.ready
	a.addrMu.RLock()
	defer a.addrMu.RUnlock()
	return a.apiToken, a.listenErr
}

func New(cfg config.Config, log *slog.Logger) *App {
	return &App{
		cfg: cfg,
		log: log,
		srv: &http.Server{
			Addr:              cfg.Addr,
			Handler:           httptransport.NewRouter(log, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil),
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
	if a.cfg.APIToken == "" {
		token, err := generateAPIToken()
		if err != nil {
			a.setStartupError(err)
			return fmt.Errorf("generate API token: %w", err)
		}
		a.cfg.APIToken = token
		a.log.Warn("NEULSANG_API_TOKEN not set — generated a session-only token for local/dev use; set NEULSANG_API_TOKEN to pin a fixed value", "token", token)
	}
	a.addrMu.Lock()
	a.apiToken = a.cfg.APIToken
	a.addrMu.Unlock()

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
	suggestRepo := sqlite.NewSuggestRepository(sqlDB)
	suggestService := suggest.NewService(a.newSuggester(), phonetic.NewMatcher(), suggestRepo)
	settingsRepo := sqlite.NewSettingsRepository(sqlDB)
	settingsService := settings.NewService(settingsRepo)
	notificationRepo := sqlite.NewNotificationRepository(sqlDB)
	notificationService := notification.NewService(notificationRepo, settingsRepo)
	backupRepo := sqlite.NewBackupRepository(sqlDB)
	backupService := backup.NewService(backupRepo)
	outboxRepo := sqlite.NewOutboxRepository(sqlDB)
	var publisher outbox.Publisher
	if a.cfg.SyncURL != "" {
		publisher = syncpush.NewClient(a.cfg.SyncURL)
	}
	outboxService := outbox.NewService(outboxRepo, publisher, a.log)
	captureHandler := handlers.NewCapture(explainingCaptureCreator{
		captureService: captureService,
		explainService: explainService,
		log:            a.log,
		baseCtx:        explainCtx,
		wg:             &explainWG,
		sem:            make(chan struct{}, maxConcurrentExplains),
	}, a.log)
	explanationHandler := handlers.NewExplanation(explainRepo, a.log)
	inboxHandler := handlers.NewInbox(inboxService, a.log)
	knowledgeHandler := handlers.NewKnowledge(knowledgeService, a.log)
	reviewHandler := handlers.NewReview(reviewService, a.log)
	dashboardHandler := handlers.NewDashboard(statsService, a.log)
	suggestHandler := handlers.NewSuggest(suggestService, a.log)
	settingsHandler := handlers.NewSettings(settingsService, a.effectiveConfig(), a.log)
	notificationHandler := handlers.NewNotification(notificationService, a.log)
	backupHandler := handlers.NewBackup(backupService, a.log)
	syncHandler := handlers.NewSync(outboxService, a.log)
	mux := httptransport.NewRouter(a.log, captureHandler, explanationHandler, inboxHandler, knowledgeHandler, reviewHandler, dashboardHandler, suggestHandler, settingsHandler, notificationHandler, backupHandler, syncHandler)
	a.srv.Handler = httptransport.Secure(mux, a.cfg.APIToken)

	// Review reminder scheduler (ADR-0008): enqueues review_due at the configured
	// morning/evening slots. Tied to explainCtx so it stops on shutdown.
	reminderScheduler := notification.NewScheduler(settingsRepo, notificationRepo, notificationRepo, a.log)
	explainWG.Add(1)
	go func() {
		defer explainWG.Done()
		reminderScheduler.Run(explainCtx)
	}()
	if a.cfg.SyncURL != "" {
		explainWG.Add(1)
		go func() {
			defer explainWG.Done()
			outboxService.Run(explainCtx)
		}()
	}
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
	// sem bounds concurrent explain calls (maxConcurrentExplains); nil disables the
	// limit (e.g. in tests that construct this struct directly without it).
	sem chan struct{}
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
		if c.sem != nil {
			select {
			case c.sem <- struct{}{}:
				defer func() { <-c.sem }()
			case <-baseCtx.Done():
				// Shutting down while waiting for a concurrency slot: give up rather
				// than block explainWG.Wait() indefinitely (this goroutine's Done()
				// above still fires via the deferred c.wg.Done()).
				c.log.Warn("skip explain: shutting down before a concurrency slot was available", "capture_id", result.CaptureID)
				return
			}
		}
		explainCtx, cancel := context.WithTimeout(baseCtx, explainProcessTimeout)
		defer cancel()
		if err := c.explainService.Process(explainCtx, result.LookupJobID, result.CaptureID, input.Text); err != nil {
			c.log.Error("process explanation", "capture_id", result.CaptureID, "lookup_job_id", result.LookupJobID, "error", err)
		}
	}()
	return result, nil
}

// resolveAIProvider is the single source of truth for AI provider selection —
// newExplainer, newSuggester, and effectiveConfig (Settings) all call this so they
// can never disagree (RW-06: newSuggester used to ignore NEULSANG_AI_PROVIDER=mock
// and check only API-key presence, so Settings could show "mock" while suggest still
// called Gemini). NEULSANG_AI_PROVIDER pins a provider explicitly ("mock"/"gemini");
// when empty, auto-selects gemini if an API key is present, otherwise mock. Gemini
// without a key, or any unrecognized value, degrades to mock (with a warning) —
// normalization happens once here, not per call site. Adding a new provider
// (openai/claude) is a new internal/infra/llm/<name> package + one case here.
func (a *App) resolveAIProvider() string {
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
		if a.cfg.GeminiAPIKey == "" {
			a.log.Warn("AI provider is gemini but NEULSANG_GEMINI_API_KEY not set, using mock")
			return "mock"
		}
		return "gemini"
	case "mock":
		return "mock"
	default:
		a.log.Warn("unknown NEULSANG_AI_PROVIDER, falling back to mock", "provider", provider)
		return "mock"
	}
}

func (a *App) newExplainer() explain.Explainer {
	if a.resolveAIProvider() != "gemini" {
		return explain.NewMockExplainer()
	}
	return a.newGeminiExplainer()
}

func (a *App) newGeminiExplainer() explain.Explainer {
	model := gemini.DefaultModel
	opts := []gemini.Option{}
	if a.cfg.GeminiModel != "" {
		model = a.cfg.GeminiModel
		opts = append(opts, gemini.WithModel(a.cfg.GeminiModel))
	}
	a.log.Info("using Gemini explainer", "model", model)
	return gemini.New(a.cfg.GeminiAPIKey, opts...)
}

// newSuggester selects the optional AI provider behind suggest.Suggester. The local
// phonetic matcher is wired separately as the fallback (suggest.Service), so mock
// provider or a missing Gemini key both simply disable the AI phase.
func (a *App) newSuggester() suggest.Suggester {
	if a.resolveAIProvider() != "gemini" {
		a.log.Info("Gemini suggester disabled; using local phonetic fallback")
		return nil
	}

	model := gemini.DefaultModel
	opts := []gemini.Option{}
	if a.cfg.GeminiModel != "" {
		model = a.cfg.GeminiModel
		opts = append(opts, gemini.WithModel(a.cfg.GeminiModel))
	}
	a.log.Info("using Gemini suggester", "model", model)
	return gemini.New(a.cfg.GeminiAPIKey, opts...)
}

// resolvedProvider reports the provider actually in effect. Used only to reflect
// read-only config on the Settings screen.
func (a *App) resolvedProvider() string {
	return a.resolveAIProvider()
}

// effectiveConfig is the read-only infra config shown on the Settings screen. The API
// key is reported as a presence flag only, never its value (ADR-0004 부록, #17).
func (a *App) effectiveConfig() handlers.EffectiveConfig {
	provider := a.resolvedProvider()
	// Only report a model when Gemini is actually in effect; otherwise the UI would
	// show a Gemini model name under a mock provider (codex #17 note).
	model := ""
	if provider == "gemini" {
		model = a.cfg.GeminiModel
		if model == "" {
			model = gemini.DefaultModel
		}
	}
	return handlers.EffectiveConfig{
		Addr:             a.cfg.Addr,
		DBPath:           a.cfg.DBPath,
		AIProvider:       provider,
		GeminiModel:      model,
		APIKeyConfigured: a.cfg.GeminiAPIKey != "",
	}
}

func (a *App) setStartupError(err error) {
	a.addrMu.Lock()
	a.listenErr = err
	a.addrMu.Unlock()
	a.readyOnce.Do(func() { close(a.ready) })
}

// generateAPIToken produces a random session token (review R-01) when
// NEULSANG_API_TOKEN was not set — 32 bytes of crypto/rand, hex-encoded, so it's
// safe to log and paste into an Authorization header without escaping.
func generateAPIToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
