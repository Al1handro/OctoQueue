BEGIN;

TRUNCATE public.tasks CASCADE;

INSERT INTO public.tasks (
    "name", "type", payload, schedule, timezone, status,
    next_run_at, last_run_at, started_at,
    retries, max_retries, retry_delay, timeout,
    target_host, tags, created_by, created_at
) VALUES
-- 1. HTTP: ожидает запуска, ежедневно в 09:00
(
    'Daily Health Check', 'http_call',
    jsonb_build_object(
        'url', 'https://api.example.com/health',
        'method', 'GET',
        'headers', jsonb_build_object('Authorization', 'Bearer dev_token')
    ),
    '0 9 * * *', 'Europe/Moscow', 'pending',
    NOW() + INTERVAL '1 hour', NULL, NULL,
    0, 3, INTERVAL '00:01:00', INTERVAL '00:00:30',
    'api.example.com', '{monitoring,health}'::text[], 'admin',
    NOW()
),
-- 2. Shell: выполняется прямо сейчас (без расписания)
(
    'Backup Database', 'shell',
    jsonb_build_object(
        'command', 'pg_dump -U postgres octoqueue > /backups/db.sql',
        'cwd', '/opt/scripts',
        'env', jsonb_build_object('PGPASSWORD', 'secret')
    ),
    NULL, 'UTC', 'running',
    NULL, NOW() - INTERVAL '5 minutes', NOW() - INTERVAL '5 minutes',
    0, 1, INTERVAL '00:05:00', INTERVAL '00:30:00',
    'db.internal', '{backup,maintenance}'::text[], 'cron_daemon',
    NOW()
),
-- 3. gRPC: успешно выполнена, повторяется каждый час
(
    'Sync User Data', 'grpc',
    jsonb_build_object(
        'service', 'UserService',
        'method', 'SyncBatch',
        'timeout_ms', 5000,
        'metadata', jsonb_build_object('env', 'staging')
    ),
    '30 * * * *', 'UTC', 'completed',
    NOW() + INTERVAL '50 minutes', NOW() - INTERVAL '2 minutes', NOW() - INTERVAL '3 minutes',
    0, 3, INTERVAL '00:01:00', INTERVAL '00:00:30',
    'grpc.internal:50051', '{sync,users}'::text[], 'system',
    NOW()
),
-- 4. HTTP: упала после exhaustion retry
(
    'Webhook Notification', 'http_call',
    jsonb_build_object(
        'url', 'https://hooks.slack.com/services/T00/B00/XXX',
        'method', 'POST',
        'body', jsonb_build_object('text', 'Task failed alert!')
    ),
    '0 0 * * 1', 'America/New_York', 'failed',
    NULL, NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day' - INTERVAL '1 minute',
    3, 3, INTERVAL '00:05:00', INTERVAL '00:00:30',
    'hooks.slack.com', '{notifications,critical}'::text[], 'alert_service',
    NOW() - INTERVAL '2 days'
),
-- 5. Shell: отменена и помечена soft-delete
(
    'Legacy Cleanup', 'shell',
    jsonb_build_object('command', 'rm -rf /tmp/old_logs/*'),
    NULL, 'UTC', 'cancelled',
    NULL, NULL, NULL,
    0, 0, INTERVAL '00:00:00', INTERVAL '00:00:30',
    'worker-01', '{cleanup,deprecated}'::text[], 'admin',
    NOW() - INTERVAL '3 days'
);

COMMIT;