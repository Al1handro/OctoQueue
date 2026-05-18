package pgsql

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

func NewStorage(ctx context.Context, dsn string, log *slog.Logger) (*Storage, error) {
	const op = "storage.pgsql.NewStorage"

	log.Info("Connecting to PostgreSQL database", "dsn", dsn)
	
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	err = pool.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	log.Info("Successfully connected to PostgreSQL database")

	log.Info("Initializing the database schema")

	queries := []string{
		// `CREATE TABLE IF NOT EXISTS users (
        //     id         BIGINT PRIMARY KEY,
        //     username   TEXT,
        //     created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        // )`,
		// `CREATE TABLE IF NOT EXISTS quotes (
        //     id          SERIAL PRIMARY KEY,
        //     text        TEXT NOT NULL,
        //     author      TEXT,
        //     share_token TEXT UNIQUE NOT NULL,
        //     created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		// 	url 		TEXT
        // )`,
		// `CREATE TABLE IF NOT EXISTS user_quote (
        //     user_id    BIGINT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
        //     quote_id   BIGINT NOT NULL REFERENCES quotes(id)  ON DELETE CASCADE,
        //     PRIMARY KEY (user_id, quote_id)
        // )`,
		// `CREATE INDEX IF NOT EXISTS idx_user_quote_user_id  ON user_quote(user_id)`,
		// `CREATE INDEX IF NOT EXISTS idx_user_quote_quote_id ON user_quote(quote_id)`,
		// `CREATE INDEX IF NOT EXISTS idx_quotes_share_token  ON quotes(share_token)`,
	}

	for _, q := range queries {
		if _, err := pool.Exec(ctx, q); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
	}

	return &Storage{pool: pool}, nil
}