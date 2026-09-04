package handlers

import (
	"OctoQueue/internal/domain"
	"OctoQueue/internal/http-server/middleware/auth"
	"OctoQueue/internal/lib/logger/sl"
	"OctoQueue/internal/storage/repository"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// Request / Response types

type CreateTaskRequest struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Payload    any      `json:"payload"`
	Schedule   *string  `json:"schedule,omitempty"`
	Timezone   string   `json:"timezone,omitempty"`
	MaxRetries int      `json:"max_retries,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	CreatedBy  *string  `json:"created_by,omitempty"`
	TargetHost *string  `json:"target_host,omitempty"`
	RunAt      *string  `json:"run_at,omitempty"` // для одноразовых задач: RFC3339
}

type UpdateTaskRequest struct {
	Name       *string  `json:"name,omitempty"`
	Payload    any      `json:"payload,omitempty"`
	Schedule   *string  `json:"schedule,omitempty"`
	Timezone   *string  `json:"timezone,omitempty"`
	MaxRetries *int     `json:"max_retries,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	TargetHost *string  `json:"target_host,omitempty"`
}

// Status GET /status

func Status(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.Status"
		logger := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		if _, err := w.Write([]byte("OK")); err != nil {
			logger.Error("failed to write response", sl.Err(err))
		}
	}
}

// CreateTask POST /tasks

func CreateTask(log *slog.Logger, st repository.TaskRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.CreateTask"
		logger := log.With(slog.String("op", op), slog.String("request_id", middleware.GetReqID(r.Context())))

		var req CreateTaskRequest
		if err := decodeJSON(r, &req); err != nil {
			logger.Error("failed to decode request", sl.Err(err))
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}

		if req.Name == "" || req.Type == "" {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "name and type are required")
			return
		}

		payloadBytes, err := json.Marshal(req.Payload)
		if err != nil {
			logger.Error("failed to marshal payload", sl.Err(err))
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid payload")
			return
		}

		timezone := req.Timezone
		if timezone == "" {
			timezone, _ = time.Now().Zone()
		}

		var nextRunAt *time.Time
		if req.RunAt != nil {
			t, err := time.Parse(time.RFC3339, *req.RunAt)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid run_at format, use RFC3339")
				return
			}
			nextRunAt = &t
		}

		maxRetries := req.MaxRetries
		if maxRetries == 0 {
			maxRetries = 3
		}

		userId, err := auth.GetUserID(r.Context())
		if errors.Is(err, auth.ErrUserIDNotFound) {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "missing auth context")
			return
		}

		task, err := st.CreateTask(r.Context(), domain.CreateTaskParams{
			Name:       req.Name,
			Type:       req.Type,
			Payload:    payloadBytes,
			Schedule:   req.Schedule,
			Timezone:   timezone,
			NextRunAt:  nextRunAt,
			MaxRetries: maxRetries,
			Tags:       req.Tags,
			CreatedBy:  req.CreatedBy,
			TargetHost: req.TargetHost,
			UserID:     userId,
		})
		if err != nil {
			logger.Error("failed to create task", sl.Err(err))
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create task")
			return
		}

		logger.Info("task created", slog.String("task_id", task.ID))
		writeJSON(w, http.StatusCreated, task)
	}
}

// GetTask GET /tasks/{id}

func GetTask(log *slog.Logger, st repository.TaskRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.GetTask"
		logger := log.With(slog.String("op", op), slog.String("request_id", middleware.GetReqID(r.Context())))

		id := chi.URLParam(r, "id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing task id")
			return
		}

		task, err := st.GetTaskByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", "task not found")
				return
			}
			logger.Error("failed to get task", sl.Err(err), slog.String("task_id", id))
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get task")
			return
		}

		writeJSON(w, http.StatusOK, task)
	}
}

// ListTasks GET /tasks?status=&type=&tags=&limit=&offset=

func ListTasks(log *slog.Logger, st repository.TaskRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.ListTasks"
		logger := log.With(slog.String("op", op), slog.String("request_id", middleware.GetReqID(r.Context())))

		q := r.URL.Query()

		var status *string
		if s := q.Get("status"); s != "" {
			status = &s
		}

		var taskType *string
		if t := q.Get("type"); t != "" {
			taskType = &t
		}

		var tags []string
		if t := q["tags"]; len(t) > 0 {
			tags = t
		}

		var limit, offset int

		if lStr := q.Get("limit"); lStr != "" {
			parsed, err := strconv.Atoi(lStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_PARAM", "limit must be an integer")
				return
			}
			limit = parsed
		}

		if oStr := q.Get("offset"); oStr != "" {
			parsed, err := strconv.Atoi(oStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_PARAM", "offset must be an integer")
				return
			}
			offset = parsed
		}

		if limit < 0 {
			writeError(w, http.StatusBadRequest, "INVALID_PARAM", "limit must be >= 0")
			return
		}
		if offset < 0 {
			writeError(w, http.StatusBadRequest, "INVALID_PARAM", "offset must be >= 0")
			return
		}

		role, err := auth.GetRole(r.Context())
		if err != nil {
			err = fmt.Errorf("%s: get role: %w", op, err)
			logger.Error("failed to get role", sl.Err(err))
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "missing auth context")
			return
		}

		var userId *uuid.UUID
		if role != domain.RoleAdmin {
			stepId, err := auth.GetUserID(r.Context())
			if err != nil {
				err = fmt.Errorf("%s: get user id: %w", op, err)
				logger.Error("failed to get user id", sl.Err(err))
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "missing auth context")
				return
			}
			userId = &stepId
		}

		tasks, err := st.ListTasks(r.Context(), domain.ListTasksParams{
			UserID: userId,
			Status: status,
			Type:   taskType,
			Tags:   tags,
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			err = fmt.Errorf("%s: list tasks: %w", op, err)
			logger.Error("failed to list tasks", sl.Err(err))
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list tasks")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"tasks":  tasks,
			"offset": offset,
			"limit":  limit,
		})
	}
}

// UpdateTask PATCH /tasks/{id}

func UpdateTask(log *slog.Logger, st repository.TaskWriter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.UpdateTask"
		logger := log.With(slog.String("op", op), slog.String("request_id", middleware.GetReqID(r.Context())))

		id := chi.URLParam(r, "id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing task id")
			return
		}

		var req UpdateTaskRequest
		if err := decodeJSON(r, &req); err != nil {
			logger.Error("failed to decode request", sl.Err(err))
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}

		var payloadBytes []byte
		if req.Payload != nil {
			b, err := json.Marshal(req.Payload)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid payload")
				return
			}
			payloadBytes = b
		}

		task, err := st.UpdateTask(r.Context(), domain.UpdateTaskParams{
			ID:         id,
			Name:       req.Name,
			Payload:    payloadBytes,
			Schedule:   req.Schedule,
			Timezone:   req.Timezone,
			MaxRetries: req.MaxRetries,
			Tags:       req.Tags,
			TargetHost: req.TargetHost,
		})
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", "task not found")
				return
			}
			logger.Error("failed to update task", sl.Err(err), slog.String("task_id", id))
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update task")
			return
		}

		logger.Info("task updated", slog.String("task_id", id))
		writeJSON(w, http.StatusOK, task)
	}
}

// DeleteTask DELETE /tasks/{id}

func DeleteTask(log *slog.Logger, st repository.TaskWriter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.DeleteTask"
		logger := log.With(slog.String("op", op), slog.String("request_id", middleware.GetReqID(r.Context())))

		id := chi.URLParam(r, "id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing task id")
			return
		}

		if err := st.DeleteTask(r.Context(), id); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", "task not found")
				return
			}
			logger.Error("failed to delete task", sl.Err(err), slog.String("task_id", id))
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete task")
			return
		}

		logger.Info("task deleted", slog.String("task_id", id))
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetTaskExecutions GET /tasks/{id}/executions?limit=

func GetTaskExecutions(log *slog.Logger, st repository.ExecutionTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.GetTaskExecutions"
		logger := log.With(slog.String("op", op), slog.String("request_id", middleware.GetReqID(r.Context())))

		id := chi.URLParam(r, "id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing task id")
			return
		}

		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid limit")
			return
		}

		userId, err := auth.GetUserID(r.Context())
		if errors.Is(err, auth.ErrUserIDNotFound) {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "missing auth context")
			return
		}

		executions, err := st.ListExecutions(r.Context(), id, userId, limit)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", "task not found")
				return
			}
			logger.Error("failed to get executions", sl.Err(err), slog.String("task_id", id))
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get executions")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"task_id":    id,
			"executions": executions,
		})
	}
}
