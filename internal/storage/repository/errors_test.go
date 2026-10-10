package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"OctoQueue/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

func pgError(code, constraint, column string) error {
	return &pgconn.PgError{
		Severity:       "ERROR",
		Code:           code,
		Message:        "boom",
		ConstraintName: constraint,
		ColumnName:     column,
	}
}

// Реактивный слой: SQLSTATE (+ имя ограничения) → доменная ошибка.
func TestMapError_SQLState(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		kind       *domain.Error // самый конкретный смысл
		category   *domain.Error // корневая категория → HTTP-статус
		wantColumn string
	}{
		{"duplicate email", pgError("23505", "users_email_key", ""), domain.ErrDuplicateEmail, domain.ErrConflict, ""},
		{"lock already held", pgError("23505", "uq_task_lock", ""), domain.ErrLockHeld, domain.ErrConflict, ""},
		{"unknown unique", pgError("23505", "some_other_key", ""), domain.ErrConflict, domain.ErrConflict, ""},
		{"exclusion", pgError("23P01", "", ""), domain.ErrConflict, domain.ErrConflict, ""},

		{"fk: no such user", pgError("23503", "tasks_user_id_fkey", ""), domain.ErrUserNotFound, domain.ErrNotFound, ""},
		{"fk: no such task (execution)", pgError("23503", "task_executions_task_id_fkey", ""), domain.ErrTaskNotFound, domain.ErrNotFound, ""},
		{"fk: unknown", pgError("23503", "x_fkey", ""), domain.ErrInvalidReference, domain.ErrInvalidRequest, ""},

		{"not null", pgError("23502", "", "payload"), domain.ErrRequiredField, domain.ErrInvalidRequest, "payload"},
		{"check: type", pgError("23514", "tasks_type_check", ""), domain.ErrInvalidTaskType, domain.ErrInvalidRequest, ""},
		{"check: status", pgError("23514", "tasks_status_check", ""), domain.ErrInvalidTaskStatus, domain.ErrInvalidRequest, ""},
		{"check: schedule", pgError("23514", "chk_schedule", ""), domain.ErrInvalidSchedule, domain.ErrInvalidRequest, ""},
		{"check: unknown", pgError("23514", "users_role_check", ""), domain.ErrConstraintViolation, domain.ErrInvalidRequest, ""},
		{"class 23 fallback", pgError("23000", "", ""), domain.ErrConstraintViolation, domain.ErrInvalidRequest, ""},

		{"value too long", pgError("22001", "", ""), domain.ErrValueTooLong, domain.ErrInvalidRequest, ""},
		{"invalid text representation (uuid/json)", pgError("22P02", "", ""), domain.ErrInvalidValue, domain.ErrInvalidRequest, ""},
		{"NUL byte", pgError("22021", "", ""), domain.ErrInvalidValue, domain.ErrInvalidRequest, ""},
		{"numeric out of range", pgError("22003", "", ""), domain.ErrInvalidValue, domain.ErrInvalidRequest, ""},
		{"class 22 fallback", pgError("22012", "", ""), domain.ErrInvalidValue, domain.ErrInvalidRequest, ""},

		{"serialization failure", pgError("40001", "", ""), domain.ErrRetryable, domain.ErrUnavailable, ""},
		{"deadlock", pgError("40P01", "", ""), domain.ErrRetryable, domain.ErrUnavailable, ""},
		{"lock not available", pgError("55P03", "", ""), domain.ErrRetryable, domain.ErrUnavailable, ""},
		{"query canceled", pgError("57014", "", ""), domain.ErrTimeout, domain.ErrUnavailable, ""},
		{"admin shutdown", pgError("57P01", "", ""), domain.ErrDBUnavailable, domain.ErrUnavailable, ""},
		{"connection failure", pgError("08006", "", ""), domain.ErrDBUnavailable, domain.ErrUnavailable, ""},
		{"too many connections", pgError("53300", "", ""), domain.ErrDBUnavailable, domain.ErrUnavailable, ""},

		{"undefined column (schema drift)", pgError("42703", "", ""), domain.ErrDatabase, domain.ErrInternal, ""},
		{"unknown sqlstate", pgError("XX000", "", ""), domain.ErrDatabase, domain.ErrInternal, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapError(fmt.Errorf("driver: %w", tt.err))

			var dbErr *domain.DBError
			if !errors.As(got, &dbErr) {
				t.Fatalf("expected *domain.DBError, got %T: %v", got, got)
			}
			if dbErr.Kind != tt.kind {
				t.Errorf("kind = %q, want %q", dbErr.Kind.Code, tt.kind.Code)
			}
			if !errors.Is(got, tt.kind) || !errors.Is(got, tt.category) {
				t.Errorf("must match kind %s and category %s", tt.kind.Code, tt.category.Code)
			}
			if dbErr.Column != tt.wantColumn {
				t.Errorf("column = %q, want %q", dbErr.Column, tt.wantColumn)
			}

			// Исходная ошибка Postgres остаётся доступной для диагностики.
			var pgErr *pgconn.PgError
			if !errors.As(got, &pgErr) {
				t.Fatal("original *pgconn.PgError must stay reachable via Unwrap")
			}
			if dbErr.SQLState != pgErr.Code {
				t.Errorf("SQLState = %q, want %q", dbErr.SQLState, pgErr.Code)
			}
		})
	}
}

func TestMapError_PassThrough(t *testing.T) {
	if mapError(nil) != nil {
		t.Error("nil must stay nil")
	}

	t.Run("already classified is not re-wrapped", func(t *testing.T) {
		if got := mapError(domain.ErrTaskNotFound); got != error(domain.ErrTaskNotFound) {
			t.Errorf("got %v", got)
		}
		first := mapError(pgError("23505", "users_email_key", ""))
		if second := mapError(first); second != first {
			t.Error("mapping must be idempotent")
		}
	})

	t.Run("unknown errors are returned unchanged", func(t *testing.T) {
		plain := errors.New("connection reset by peer")
		if got := mapError(plain); got != plain {
			t.Errorf("got %v", got)
		}
	})

	t.Run("deadline exceeded becomes a timeout but stays detectable", func(t *testing.T) {
		got := mapError(fmt.Errorf("query: %w", context.DeadlineExceeded))
		if !errors.Is(got, domain.ErrTimeout) || !errors.Is(got, domain.ErrUnavailable) {
			t.Error("must match ErrTimeout and ErrUnavailable")
		}
		if !errors.Is(got, context.DeadlineExceeded) {
			t.Error("context.DeadlineExceeded must stay reachable")
		}
	})
}

func TestWrap(t *testing.T) {
	err := wrap("storage.repository.CreateTask", pgError("23514", "tasks_type_check", ""))

	if !strings.HasPrefix(err.Error(), "storage.repository.CreateTask: ") {
		t.Errorf("missing op prefix: %q", err.Error())
	}
	if !errors.Is(err, domain.ErrInvalidTaskType) {
		t.Error("wrap must classify the error")
	}
	if wrap("op", nil) != nil {
		t.Error("wrap(nil) must stay nil")
	}
}

// Проактивный слой.

func TestCheckID(t *testing.T) {
	if err := checkID("123e4567-e89b-12d3-a456-426614174000"); err != nil {
		t.Errorf("valid uuid rejected: %v", err)
	}
	for _, bad := range []string{"", "not-a-uuid", "123", "123e4567-e89b-12d3-a456-42661417400z"} {
		err := checkID(bad)
		if !errors.Is(err, domain.ErrInvalidID) || !errors.Is(err, domain.ErrInvalidRequest) {
			t.Errorf("checkID(%q) = %v, want ErrInvalidID", bad, err)
		}
		if errors.Is(err, domain.ErrNotFound) {
			t.Errorf("checkID(%q): invalid id must not be reported as not found", bad)
		}
	}
}

func TestValidateCreateTask(t *testing.T) {
	ok := domain.CreateTaskParams{Name: "backup", Type: "shell"}

	tests := []struct {
		name string
		mod  func(p *domain.CreateTaskParams)
		want *domain.Error
	}{
		{"valid", func(p *domain.CreateTaskParams) {}, nil},
		{"name: 255 chars is the limit", func(p *domain.CreateTaskParams) { p.Name = strings.Repeat("x", 255) }, nil},
		{"name: limit counts characters, not bytes", func(p *domain.CreateTaskParams) { p.Name = strings.Repeat("я", 255) }, nil},
		{"name: empty", func(p *domain.CreateTaskParams) { p.Name = "" }, domain.ErrInvalidTaskName},
		{"name: blank", func(p *domain.CreateTaskParams) { p.Name = "   \t" }, domain.ErrInvalidTaskName},
		{"name: 256 chars", func(p *domain.CreateTaskParams) { p.Name = strings.Repeat("x", 256) }, domain.ErrInvalidTaskName},
		{"name: NUL byte", func(p *domain.CreateTaskParams) { p.Name = "a\x00b" }, domain.ErrInvalidTaskName},
		{"type: empty", func(p *domain.CreateTaskParams) { p.Type = "" }, domain.ErrInvalidTaskType},
		{"type: unknown", func(p *domain.CreateTaskParams) { p.Type = "carrier_pigeon" }, domain.ErrInvalidTaskType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ok
			tt.mod(&p)
			err := validateCreateTask(p)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.want) || !errors.Is(err, domain.ErrInvalidRequest) {
				t.Fatalf("got %v, want %s", err, tt.want.Code)
			}
		})
	}

	// Проактивная валидация не должна отставать от БД: все типы из домена проходят.
	for typ := range domain.ValidTaskTypes {
		if err := validateCreateTask(domain.CreateTaskParams{Name: "n", Type: typ}); err != nil {
			t.Errorf("type %q from ValidTaskTypes rejected: %v", typ, err)
		}
	}
}
