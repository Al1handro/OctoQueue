package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"OctoQueue/internal/domain"
)

func TestStatusFor(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{domain.ErrInvalidTaskType, http.StatusBadRequest},
		{domain.ErrInvalidID, http.StatusBadRequest},
		{domain.ErrRequiredField, http.StatusBadRequest},
		{domain.ErrUnauthorized, http.StatusUnauthorized},
		{domain.ErrForbidden, http.StatusForbidden},
		{domain.ErrTaskNotFound, http.StatusNotFound},
		{domain.ErrUserNotFound, http.StatusNotFound},
		{domain.ErrDuplicateEmail, http.StatusConflict},
		{domain.ErrLockHeld, http.StatusConflict},
		{domain.ErrTimeout, http.StatusGatewayTimeout}, // вложена в ErrUnavailable, но должна выигрывать
		{domain.ErrDBUnavailable, http.StatusServiceUnavailable},
		{domain.ErrRetryable, http.StatusServiceUnavailable},
		{domain.ErrDatabase, http.StatusInternalServerError},
		{errors.New("something unexpected"), http.StatusInternalServerError},
		{fmt.Errorf("op: %w", fmt.Errorf("repo: %w", domain.ErrInvalidSchedule)), http.StatusBadRequest},
	}
	for _, tt := range tests {
		if got := statusFor(tt.err); got != tt.want {
			t.Errorf("statusFor(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}

func decodeErrBody(t *testing.T, rec *httptest.ResponseRecorder) errDetail {
	t.Helper()
	var body errBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v\n%s", err, rec.Body.String())
	}
	return body.Error
}

func TestWriteServiceError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{
			name:       "invalid task type from a check constraint",
			err:        fmt.Errorf("repo: %w", &domain.DBError{Kind: domain.ErrInvalidTaskType, SQLState: "23514", Constraint: "tasks_type_check"}),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_TASK_TYPE",
		},
		{
			name:       "required field reports which field",
			err:        &domain.DBError{Kind: domain.ErrRequiredField, SQLState: "23502", Column: "payload"},
			wantStatus: http.StatusBadRequest,
			wantCode:   "REQUIRED_FIELD",
			wantField:  "payload",
		},
		{
			name:       "duplicate email",
			err:        &domain.DBError{Kind: domain.ErrDuplicateEmail, SQLState: "23505", Constraint: "users_email_key"},
			wantStatus: http.StatusConflict,
			wantCode:   "EMAIL_TAKEN",
		},
		{
			name:       "plain domain error",
			err:        fmt.Errorf("repo: %w", domain.ErrInvalidID),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ID",
		},
		{
			name:       "database down",
			err:        &domain.DBError{Kind: domain.ErrDBUnavailable, SQLState: "08006"},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "DB_UNAVAILABLE",
		},
		{
			name:       "timeout",
			err:        &domain.DBError{Kind: domain.ErrTimeout, SQLState: "57014"},
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "TIMEOUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeServiceError(rec, logger, tt.err, "failed to do thing")

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q", ct)
			}
			got := decodeErrBody(t, rec)
			if got.Code != tt.wantCode || got.Field != tt.wantField {
				t.Errorf("got code=%q field=%q, want code=%q field=%q", got.Code, got.Field, tt.wantCode, tt.wantField)
			}
		})
	}
}

// Внутренности (текст ошибки Postgres, SQLSTATE, имена ограничений) не должны утекать клиенту.
func TestWriteServiceError_DoesNotLeakInternals(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	secrets := []string{"42703", "SQLSTATE", "user_id", "task_executions", "password=hunter2"}

	errs := map[string]error{
		"unclassified error": errors.New(`dial tcp: password=hunter2 refused`),
		"classified as internal": &domain.DBError{
			Kind:       domain.ErrDatabase,
			SQLState:   "42703",
			Constraint: "task_executions",
			Column:     "user_id",
			Cause:      errors.New(`ERROR: column "user_id" does not exist (SQLSTATE 42703)`),
		},
	}
	for name, err := range errs {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeServiceError(rec, logger, err, "failed to get executions")

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", rec.Code)
			}
			got := decodeErrBody(t, rec)
			if got.Code != "INTERNAL_ERROR" || got.Message != "failed to get executions" || got.Field != "" {
				t.Errorf("unexpected body: %+v", got)
			}
			for _, s := range secrets {
				if strings.Contains(rec.Body.String(), s) {
					t.Errorf("response leaks %q: %s", s, rec.Body.String())
				}
			}
		})
	}
}
