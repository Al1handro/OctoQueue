package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"OctoQueue/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Маппинг ошибок в репозитории устроен в два слоя.
//
//  1. Проактивный — сам код репозитория знает, что значит результат запроса
//     или заведомо плохой вход, и не дожидается ответа Postgres:
//     валидация параметров и UUID до похода в БД (validateCreateTask, checkID),
//     pgx.ErrNoRows и RowsAffected()==0 → ErrXxxNotFound, нормализация NULL-able
//     значений (tags). Это дёшево и даёт точную ошибку без лишнего round-trip.
//
//  2. Реактивный — всё, что проактивные проверки не поймали (а БД остаётся
//     источником истины: ограничения, гонки, сбои), разбирается по SQLSTATE
//     из *pgconn.PgError (mapError). Результат — *domain.DBError с категорией,
//     которую хендлер превращает в конкретный HTTP-ответ.

// ---------- проактивный слой ----------

const maxTaskNameLen = 255 // tasks.name VARCHAR(255)

func validateCreateTask(p domain.CreateTaskParams) error {
	if strings.TrimSpace(p.Name) == "" ||
		utf8.RuneCountInString(p.Name) > maxTaskNameLen ||
		strings.ContainsRune(p.Name, 0) { // Postgres не принимает NUL-байт в тексте
		return domain.ErrInvalidTaskName
	}
	if !domain.ValidTaskTypes[p.Type] {
		return domain.ErrInvalidTaskType
	}
	return nil
}

// checkID отсекает невалидный UUID до запроса: иначе Postgres ответит 22P02,
// а пользователю нужно "invalid id", а не "internal error".
func checkID(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return domain.ErrInvalidID
	}
	return nil
}

// ---------- реактивный слой ----------

// wrap добавляет контекст операции и классифицирует ошибку: fmt.Errorf("%s: %w").
func wrap(op string, err error) error {
	if err == nil {
		return nil // fmt.Errorf("%w", nil) вернул бы НЕ-nil ошибку
	}
	return fmt.Errorf("%s: %w", op, mapError(err))
}

// mapError превращает ошибку драйвера в доменную. Уже классифицированные
// ошибки возвращаются как есть — повторная обёртка не нужна.
func mapError(err error) error {
	var (
		domErr *domain.Error
		dbErr  *domain.DBError
		pgErr  *pgconn.PgError
	)

	switch {
	case err == nil:
		return nil
	case errors.As(err, &domErr), errors.As(err, &dbErr):
		return err
	case errors.As(err, &pgErr):
		return &domain.DBError{
			Kind:       classify(pgErr),
			SQLState:   pgErr.Code,
			Constraint: pgErr.ConstraintName,
			Table:      pgErr.TableName,
			Column:     pgErr.ColumnName,
			Cause:      pgErr,
		}
	case errors.Is(err, context.DeadlineExceeded):
		return &domain.DBError{Kind: domain.ErrTimeout, Cause: err}
	default:
		return err // сетевые и прочие ошибки — наверх без изменений
	}
}

// SQLSTATE, которые мы различаем. Полный список: appendix A документации Postgres.
const (
	// Class 22 — Data Exception
	sqlStateStringDataRightTruncation = "22001"
	sqlStateNumericValueOutOfRange    = "22003"
	sqlStateInvalidDatetimeFormat     = "22007"
	sqlStateDatetimeFieldOverflow     = "22008"
	sqlStateCharacterNotInRepertoire  = "22021" // в т.ч. NUL-байт
	sqlStateInvalidTextRepresentation = "22P02" // битый uuid / json / число

	// Class 23 — Integrity Constraint Violation
	sqlStateNotNullViolation    = "23502"
	sqlStateForeignKeyViolation = "23503"
	sqlStateUniqueViolation     = "23505"
	sqlStateCheckViolation      = "23514"
	sqlStateExclusionViolation  = "23P01"

	// Class 40 / 55 / 57 — откат транзакции, блокировки, отмена
	sqlStateSerializationFailure = "40001"
	sqlStateDeadlockDetected     = "40P01"
	sqlStateLockNotAvailable     = "55P03"
	sqlStateQueryCanceled        = "57014"
)

// constraintKinds уточняет смысл нарушения по имени ограничения
// (имена — из миграций; автогенерированные: <table>_<column>_<fkey|key|check>).
var constraintKinds = map[string]*domain.Error{
	"users_email_key":              domain.ErrDuplicateEmail,
	"uq_task_lock":                 domain.ErrLockHeld,
	"tasks_user_id_fkey":           domain.ErrUserNotFound,
	"task_executions_task_id_fkey": domain.ErrTaskNotFound,
	"task_locks_task_id_fkey":      domain.ErrTaskNotFound,
	"tasks_type_check":             domain.ErrInvalidTaskType,
	"tasks_status_check":           domain.ErrInvalidTaskStatus,
	"chk_schedule":                 domain.ErrInvalidSchedule,
}

func classify(e *pgconn.PgError) *domain.Error {
	// 1. Самое конкретное: известное ограничение.
	if k, ok := constraintKinds[e.ConstraintName]; ok {
		return k
	}

	// 2. Точный SQLSTATE.
	switch e.Code {
	case sqlStateUniqueViolation, sqlStateExclusionViolation:
		return domain.ErrConflict
	case sqlStateForeignKeyViolation:
		return domain.ErrInvalidReference
	case sqlStateNotNullViolation:
		return domain.ErrRequiredField
	case sqlStateCheckViolation:
		return domain.ErrConstraintViolation
	case sqlStateStringDataRightTruncation:
		return domain.ErrValueTooLong
	case sqlStateInvalidTextRepresentation, sqlStateNumericValueOutOfRange,
		sqlStateInvalidDatetimeFormat, sqlStateDatetimeFieldOverflow,
		sqlStateCharacterNotInRepertoire:
		return domain.ErrInvalidValue
	case sqlStateSerializationFailure, sqlStateDeadlockDetected, sqlStateLockNotAvailable:
		return domain.ErrRetryable
	case sqlStateQueryCanceled:
		return domain.ErrTimeout
	}

	// 3. Класс (первые два символа).
	if len(e.Code) >= 2 {
		switch e.Code[:2] {
		case "08", "53", "57": // connection exception / нехватка ресурсов / остановка сервера
			return domain.ErrDBUnavailable
		case "22":
			return domain.ErrInvalidValue
		case "23":
			return domain.ErrConstraintViolation
		}
	}

	// 4. Всё остальное, включая 42xxx (ошибка запроса / нет таблицы или колонки).
	return domain.ErrDatabase
}
