package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"microapi/internal/config"
	"microapi/internal/database"
	"microapi/internal/server"
)

var version string = "dev"

func main() {
	// Structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	db, err := database.Open(cfg)
	if err != nil {
		logger.Error("failed to open database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		logger.Error("failed to migrate database", slog.String("error", err.Error()))
		os.Exit(1)
	}

	srv := server.New(cfg, db)

	go func() {
		logger.Info("microapi starting server", slog.String("port", cfg.Port))
		logger.Info("microapi version", slog.String("version", version))
		if err := http.ListenAndServe(":"+cfg.Port, srv); err != nil && err != http.ErrServerClosed {
			logger.Error("microapi server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	logger.Info("server stopped")
}
