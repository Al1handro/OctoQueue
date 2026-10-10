package domain_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"OctoQueue/internal/domain"
)

func TestErrorHierarchy(t *testing.T) {
	tests := []struct {
		child, parent *domain.Error
	}{
		{domain.ErrUserNotFound, domain.ErrNotFound},
		{domain.ErrTaskNotFound, domain.ErrNotFound},
		{domain.ErrExecutionNotFound, domain.ErrNotFound},
		{domain.ErrLockNotFound, domain.ErrNotFound},
		{domain.ErrDuplicateEmail, domain.ErrConflict},
		{domain.ErrDuplicateTask, domain.ErrConflict},
		{domain.ErrLockHeld, domain.ErrConflict},
		{domain.ErrInvalidID, domain.ErrInvalidRequest},
		{domain.ErrInvalidTaskName, domain.ErrInvalidRequest},
		{domain.ErrInvalidTaskType, domain.ErrInvalidRequest},
		{domain.ErrInvalidTaskStatus, domain.ErrInvalidRequest},
		{domain.ErrInvalidSchedule, domain.ErrInvalidRequest},
		{domain.ErrRequiredField, domain.ErrInvalidRequest},
		{domain.ErrValueTooLong, domain.ErrInvalidRequest},
		{domain.ErrInvalidValue, domain.ErrInvalidRequest},
		{domain.ErrInvalidReference, domain.ErrInvalidRequest},
		{domain.ErrConstraintViolation, domain.ErrInvalidRequest},
		{domain.ErrDBUnavailable, domain.ErrUnavailable},
		{domain.ErrRetryable, domain.ErrUnavailable},
		{domain.ErrTimeout, domain.ErrUnavailable},
		{domain.ErrDatabase, domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.child.Code, func(t *testing.T) {
			if !errors.Is(tt.child, tt.child) {
				t.Error("error must match itself")
			}
			if !errors.Is(tt.child, tt.parent) {
				t.Errorf("%s must be a %s", tt.child.Code, tt.parent.Code)
			}
			if errors.Is(tt.parent, tt.child) {
				t.Errorf("%s must NOT match its child %s", tt.parent.Code, tt.child.Code)
			}
		})
	}
}

func TestErrorSiblingsDoNotMatch(t *testing.T) {
	if errors.Is(domain.ErrUserNotFound, domain.ErrTaskNotFound) {
		t.Error("ErrUserNotFound must not match ErrTaskNotFound")
	}
	if errors.Is(domain.ErrInvalidTaskType, domain.ErrNotFound) {
		t.Error("validation error must not be a not-found error")
	}
}

func TestErrorUnwrapOfRootIsNil(t *testing.T) {
	// Защита от typed-nil: Unwrap корня обязан вернуть именно nil-интерфейс,
	// иначе errors.Is/As пойдут по цепочке с «nil внутри».
	if got := errors.Unwrap(domain.ErrNotFound); got != nil {
		t.Errorf("Unwrap(root) = %#v, want nil", got)
	}
	if got := errors.Unwrap(domain.ErrTaskNotFound); got != domain.ErrNotFound {
		t.Errorf("Unwrap(ErrTaskNotFound) = %v, want ErrNotFound", got)
	}
}

func TestErrorThroughFmtWrapping(t *testing.T) {
	err := fmt.Errorf("handler: %w", fmt.Errorf("repo: %w", domain.ErrInvalidTaskType))

	if !errors.Is(err, domain.ErrInvalidTaskType) || !errors.Is(err, domain.ErrInvalidRequest) {
		t.Fatal("wrapped error must match both the leaf and its category")
	}

	// errors.As возвращает самый конкретный узел — из него берутся Code/Message для API.
	var de *domain.Error
	if !errors.As(err, &de) {
		t.Fatal("errors.As(*domain.Error) failed")
	}
	if de != domain.ErrInvalidTaskType || de.Code != "INVALID_TASK_TYPE" {
		t.Errorf("As returned %q, want the most specific node INVALID_TASK_TYPE", de.Code)
	}
}

func TestDBError(t *testing.T) {
	cause := errors.New("ERROR: duplicate key (SQLSTATE 23505)")
	dbErr := &domain.DBError{
		Kind:       domain.ErrDuplicateEmail,
		SQLState:   "23505",
		Constraint: "users_email_key",
		Cause:      cause,
	}
	err := fmt.Errorf("repo: %w", dbErr)

	t.Run("matches kind, its category and the driver error", func(t *testing.T) {
		for name, target := range map[string]error{
			"kind":     domain.ErrDuplicateEmail,
			"category": domain.ErrConflict,
			"cause":    cause,
		} {
			if !errors.Is(err, target) {
				t.Errorf("errors.Is(err, %s) = false", name)
			}
		}
		if errors.Is(err, domain.ErrNotFound) {
			t.Error("must not match an unrelated category")
		}
	})

	t.Run("errors.As extracts DBError fields", func(t *testing.T) {
		var got *domain.DBError
		if !errors.As(err, &got) {
			t.Fatal("errors.As(*DBError) failed")
		}
		if got.SQLState != "23505" || got.Constraint != "users_email_key" {
			t.Errorf("unexpected fields: %+v", got)
		}
	})

	t.Run("Error() keeps kind and driver text", func(t *testing.T) {
		msg := dbErr.Error()
		if !strings.Contains(msg, "email already taken") || !strings.Contains(msg, "SQLSTATE 23505") {
			t.Errorf("Error() = %q", msg)
		}
	})

	t.Run("without cause the SQLSTATE is still reported", func(t *testing.T) {
		e := &domain.DBError{Kind: domain.ErrTimeout, SQLState: "57014"}
		if !strings.Contains(e.Error(), "57014") {
			t.Errorf("Error() = %q", e.Error())
		}
		if got := e.Unwrap(); len(got) != 1 {
			t.Errorf("Unwrap() len = %d, want 1", len(got))
		}
	})

	t.Run("nil kind falls back to database error", func(t *testing.T) {
		e := &domain.DBError{SQLState: "XX000"}
		if !strings.Contains(e.Error(), domain.ErrDatabase.Message) {
			t.Errorf("Error() = %q", e.Error())
		}
		if got := e.Unwrap(); len(got) != 0 {
			t.Errorf("Unwrap() len = %d, want 0", len(got))
		}
	})
}

// Коды уходят клиентам API — дубликаты сделали бы ответы неоднозначными.
func TestErrorCodesAreUnique(t *testing.T) {
	all := []*domain.Error{
		domain.ErrNotFound, domain.ErrConflict, domain.ErrInvalidRequest, domain.ErrUnauthorized,
		domain.ErrForbidden, domain.ErrUnavailable, domain.ErrInternal,
		domain.ErrUserNotFound, domain.ErrTaskNotFound, domain.ErrExecutionNotFound, domain.ErrLockNotFound,
		domain.ErrDuplicateEmail, domain.ErrDuplicateTask, domain.ErrLockHeld,
		domain.ErrInvalidID, domain.ErrInvalidTaskName, domain.ErrInvalidTaskType, domain.ErrInvalidTaskStatus,
		domain.ErrInvalidSchedule, domain.ErrRequiredField, domain.ErrValueTooLong, domain.ErrInvalidValue,
		domain.ErrInvalidReference, domain.ErrConstraintViolation,
		domain.ErrDBUnavailable, domain.ErrRetryable, domain.ErrTimeout, domain.ErrDatabase,
	}
	seen := make(map[string]bool, len(all))
	for _, e := range all {
		if e.Code == "" || e.Message == "" {
			t.Errorf("error %+v has empty code or message", e)
		}
		if seen[e.Code] {
			t.Errorf("duplicate error code %q", e.Code)
		}
		seen[e.Code] = true
	}
}
