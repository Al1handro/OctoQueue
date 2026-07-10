package handlers_test

import (
	"OctoQueue/internal/domain"
	"OctoQueue/internal/http-server/handlers"
	"OctoQueue/internal/storage/repository/mocks"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestStatus(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rr := httptest.NewRecorder()

	handler := handlers.Status(logger)
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "text/plain", rr.Header().Get("Content-Type"))
	require.Equal(t, "OK", rr.Body.String())
}

func TestCreateTask_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	task := &domain.Task{
		ID:   "task-1",
		Name: "Backup",
		Type: "backup",
	}

	repo.
		On("CreateTask", mock.Anything, mock.Anything).
		Return(task, nil)

	body := `{
		"name":"Backup",
		"type":"backup"
	}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/tasks",
		strings.NewReader(body),
	)

	req.Header.Set("Content-Type", "application/json")

	userID := uuid.New()

	ctx := context.WithValue(
		req.Context(),
		domain.CtxUserID,
		userID,
	)

	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	handler := handlers.CreateTask(logger, repo)
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)

	var got domain.Task
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))

	require.Equal(t, task.ID, got.ID)
	require.Equal(t, task.Name, got.Name)

	repo.AssertExpectations(t)
}

func TestCreateTask_InvalidJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	req := httptest.NewRequest(
		http.MethodPost,
		"/tasks",
		strings.NewReader("{invalid json}"),
	)

	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()

	handler := handlers.CreateTask(logger, repo)
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	repo.AssertNotCalled(t, "CreateTask")
}

func TestCreateTask_EmptyName(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	body := `{
		"type":"backup"
	}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/tasks",
		strings.NewReader(body),
	)

	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()

	handler := handlers.CreateTask(logger, repo)
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	repo.AssertNotCalled(t, "CreateTask")
}

func TestCreateTask_InvalidRunAt(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	body := `{
		"name":"Backup",
		"type":"backup",
		"run_at":"hello"
	}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/tasks",
		strings.NewReader(body),
	)

	req.Header.Set("Content-Type", "application/json")

	userID := uuid.New()

	ctx := context.WithValue(
		req.Context(),
		domain.CtxUserID,
		userID,
	)

	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	handler := handlers.CreateTask(logger, repo)
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	repo.AssertNotCalled(t, "CreateTask")
}

func TestCreateTask_NoUserID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	body := `{
		"name":"Backup",
		"type":"backup"
	}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/tasks",
		strings.NewReader(body),
	)

	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()

	handler := handlers.CreateTask(logger, repo)
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)

	repo.AssertNotCalled(t, "CreateTask")
}

func TestCreateTask_RepositoryError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	repo.
		On("CreateTask", mock.Anything, mock.Anything).
		Return((*domain.Task)(nil), errors.New("db error"))

	body := `{
		"name":"Backup",
		"type":"backup"
	}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/tasks",
		strings.NewReader(body),
	)

	userID := uuid.New()

	ctx := context.WithValue(
		req.Context(),
		domain.CtxUserID,
		userID,
	)

	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	handler := handlers.CreateTask(logger, repo)
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)

	repo.AssertExpectations(t)
}

func TestGetTask_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	task := &domain.Task{
		ID:   "task-1",
		Name: "Backup",
		Type: "backup",
	}

	repo.
		On("GetTaskByID", mock.Anything, "task-1").
		Return(task, nil)

	r := chi.NewRouter()
	r.Get("/tasks/{id}", handlers.GetTask(logger, repo))

	req := httptest.NewRequest(http.MethodGet, "/tasks/task-1", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var got domain.Task
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))

	require.Equal(t, task.ID, got.ID)
	require.Equal(t, task.Name, got.Name)
	require.Equal(t, task.Type, got.Type)

	repo.AssertExpectations(t)
}

func TestGetTask_MissingID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	r := chi.NewRouter()
	r.Get("/tasks/{id}", handlers.GetTask(logger, repo))

	req := httptest.NewRequest(http.MethodGet, "/tasks/", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)

	repo.AssertNotCalled(t, "GetTaskByID")
}

func TestGetTask_EmptyURLParam(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	req := httptest.NewRequest(http.MethodGet, "/tasks/", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "")

	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	handler := handlers.GetTask(logger, repo)
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	repo.AssertNotCalled(t, "GetTaskByID")
}

func TestGetTask_NotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	repo.
		On("GetTaskByID", mock.Anything, "task-1").
		Return((*domain.Task)(nil), domain.ErrNotFound)

	r := chi.NewRouter()
	r.Get("/tasks/{id}", handlers.GetTask(logger, repo))

	req := httptest.NewRequest(http.MethodGet, "/tasks/task-1", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)

	repo.AssertExpectations(t)
}

func TestGetTask_InternalError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	repo.
		On("GetTaskByID", mock.Anything, "task-1").
		Return((*domain.Task)(nil), errors.New("database error"))

	r := chi.NewRouter()
	r.Get("/tasks/{id}", handlers.GetTask(logger, repo))

	req := httptest.NewRequest(http.MethodGet, "/tasks/task-1", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)

	repo.AssertExpectations(t)
}

func TestListTasks_Admin(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	tasks := []*domain.Task{
		{ID: "1", Name: "task1"},
		{ID: "2", Name: "task2"},
	}

	repo.
		On(
			"ListTasks",
			mock.Anything,
			mock.MatchedBy(func(p domain.ListTasksParams) bool {
				return p.UserID == nil
			}),
		).
		Return(tasks, nil)

	handler := handlers.ListTasks(logger, repo)

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)

	ctx := context.WithValue(req.Context(), domain.CtxRole, domain.RoleAdmin)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	repo.AssertExpectations(t)
}

func TestListTasks_User(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	userID := uuid.New()

	repo.
		On(
			"ListTasks",
			mock.Anything,
			mock.MatchedBy(func(p domain.ListTasksParams) bool {
				return p.UserID != nil &&
					*p.UserID == userID
			}),
		).
		Return([]*domain.Task{}, nil)

	handler := handlers.ListTasks(logger, repo)

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)

	ctx := context.WithValue(req.Context(), domain.CtxRole, domain.RoleUser)
	ctx = context.WithValue(ctx, domain.CtxUserID, userID)

	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	repo.AssertExpectations(t)
}

func TestListTasks_InvalidLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskRepository)

	handler := handlers.ListTasks(logger, repo)
	req := httptest.NewRequest(http.MethodGet, "/tasks?limit=abc", nil)
	ctx := context.WithValue(req.Context(), domain.CtxRole, domain.RoleAdmin)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	repo.AssertNotCalled(t, "ListTasks", mock.Anything, mock.Anything)
}

func TestListTasks_InvalidOffset(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskRepository)

	handler := handlers.ListTasks(logger, repo)
	req := httptest.NewRequest(http.MethodGet, "/tasks?offset=xyz", nil)
	ctx := context.WithValue(req.Context(), domain.CtxRole, domain.RoleAdmin)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	repo.AssertNotCalled(t, "ListTasks", mock.Anything, mock.Anything)
}

func TestListTasks_NegativeLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskRepository)

	handler := handlers.ListTasks(logger, repo)
	req := httptest.NewRequest(http.MethodGet, "/tasks?limit=-1", nil)
	ctx := context.WithValue(req.Context(), domain.CtxRole, domain.RoleAdmin)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	repo.AssertNotCalled(t, "ListTasks", mock.Anything, mock.Anything)
}

func TestListTasks_NegativeOffset(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskRepository)

	handler := handlers.ListTasks(logger, repo)
	req := httptest.NewRequest(http.MethodGet, "/tasks?offset=-5", nil)
	ctx := context.WithValue(req.Context(), domain.CtxRole, domain.RoleAdmin)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	repo.AssertNotCalled(t, "ListTasks", mock.Anything, mock.Anything)
}

func TestListTasks_MissingRole(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskRepository)

	handler := handlers.ListTasks(logger, repo)
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	ctx := context.WithValue(req.Context(), domain.CtxUserID, domain.RoleUser)

	rr := httptest.NewRecorder()
	req = req.WithContext(ctx)

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	repo.AssertNotCalled(t, "ListTasks", mock.Anything, mock.Anything)
}

func TestListTasks_MissingUserID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskRepository)

	handler := handlers.ListTasks(logger, repo)
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	ctx := context.WithValue(req.Context(), domain.CtxRole, domain.RoleUser)

	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	repo.AssertNotCalled(t, "ListTasks", mock.Anything, mock.Anything)
}

func TestListTasks_RepositoryError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskRepository)

	repo.
		On("ListTasks", mock.Anything, mock.Anything).
		Return(nil, errors.New("db is down"))

	handler := handlers.ListTasks(logger, repo)
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)

	ctx := context.WithValue(req.Context(), domain.CtxRole, domain.RoleAdmin)
	ctx = context.WithValue(ctx, domain.CtxUserID, uuid.New())
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	repo.AssertExpectations(t)
}

func TestListTasks_WithFilters(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskRepository)

	repo.
		On(
			"ListTasks",
			mock.Anything,
			mock.MatchedBy(func(p domain.ListTasksParams) bool {
				return p.UserID == nil &&
					p.Status != nil && *p.Status == "done" &&
					p.Type != nil && *p.Type == "bug" &&
					len(p.Tags) == 2 && p.Tags[0] == "backend" && p.Tags[1] == "urgent" &&
					p.Limit == 10 &&
					p.Offset == 5
			}),
		).
		Return([]*domain.Task{}, nil)

	handler := handlers.ListTasks(logger, repo)
	req := httptest.NewRequest(
		http.MethodGet,
		"/tasks?status=done&type=bug&tags=backend&tags=urgent&limit=10&offset=5",
		nil,
	)

	ctx := context.WithValue(req.Context(), domain.CtxRole, domain.RoleAdmin)
	ctx = context.WithValue(ctx, domain.CtxUserID, uuid.New())
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	repo.AssertExpectations(t)
}

func TestListTasks_EmptyResult(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskRepository)

	repo.
		On("ListTasks", mock.Anything, mock.Anything).
		Return([]*domain.Task{}, nil)

	handler := handlers.ListTasks(logger, repo)
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	
	ctx := context.WithValue(req.Context(), domain.CtxRole, domain.RoleAdmin)
	ctx = context.WithValue(ctx, domain.CtxUserID, uuid.New())
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	tasksField, ok := body["tasks"].([]any)
	require.True(t, ok)
	require.Empty(t, tasksField)

	repo.AssertExpectations(t)
}