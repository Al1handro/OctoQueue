package domain

import "fmt"

// Error — узел иерархии доменных ошибок.
//
// Ошибки образуют дерево: у конкретной ошибки есть родитель-категория
// (ErrTaskNotFound → ErrNotFound). Unwrap возвращает родителя, поэтому
//
//	errors.Is(err, domain.ErrTaskNotFound) // конкретика
//	errors.Is(err, domain.ErrNotFound)     // категория → HTTP 404
//
// истинны одновременно, а errors.As(err, &*domain.Error) вернёт самый
// конкретный узел — из него берутся Code и Message для ответа клиенту.
//
// Message безопасно показывать пользователю: в нём нет данных из БД.
type Error struct {
	Code    string // стабильный машиночитаемый код для API
	Message string // человекочитаемое описание
	parent  *Error
}

func (e *Error) Error() string { return e.Message }

// Unwrap возвращает родительскую категорию (nil у корневых ошибок).
// Явная проверка нужна, чтобы не вернуть интерфейс с typed-nil внутри.
func (e *Error) Unwrap() error {
	if e.parent == nil {
		return nil
	}
	return e.parent
}

func newRoot(code, msg string) *Error { return &Error{Code: code, Message: msg} }

func newError(parent *Error, code, msg string) *Error {
	return &Error{Code: code, Message: msg, parent: parent}
}

// Корневые категории. Каждой соответствует один HTTP-статус (см. handlers).
var (
	ErrNotFound       = newRoot("NOT_FOUND", "resource not found")                                 // 404
	ErrConflict       = newRoot("CONFLICT", "resource already exists or conflicts with its state") // 409
	ErrInvalidRequest = newRoot("INVALID_REQUEST", "invalid request")                              // 400
	ErrUnauthorized   = newRoot("UNAUTHORIZED", "unauthorized")                                    // 401
	ErrForbidden      = newRoot("FORBIDDEN", "forbidden")                                          // 403
	ErrUnavailable    = newRoot("SERVICE_UNAVAILABLE", "service temporarily unavailable")          // 503
	ErrInternal       = newRoot("INTERNAL_ERROR", "internal error")                                // 500
)

// ErrNotFound
var (
	ErrUserNotFound      = newError(ErrNotFound, "USER_NOT_FOUND", "user not found")
	ErrTaskNotFound      = newError(ErrNotFound, "TASK_NOT_FOUND", "task not found")
	ErrExecutionNotFound = newError(ErrNotFound, "EXECUTION_NOT_FOUND", "task execution not found")
	ErrLockNotFound      = newError(ErrNotFound, "LOCK_NOT_FOUND", "task lock not found")
)

// ErrConflict
var (
	ErrDuplicateEmail = newError(ErrConflict, "EMAIL_TAKEN", "email already taken")
	ErrDuplicateTask  = newError(ErrConflict, "DUPLICATE_TASK", "task already exists")
	ErrLockHeld       = newError(ErrConflict, "LOCK_HELD", "task is locked by another worker")
)

// ErrInvalidRequest
var (
	ErrInvalidID           = newError(ErrInvalidRequest, "INVALID_ID", "invalid id format")
	ErrInvalidTaskName     = newError(ErrInvalidRequest, "INVALID_TASK_NAME", "task name must be 1-255 characters")
	ErrInvalidTaskType     = newError(ErrInvalidRequest, "INVALID_TASK_TYPE", "unsupported task type")
	ErrInvalidTaskStatus   = newError(ErrInvalidRequest, "INVALID_TASK_STATUS", "unsupported task status")
	ErrInvalidSchedule     = newError(ErrInvalidRequest, "INVALID_SCHEDULE", "schedule must be a valid cron expression")
	ErrRequiredField       = newError(ErrInvalidRequest, "REQUIRED_FIELD", "required field is missing")
	ErrValueTooLong        = newError(ErrInvalidRequest, "VALUE_TOO_LONG", "value is too long")
	ErrInvalidValue        = newError(ErrInvalidRequest, "INVALID_VALUE", "value has an invalid format")
	ErrInvalidReference    = newError(ErrInvalidRequest, "INVALID_REFERENCE", "referenced resource does not exist")
	ErrConstraintViolation = newError(ErrInvalidRequest, "CONSTRAINT_VIOLATION", "value violates a data constraint")
)

// ErrUnavailable
var (
	ErrDBUnavailable = newError(ErrUnavailable, "DB_UNAVAILABLE", "storage is temporarily unavailable")
	ErrRetryable     = newError(ErrUnavailable, "RETRY_LATER", "conflict with a concurrent operation, retry later")
	ErrTimeout       = newError(ErrUnavailable, "TIMEOUT", "operation timed out")
)

// ErrInternal
var (
	// ErrDatabase — любая SQLSTATE, которую мы не умеем классифицировать
	// (в т.ч. 42xxx: ошибка в запросе или рассинхрон схемы). Детали — только в логах.
	ErrDatabase = newError(ErrInternal, "DATABASE_ERROR", "database error")
)

// DBError — ошибка БД, разобранная по SQLSTATE. Вложенная: в одном значении
// лежат и смысл для приложения (Kind), и исходная ошибка драйвера (Cause).
//
// Unwrap() []error позволяет errors.Is / errors.As идти по обеим веткам:
//
//	errors.Is(err, domain.ErrDuplicateEmail)    // Kind
//	errors.Is(err, domain.ErrConflict)          // родитель Kind
//	errors.As(err, &pgErr /* *pgconn.PgError */) // Cause — сырые детали Postgres
//
// Пакет domain намеренно не импортирует pgx: поля — обычные строки.
type DBError struct {
	Kind       *Error // классификация: что случилось с точки зрения приложения
	SQLState   string // пять символов SQLSTATE, напр. "23505"; пусто для не-PG ошибок
	Constraint string // имя нарушенного ограничения, если есть
	Table      string
	Column     string // колонка (заполняется, например, для 23502)
	Cause      error  // исходная ошибка драйвера
}

func (e *DBError) Error() string {
	msg := ErrDatabase.Message
	if e.Kind != nil {
		msg = e.Kind.Message
	}
	if e.Cause != nil {
		// У *pgconn.PgError Error() содержит текст и SQLSTATE, но не Detail
		// (а в Detail могут быть значения из данных).
		return msg + ": " + e.Cause.Error()
	}
	return fmt.Sprintf("%s (SQLSTATE %s)", msg, e.SQLState)
}

func (e *DBError) Unwrap() []error {
	errs := make([]error, 0, 2)
	if e.Kind != nil {
		errs = append(errs, e.Kind)
	}
	if e.Cause != nil {
		errs = append(errs, e.Cause)
	}
	return errs
}
