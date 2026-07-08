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

	app := bootstrap.New(cfg, log)
	if err := app.Run(ctx); err != nil {
		log.Error("run desktopd", "error", err)
		os.Exit(1)
	}
}
