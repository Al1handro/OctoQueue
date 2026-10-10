-- Тестовые данные для локальной разработки (make seed).
-- Требует применённых миграций (сервис применяет их при старте).
-- Скрипт идемпотентен: можно запускать повторно.
--
-- ТОЛЬКО ДЛЯ DEV — пароли ниже публичны:
--   admin@example.com / admin123   (role = admin, видит задачи всех пользователей)
--   user@example.com  / user1234   (role = user,  видит только свои задачи)
BEGIN;

-- 1. Пользователи. tasks.user_id — NOT NULL + FK на users(id), поэтому
--    пользователи должны существовать до вставки задач.
--    users НЕ очищаем (на них могут ссылаться реальные данные), только upsert по email.
INSERT INTO public.users (email, password_hash, role) VALUES
    ('admin@example.com', '$2a$10$2g4vtedvhST9qnWB8XQwi.VO6nU6xAFtDkOjhfrvpyWyYrlxOI04i', 'admin'),
    ('user@example.com',  '$2a$10$3CcVsuo4zDxdBk0iwmrsb.wDD7Av5wF18XQmoGHsZUKs.35N5tpAa', 'user')
ON CONFLICT (email) DO UPDATE
    SET password_hash = EXCLUDED.password_hash,
        role          = EXCLUDED.role;

-- 2. Задачи. CASCADE заодно очищает task_executions и task_locks.
TRUNCATE public.tasks CASCADE;

-- user_id берём по email, а не по зашитому UUID: так скрипт работает и тогда,
-- когда пользователь с таким email уже существовал с другим id.
-- tags — NOT NULL (DEFAULT '{}'): всегда задаём text[], для «без тегов» — '{}'.
-- type — только из tasks_type_check: http_call, shell, grpc, kafka, email.
INSERT INTO public.tasks (
    "name", "type", payload, schedule, timezone, status,
    next_run_at, last_run_at, started_at,
    retries, max_retries, retry_delay, timeout,
    target_host, tags, created_by, created_at, deleted_at,
    user_id
) VALUES
-- 1. HTTP (user): ожидает запуска, ежедневно в 09:00
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
    NOW(), NULL,
    (SELECT id FROM public.users WHERE email = 'user@example.com')
),
-- 2. Shell (admin): выполняется прямо сейчас (без расписания)
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
    NOW(), NULL,
    (SELECT id FROM public.users WHERE email = 'admin@example.com')
),
-- 3. gRPC (admin): успешно выполнена, повторяется каждый час
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
    NOW(), NULL,
    (SELECT id FROM public.users WHERE email = 'admin@example.com')
),
-- 4. HTTP (user): упала после исчерпания retry
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
    NOW() - INTERVAL '2 days', NULL,
    (SELECT id FROM public.users WHERE email = 'user@example.com')
),
-- 5. Email (user): еженедельный отчёт, по понедельникам в 08:00
(
    'Weekly Report Email', 'email',
    jsonb_build_object(
        'to', jsonb_build_array('team@example.com'),
        'subject', 'Weekly report',
        'body', 'Summary of the last 7 days'
    ),
    '0 8 * * 1', 'Europe/Moscow', 'pending',
    NOW() + INTERVAL '3 days', NULL, NULL,
    0, 3, INTERVAL '00:10:00', INTERVAL '00:01:00',
    'smtp.internal', '{reports,email}'::text[], 'report_service',
    NOW(), NULL,
    (SELECT id FROM public.users WHERE email = 'user@example.com')
),
-- 6. Kafka (admin): одноразовая, без тегов — проверяет NOT NULL DEFAULT '{}' для tags
(
    'Publish Audit Events', 'kafka',
    jsonb_build_object(
        'brokers', jsonb_build_array('kafka.internal:9092'),
        'topic', 'audit-events',
        'key', 'audit',
        'message', jsonb_build_object('event', 'seed_loaded')
    ),
    NULL, 'UTC', 'pending',
    NOW() + INTERVAL '5 minutes', NULL, NULL,
    0, 3, INTERVAL '00:01:00', INTERVAL '00:00:30',
    'kafka.internal:9092', '{}'::text[], 'audit_service',
    NOW(), NULL,
    (SELECT id FROM public.users WHERE email = 'admin@example.com')
),
-- 7. Shell (user): отменена и помечена soft-delete — в API её быть не должно
(
    'Legacy Cleanup', 'shell',
    jsonb_build_object('command', 'rm -rf /tmp/old_logs/*'),
    NULL, 'UTC', 'cancelled',
    NULL, NULL, NULL,
    0, 0, INTERVAL '00:00:00', INTERVAL '00:00:30',
    'worker-01', '{cleanup,deprecated}'::text[], 'admin',
    NOW() - INTERVAL '3 days', NOW() - INTERVAL '1 day',
    (SELECT id FROM public.users WHERE email = 'user@example.com')
);

COMMIT;
