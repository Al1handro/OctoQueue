package repository

import (
	"OctoQueue/internal/domain"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrInvalidPayload      = errors.New("invalid payload")
	ErrInvalidUTF8         = errors.New("invalid UTF-8 encoding")
	ErrContainsNullBytes   = errors.New("payload contains null bytes")
	ErrDuplicateTask       = errors.New("task already exists")
	ErrForeignKeyViolation = errors.New("foreign key violation")
	ErrNotNullViolation    = errors.New("not null violation")
)

type ValidationError struct {
	Field   string
	Message string
	Err     error
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Message)
	}
	return e.Message
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}

// Быстрая валидация критических полей
func validateCriticalFields(p domain.CreateTaskParams) error {
	// Проверка обязательных полей
	if strings.TrimSpace(p.Name) == "" {
		return &ValidationError{
			Field:   "name",
			Message: "cannot be empty",
			Err:     ErrNotNullViolation,
		}
	}

	if strings.TrimSpace(p.Type) == "" {
		return &ValidationError{
			Field:   "type",
			Message: "cannot be empty",
			Err:     ErrNotNullViolation,
		}
	}

	if p.UserID == uuid.Nil {
		return &ValidationError{
			Field:   "user_id",
			Message: "cannot be empty",
			Err:     ErrForeignKeyViolation,
		}
	}

	return nil
}

func validateAndNormalizePayload(p *domain.CreateTaskParams) error {
	if p.Payload == nil {
		return &ValidationError{
			Field:   "payload",
			Message: "cannot be nil",
			Err:     ErrInvalidPayload,
		}
	}

	if bytes.Contains(p.Payload, []byte{0}) {
		return &ValidationError{
			Field:   "payload",
			Message: "contains null bytes (0x00)",
			Err:     ErrContainsNullBytes,
		}
	}

	if !utf8.Valid(p.Payload) {
		return &ValidationError{
			Field:   "payload",
			Message: "invalid UTF-8 encoding",
			Err:     ErrInvalidUTF8,
		}
	}

	if !json.Valid(p.Payload) {
		return &ValidationError{
			Field:   "payload",
			Message: "invalid JSON format",
			Err:     ErrInvalidPayload,
		}
	}

	var normalized bytes.Buffer
	if err := json.Compact(&normalized, p.Payload); err == nil {
		p.Payload = normalized.Bytes()
	}

	return nil
}

// Проверка бизнес-правил (требует доступа к БД)
func validateBusinessRules(ctx context.Context, s *Storage, p domain.CreateTaskParams) error {
	// Проверка существования пользователя
	var exists bool
	err := s.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", p.UserID,
	).Scan(&exists)

	if err != nil {
		return fmt.Errorf("check user existence: %w", err)
	}

	if !exists {
		return &ValidationError{
			Field:   "user_id",
			Message: "user does not exist",
			Err:     ErrForeignKeyViolation,
		}
	}

	// Другие бизнес-проверки...

	return nil
}

func mapDBError(err error) error {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "22021": // invalid byte sequence
			return fmt.Errorf("%w: invalid byte sequence (SQLSTATE %s)",
				ErrInvalidUTF8, pgErr.Code)

		case "23505": // unique violation
			return fmt.Errorf("%w: %s", ErrDuplicateTask, pgErr.Detail)

		case "23503": // foreign key violation
			return fmt.Errorf("%w: %s", ErrForeignKeyViolation, pgErr.Detail)

		default:
			return fmt.Errorf("database error (SQLSTATE %s): %s",
				pgErr.Code, pgErr.Message)
		}
	}

	return err
}

func (s *Storage) CreateTask(ctx context.Context, p domain.CreateTaskParams) (*domain.Task, error) {
	const op = "storage.repository.CreateTask"

    if err := validateCriticalFields(p); err != nil {
        return nil, fmt.Errorf("%s: %w", op, err)
    }
    
    if err := validateAndNormalizePayload(&p); err != nil {
        return nil, fmt.Errorf("%s: %w", op, err)
    }
    
    if err := validateBusinessRules(ctx, s, p); err != nil {
        return nil, fmt.Errorf("%s: %w", op, err)
    }

	row := s.pool.QueryRow(ctx, `
		INSERT INTO tasks (
			name, type, payload, schedule, timezone,
			next_run_at, max_retries, tags, created_by, 
			target_host, user_id
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11
		)
		RETURNING
			id, name, type, payload, schedule, timezone,
			status, next_run_at, last_run_at, started_at,
			retries, max_retries, retry_delay, timeout,
			target_host, tags, created_by, created_at, 
			updated_at, deleted_at, user_id`,
		p.Name, p.Type, p.Payload, p.Schedule, p.Timezone,
		p.NextRunAt, p.MaxRetries, p.Tags, p.CreatedBy, p.TargetHost, p.UserID,
	)

	task, err := scanTask(row, op)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, mapDBError(err))
	}

	return task, nil
}

func (s *Storage) GetTaskByID(ctx context.Context, id string) (*domain.Task, error) {
	const op = "storage.repository.GetTaskByID"

	row := s.pool.QueryRow(ctx, `
		SELECT
			id, name, type, payload, schedule, timezone,
			status, next_run_at, last_run_at, started_at,
			retries, max_retries, retry_delay, timeout,
			target_host, tags, created_by, created_at, updated_at, deleted_at, user_id
		FROM tasks
		WHERE id = $1 AND deleted_at IS NULL`,
		id,
	)

	t, err := scanTask(row, op)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return t, nil
}

func (s *Storage) ListTasks(ctx context.Context, p domain.ListTasksParams) ([]*domain.Task, error) {
	const op = "storage.repository.ListTasks"

	if p.Limit <= 0 {
		p.Limit = 50
	}

	if p.Limit > 100 {
		p.Limit = 100
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			id, name, type, payload, schedule, timezone,
			status, next_run_at, last_run_at, started_at,
			retries, max_retries, retry_delay, timeout,
			target_host, tags, created_by, created_at, 
			updated_at, deleted_at, user_id
		FROM tasks
		WHERE deleted_at IS NULL
		  AND ($1::varchar IS NULL OR status = $1)
		  AND ($2::varchar IS NULL OR type   = $2)
		  AND ($3::text[]  IS NULL OR tags   @> $3)
		  AND ($6::uuid IS NULL OR user_id = $6)
		ORDER BY created_at DESC, id DESC
		LIMIT $4 OFFSET $5`,
		p.Status, p.Type, p.Tags, p.Limit, p.Offset, p.UserID,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		t, err := scanTask(rows, op)
		if err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

func (s *Storage) UpdateTask(ctx context.Context, p domain.UpdateTaskParams) (*domain.Task, error) {
	const op = "storage.repository.UpdateTask"

	row := s.pool.QueryRow(ctx, `
		UPDATE tasks SET
			name        = COALESCE($2, name),
			payload     = COALESCE($3, payload),
			schedule    = COALESCE($4, schedule),
			timezone    = COALESCE($5, timezone),
			max_retries = COALESCE($6, max_retries),
			tags        = COALESCE($7, tags),
			target_host = COALESCE($8, target_host)
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING
			id, name, type, payload, schedule, timezone,
			status, next_run_at, last_run_at, started_at,
			retries, max_retries, retry_delay, timeout,
			target_host, tags, created_by, created_at, updated_at, deleted_at, user_id`,
		p.ID, p.Name, p.Payload, p.Schedule, p.Timezone,
		p.MaxRetries, p.Tags, p.TargetHost,
	)

	tag, err := scanTask(row, op)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return tag, nil
}

func (s *Storage) UpdateTaskStatus(ctx context.Context, id, status string, nextRunAt *time.Time) error {
	const op = "storage.repository.UpdateTaskStatus"

	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks SET
			status = $2,
			next_run_at = $3,
			last_run_at = CASE
				WHEN CAST($2 AS varchar(20)) IN ('completed', 'failed')
				THEN NOW()
				ELSE last_run_at
			END
		WHERE id = $1
		AND deleted_at IS NULL`,
		id, status, nextRunAt,
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, domain.ErrNotFound)
	}

	return nil
}

func (s *Storage) DeleteTask(ctx context.Context, id string) error {
	const op = "storage.repository.DeleteTask"

	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks SET deleted_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`,
		id,
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, domain.ErrNotFound)
	}

	return nil
}

// Берёт до limit задач готовых к запуску, атомарно переводит в running

func (s *Storage) AcquirePendingTasks(ctx context.Context, workerID string, limit int) ([]*domain.Task, error) {
	const op = "storage.repository.AcquirePendingTasks"

	rows, err := s.pool.Query(ctx, `
		WITH next_tasks AS (
			SELECT id FROM tasks
			WHERE status     = 'pending'
			  AND next_run_at <= NOW()
			  AND deleted_at IS NULL
			ORDER BY next_run_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE tasks SET
			status     = 'running',
			started_at = NOW(),
			retries    = retries + 1
		FROM next_tasks
		WHERE tasks.id = next_tasks.id
		RETURNING
			tasks.id, tasks.name, tasks.type, tasks.payload, tasks.schedule, tasks.timezone,
			tasks.status, tasks.next_run_at, tasks.last_run_at, tasks.started_at,
			tasks.retries, tasks.max_retries, tasks.retry_delay, tasks.timeout,
			tasks.target_host, tasks.tags, tasks.created_by,
			tasks.created_at, tasks.updated_at, tasks.deleted_at`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		t, err := scanTask(rows, op)
		if err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

func (s *Storage) CreateExecution(ctx context.Context, p domain.CreateExecutionParams) (*domain.TaskExecution, error) {
	const op = "storage.repository.CreateExecution"

	row := s.pool.QueryRow(ctx, `
		INSERT INTO task_executions (task_id, attempt, worker_id, request)
		VALUES ($1, $2, $3, $4)
		RETURNING
			id, task_id, status, attempt,
			started_at, finished_at, duration,
			request, response,
			error_code, error_msg, error_trace,
			worker_id, created_at`,
		p.TaskID, p.Attempt, p.WorkerID, p.Request,
	)

	e, err := scanExecution(row)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return e, nil
}

func (s *Storage) FinishExecution(ctx context.Context, p domain.FinishExecutionParams) (*domain.TaskExecution, error) {
	const op = "storage.repository.FinishExecution"

	row := s.pool.QueryRow(ctx, `
		UPDATE task_executions SET
			status      = $2,
			finished_at = NOW(),
			duration    = NOW() - started_at,
			response    = $3,
			error_code  = $4,
			error_msg   = $5,
			error_trace = $6
		WHERE id = $1
		RETURNING
			id, task_id, status, attempt,
			started_at, finished_at, duration,
			request, response,
			error_code, error_msg, error_trace,
			worker_id, created_at`,
		p.ID, p.Status, p.Response, p.ErrorCode, p.ErrorMsg, p.ErrorTrace,
	)

	e, err := scanExecution(row)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return e, nil
}

func (s *Storage) ListExecutions(ctx context.Context, taskID string, userID uuid.UUID, limit int) ([]*domain.TaskExecution, error) {
	const op = "storage.repository.ListExecutions"

	if limit == 0 {
		limit = 20
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			id, task_id, status, attempt,
			started_at, finished_at, duration,
			request, response,
			error_code, error_msg, error_trace,
			worker_id, created_at
		FROM task_executions
		WHERE task_id = $1 and user_id = $2
		ORDER BY started_at DESC
		LIMIT $3`,
		taskID, userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var execs []*domain.TaskExecution
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		execs = append(execs, e)
	}

	return execs, rows.Err()
}

func (s *Storage) AcquireLock(ctx context.Context, taskID, workerID string, ttl time.Duration) (*domain.TaskLock, error) {
	const op = "storage.repository.AcquireLock"

	row := s.pool.QueryRow(ctx, `
		INSERT INTO task_locks (task_id, worker_id, expires_at)
		VALUES ($1, $2, NOW() + $3::interval)
		ON CONFLICT (task_id) DO UPDATE
			SET worker_id   = EXCLUDED.worker_id,
			    acquired_at = NOW(),
			    expires_at  = EXCLUDED.expires_at
			WHERE task_locks.expires_at < NOW()
		RETURNING id, task_id, worker_id, acquired_at, expires_at`,
		taskID, workerID, ttl.String(),
	)

	var l domain.TaskLock
	err := row.Scan(&l.ID, &l.TaskID, &l.WorkerID, &l.AcquiredAt, &l.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &l, nil
}

func (s *Storage) ReleaseLock(ctx context.Context, taskID, workerID string) error {
	const op = "storage.repository.ReleaseLock"

	tag, err := s.pool.Exec(ctx, `
		DELETE FROM task_locks
		WHERE task_id = $1 AND worker_id = $2`,
		taskID, workerID,
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: lock not found for task %s", op, taskID)
	}

	return nil
}

func (s *Storage) CleanExpiredLocks(ctx context.Context) (int64, error) {
	const op = "storage.repository.CleanExpiredLocks"

	tag, err := s.pool.Exec(ctx, `DELETE FROM task_locks WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return tag.RowsAffected(), nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(s scanner, op string) (*domain.Task, error) {
	var t domain.Task
	err := s.Scan(
		&t.ID, &t.Name, &t.Type, &t.Payload, &t.Schedule, &t.Timezone,
		&t.Status, &t.NextRunAt, &t.LastRunAt, &t.StartedAt,
		&t.Retries, &t.MaxRetries, &t.RetryDelay, &t.Timeout,
		&t.TargetHost, &t.Tags, &t.CreatedBy,
		&t.CreatedAt, &t.UpdatedAt, &t.DeletedAt, &t.UserID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s undefinde error with db: %w", op, domain.ErrTaskNotFound)
		}
		return nil, err
	}
	return &t, nil
}

func scanExecution(s scanner) (*domain.TaskExecution, error) {
	var e domain.TaskExecution
	err := s.Scan(
		&e.ID, &e.TaskID, &e.Status, &e.Attempt,
		&e.StartedAt, &e.FinishedAt, &e.Duration,
		&e.Request, &e.Response,
		&e.ErrorCode, &e.ErrorMsg, &e.ErrorTrace,
		&e.WorkerID, &e.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &e, nil
}
