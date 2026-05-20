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
		return nil, fmt.Errorf("%s: connect: %w", op, err)
	}

	err = pool.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: ping: %w", op, err)
	}

	log.Info("Successfully connected to PostgreSQL database")

	log.Info("Initializing the database schema")

	queries := []string{
		// Функция автообновления updated_at
		`CREATE OR REPLACE FUNCTION update_timestamp()
		RETURNS TRIGGER AS $$
		BEGIN
			NEW.updated_at = NOW();
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`,

		// Таблица tasks
		`CREATE TABLE IF NOT EXISTS tasks (
        id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        name           VARCHAR(255) NOT NULL,
        type           VARCHAR(50)  NOT NULL CHECK (type IN ('http_call', 'shell', 'grpc')),

        payload        BYTEA        NOT NULL,
        payload_type   VARCHAR(100) NOT NULL,
        schema_version INT          NOT NULL DEFAULT 1,

        schedule       VARCHAR(100),
        timezone       VARCHAR(50)  NOT NULL DEFAULT 'UTC',

        status         VARCHAR(20)  NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
        next_run_at    TIMESTAMPTZ,
        last_run_at    TIMESTAMPTZ,
        started_at     TIMESTAMPTZ,

        retries        INT          NOT NULL DEFAULT 0,
        max_retries    INT          NOT NULL DEFAULT 3,
        retry_delay    INTERVAL     NOT NULL DEFAULT '1 minute',
        timeout        INTERVAL     NOT NULL DEFAULT '30 seconds',

        target_host    VARCHAR(255),

        tags           TEXT[]       NOT NULL DEFAULT '{}',
        created_by     VARCHAR(100),
        created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
        updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
        deleted_at     TIMESTAMPTZ,

        CONSTRAINT chk_schedule CHECK (
            schedule IS NULL OR schedule ~ '^(\*|[\d\-,/]+)\s+(\*|[\d\-,/]+)\s+(\*|[\d\-,/]+)\s+(\*|[\d\-,/]+)\s+(\*|[\d\-,/]+)$'
        )
    )`,

		// Индексы tasks
		`CREATE INDEX IF NOT EXISTS idx_tasks_status
        ON tasks(status) WHERE deleted_at IS NULL`,

		`CREATE INDEX IF NOT EXISTS idx_tasks_next_run
        ON tasks(next_run_at) WHERE status = 'pending' AND deleted_at IS NULL`,

		`CREATE INDEX IF NOT EXISTS idx_tasks_type
        ON tasks(type) WHERE deleted_at IS NULL`,

		`CREATE INDEX IF NOT EXISTS idx_tasks_tags
        ON tasks USING GIN(tags) WHERE deleted_at IS NULL`,

		`CREATE INDEX IF NOT EXISTS idx_tasks_created_at
        ON tasks(created_at DESC)`,

		`CREATE INDEX IF NOT EXISTS idx_tasks_target_host
        ON tasks(target_host) WHERE deleted_at IS NULL`,

		// Триггер updated_at для tasks
		`DO $$ BEGIN
        IF NOT EXISTS (
            SELECT 1 FROM pg_trigger
            WHERE tgname = 'trg_tasks_updated_at'
        ) THEN
            CREATE TRIGGER trg_tasks_updated_at
                BEFORE UPDATE ON tasks
                FOR EACH ROW
                EXECUTE FUNCTION update_timestamp();
        END IF;
    END $$`,

		// Таблица task_executions
		`CREATE TABLE IF NOT EXISTS task_executions (
        id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
        task_id       UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,

        status        VARCHAR(20) NOT NULL DEFAULT 'running'
                      CHECK (status IN ('running', 'success', 'failed', 'timeout', 'cancelled')),
        attempt       INT         NOT NULL DEFAULT 1,

        started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        finished_at   TIMESTAMPTZ,
        duration      INTERVAL,

        request       BYTEA,
        request_type  VARCHAR(100),
        response      BYTEA,
        response_type VARCHAR(100),

        error_code    VARCHAR(50),
        error_msg     TEXT,
        error_trace   TEXT,

        worker_id     VARCHAR(100),
        created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
    )`,

		// Индексы task_executions
		`CREATE INDEX IF NOT EXISTS idx_executions_task_id
        ON task_executions(task_id)`,

		`CREATE INDEX IF NOT EXISTS idx_executions_status
        ON task_executions(status)`,

		`CREATE INDEX IF NOT EXISTS idx_executions_started
        ON task_executions(started_at DESC)`,

		`CREATE INDEX IF NOT EXISTS idx_executions_task_status
        ON task_executions(task_id, status)`,

		// Таблица task_locks
		`CREATE TABLE IF NOT EXISTS task_locks (
        id          SERIAL      PRIMARY KEY,
        task_id     UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
        worker_id   VARCHAR(100) NOT NULL,
        acquired_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        expires_at  TIMESTAMPTZ NOT NULL,

        CONSTRAINT uq_task_lock UNIQUE (task_id)
    )`,

		// Индекс task_locks
		`CREATE INDEX IF NOT EXISTS idx_locks_expires
        ON task_locks(expires_at) WHERE expires_at < NOW()`,
	}

	for _, query := range queries {
		if _, err := pool.Exec(ctx, query); err != nil {
			pool.Close()
			return nil, fmt.Errorf("%s: init schema: %w", op, err)
		}
	}

	log.Info("Database schema initialized successfully")

	return &Storage{pool: pool}, nil
}
