package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"OctoQueue/internal/config"
	"OctoQueue/internal/http-server/handlers"
	"OctoQueue/internal/http-server/middleware/logger"
	"OctoQueue/internal/lib/logger/handlers/slogpretty"
	"OctoQueue/internal/storage/pgsql"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

func main() {
	cfg := config.MustLode()

	dns := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.PsqlInfo.User, cfg.PsqlInfo.Password, cfg.PsqlInfo.Рost, cfg.PsqlInfo.Port, cfg.PsqlInfo.Dbname)

	log := setupLogger(cfg.Env)

	storage, err := pgsql.NewStorage(context.Background(), dns, log)
	if err != nil {
		log.Error("Failed to create storage", "error", err.Error())
		os.Exit(1)
	}

	// _ = storage // TODO: Use storage in handlers

	log.Info("Application started")

	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(logger.New(log))
	router.Use(middleware.Recoverer)
	router.Use(middleware.URLFormat)

	router.Get("/status", handlers.Status(log))
	router.Post("/tasks", handlers.CreateTask(log, storage))
	router.Get("/tasks", handlers.ListTasks(log, storage))
	router.Get("/tasks/{id}", handlers.GetTask(log, storage))
	router.Patch("/tasks/{id}", handlers.UpdateTask(log, storage))
	router.Delete("/tasks/{id}", handlers.DeleteTask(log, storage))
	router.Get("/tasks/{id}/executions", handlers.GetTaskExecutions(log, storage))
	
	log.Info("Starting HTTP server on :8080")
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Error("Failed to start HTTP server", "error", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	srv := &http.Server{
		Addr:         cfg.Adress,
		Handler:      router,
		ReadTimeout:  cfg.HttpServer.Timeout,
		WriteTimeout: cfg.HttpServer.Timeout,
		IdleTimeout:  cfg.HttpServer.IdleTimeout,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Error("failed to start server")
		}
	}()

	log.Info("server started")

	<-done
	log.Info("stopping server")
}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger

	switch env {
	case envLocal:
		log = setupPrettySlog()
	case envDev:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	case envProd:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		)
	default: // If env config is invalid, set prod settings by default due to security
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		)
	}

	return log
}

func setupPrettySlog() *slog.Logger {
	opts := slogpretty.PrettyHandlerOptions{
		SlogOpts: &slog.HandlerOptions{
			Level: slog.LevelDebug,
		},
	}

	handler := opts.NewPrettyHandler(os.Stdout)

	return slog.New(handler)
}
