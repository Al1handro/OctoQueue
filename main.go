package main

import (
	"context"
	"database/sql"
	"embed"
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
	"OctoQueue/internal/lib/logger/sl"
	"OctoQueue/internal/storage/migrator"
	"OctoQueue/internal/storage/pgsql"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

//go:embed migrations
var migrationsFS embed.FS

func main() {
	cfg := config.MustLode()

	log := setupLogger(cfg.Env)

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.PsqlInfo.User, cfg.PsqlInfo.Password, cfg.PsqlInfo.Рost, cfg.PsqlInfo.Port, cfg.PsqlInfo.Dbname)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Error("failed to open db for migrations", sl.Err(err))
		os.Exit(1)
	}

	m := migrator.MustGetNewMigrator(migrationsFS, "migrations")
	if err := m.ApplyMigrations(db); err != nil {
		log.Error("failed to apply migrations", sl.Err(err))
		os.Exit(1)
	}
	db.Close()

	log.Info("Migrations applied successfully")

	ctx := context.Background()
	storage, err := pgsql.NewStorage(ctx, dsn, log)
	if err != nil {
		log.Error("failed to create storage", sl.Err(err))
		os.Exit(1)
	}
	defer storage.Close()

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

	srv := &http.Server{
		Addr:         cfg.Adress,
		Handler:      router,
		ReadTimeout:  cfg.HttpServer.Timeout,
		WriteTimeout: cfg.HttpServer.Timeout,
		IdleTimeout:  cfg.HttpServer.IdleTimeout,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("Starting HTTP server", slog.String("addr", cfg.Adress))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("failed to start server", sl.Err(err))
		}
	}()

	log.Info("Server started")
	<-done
	log.Info("Stopping server")

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("failed to shutdown server", sl.Err(err))
	}
}

func setupLogger(env string) *slog.Logger {
	switch env {
	case envLocal:
		return setupPrettySlog()
	case envDev:
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case envProd:
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	default:
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
}

func setupPrettySlog() *slog.Logger {
	opts := slogpretty.PrettyHandlerOptions{
		SlogOpts: &slog.HandlerOptions{
			Level: slog.LevelDebug,
		},
	}
	return slog.New(opts.NewPrettyHandler(os.Stdout))
}