package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Storage struct {
	pool *pgxpool.Pool
}

// Идеоматичней использовать методы, так как это не ломает инкапсуляцию
func (s *Storage) Pool() *pgxpool.Pool {
	return s.pool
}

func (s *Storage) Close() {
	s.pool.Close()
}

func NewStorage(ctx context.Context, dsn string, log *slog.Logger) (*Storage, error) {
    const op = "storage.pgsql.NewStorage"

    log.Info("Connecting to PostgreSQL database", "dsn", dsn)

    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    pool, err := pgxpool.New(ctx, dsn)
    if err != nil {
        return nil, fmt.Errorf("%s: connect: %w", op, err)
    }

    if err = pool.Ping(ctx); err != nil {
        return nil, fmt.Errorf("%s: ping: %w", op, err)
    }

    log.Info("Successfully connected to PostgreSQL database")
    return &Storage{pool: pool}, nil
}