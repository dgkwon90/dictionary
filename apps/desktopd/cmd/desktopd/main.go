package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"neulsang/desktopd/internal/app/bootstrap"
	"neulsang/desktopd/internal/config"
	"neulsang/desktopd/internal/logger"
	"neulsang/desktopd/internal/watchdog"
)

func main() {
	if err := config.LoadDotenv(); err != nil {
		slog.Warn("load .env file", "error", err)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 사이드카 모드(셸이 NEULSANG_PARENT_PID 설정)면 셸이 비정상 종료돼도 고아로
	// 남지 않도록 부모 소멸 시 위 컨텍스트와 함께 취소한다.
	ctx = watchdog.WatchParent(ctx, log)

	app := bootstrap.New(cfg, log)
	if err := app.Run(ctx); err != nil {
		log.Error("run desktopd", "error", err)
		os.Exit(1)
	}
}
