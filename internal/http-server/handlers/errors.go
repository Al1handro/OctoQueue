package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"OctoQueue/internal/domain"
	"OctoQueue/internal/lib/logger/sl"
)

// statusFor сопоставляет корневую категорию доменной ошибки с HTTP-статусом.
// Благодаря Unwrap достаточно проверить категорию: любая вложенная ошибка
// (ErrInvalidTaskType, ErrDuplicateEmail, …) попадёт в свою ветку сама.
// Порядок важен: ErrTimeout вложена в ErrUnavailable, поэтому проверяется раньше.
func statusFor(err error) int {
	switch {
	case errors.Is(err, domain.ErrInvalidRequest):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, domain.ErrTimeout):
		return http.StatusGatewayTimeout
	case errors.Is(err, domain.ErrUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// writeServiceError превращает ошибку из репозитория/сервиса в HTTP-ответ.
//
//   - 4xx/503/504: клиент получает code и message ближайшей доменной ошибки
//     (например INVALID_TASK_TYPE или EMAIL_TAKEN), а для DBError ещё и поле field.
//   - 500 и неклассифицированные ошибки: наружу уходит только failMsg,
//     подробности (SQLSTATE, текст ошибки Postgres) остаются в логе.
//
// Сырые сообщения Postgres клиенту не отдаются намеренно: в них бывают имена
// ограничений и значения из данных.
func writeServiceError(w http.ResponseWriter, logger *slog.Logger, err error, failMsg string, attrs ...any) {
	status := statusFor(err)

	var dbErr *domain.DBError
	if errors.As(err, &dbErr) {
		attrs = append(attrs,
			slog.String("sqlstate", dbErr.SQLState),
			slog.String("constraint", dbErr.Constraint),
		)
	}
	attrs = append(attrs, sl.Err(err))

	if status == http.StatusInternalServerError {
		logger.Error(failMsg, attrs...)
		writeError(w, status, domain.ErrInternal.Code, failMsg)
		return
	}

	// Ожидаемые отказы (плохой ввод, дубликат) — не авария сервера.
	if status >= http.StatusInternalServerError {
		logger.Error(failMsg, attrs...)
	} else {
		logger.Warn(failMsg, attrs...)
	}

	detail := errDetail{Code: domain.ErrInternal.Code, Message: failMsg}
	var de *domain.Error
	if errors.As(err, &de) {
		detail.Code, detail.Message = de.Code, de.Message
	}
	if dbErr != nil && status < http.StatusInternalServerError {
		detail.Field = dbErr.Column
	}

	writeJSON(w, status, errBody{Error: detail})
}
