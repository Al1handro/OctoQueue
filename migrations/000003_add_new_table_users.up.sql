BEGIN;

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE IF NOT EXISTS tasks
ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE SET NULL;

COMMIT;