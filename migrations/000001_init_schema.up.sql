CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS tasks (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name           VARCHAR(255) NOT NULL,
    type           VARCHAR(50)  NOT NULL CHECK (type IN ('http_call', 'shell', 'grpc')),
    payload        JSONB        NOT NULL DEFAULT '{}',
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
);

CREATE INDEX IF NOT EXISTS idx_tasks_status
ON tasks(status) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tasks_next_run
ON tasks(next_run_at) WHERE status = 'pending' AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tasks_type
ON tasks(type) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tasks_tags
ON tasks USING GIN(tags) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tasks_created_at
ON tasks(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_tasks_target_host
ON tasks(target_host) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tasks_payload
ON tasks USING GIN(payload) WHERE deleted_at IS NULL;

DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'trg_tasks_updated_at'
    ) THEN
        CREATE TRIGGER trg_tasks_updated_at
            BEFORE UPDATE ON tasks
            FOR EACH ROW
            EXECUTE FUNCTION update_timestamp();
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS task_executions (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id       UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    status        VARCHAR(20) NOT NULL DEFAULT 'running'
                          CHECK (status IN ('running', 'success', 'failed', 'timeout', 'cancelled')),
    attempt       INT         NOT NULL DEFAULT 1,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at   TIMESTAMPTZ,
    duration      INTERVAL,
    request       JSONB,
    response      JSONB,
    error_code    VARCHAR(50),
    error_msg     TEXT,
    error_trace   TEXT,
    worker_id     VARCHAR(100),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_executions_task_id
ON task_executions(task_id);

CREATE INDEX IF NOT EXISTS idx_executions_status
ON task_executions(status);

CREATE INDEX IF NOT EXISTS idx_executions_started
ON task_executions(started_at DESC);

CREATE INDEX IF NOT EXISTS idx_executions_task_status
ON task_executions(task_id, status);

CREATE TABLE IF NOT EXISTS task_locks (
    id          SERIAL       PRIMARY KEY,
    task_id     UUID         NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    worker_id   VARCHAR(100) NOT NULL,
    acquired_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ  NOT NULL,
    CONSTRAINT uq_task_lock UNIQUE (task_id)
);

CREATE INDEX IF NOT EXISTS idx_locks_expires
ON task_locks(expires_at);