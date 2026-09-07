package handlers_test

import (
	"OctoQueue/internal/domain"
	"OctoQueue/internal/http-server/handlers"
	"OctoQueue/internal/storage/repository/mocks"
	"bytes"
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

func TestCreateTask_EmptyType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := new(mocks.TaskRepository)

	body := `{
		"name":"backup"
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

func withChiURLParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestUpdateTask_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	updated := &domain.Task{ID: "123", Name: "new name"}

	repo.
		On(
			"UpdateTask",
			mock.Anything,
			mock.MatchedBy(func(p domain.UpdateTaskParams) bool {
				return p.ID == "123" &&
					p.Name != nil && *p.Name == "new name" &&
					p.Payload == nil &&
					p.Schedule == nil &&
					p.Timezone == nil &&
					p.MaxRetries == nil &&
					p.Tags == nil &&
					p.TargetHost == nil
			}),
		).
		Return(updated, nil)

	handler := handlers.UpdateTask(logger, repo)

	body := `{"name":"new name"}`
	req := httptest.NewRequest(http.MethodPatch, "/tasks/123", bytes.NewBufferString(body))
	req = withChiURLParam(req, "id", "123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var resp domain.Task
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Equal(t, "123", resp.ID)
	require.Equal(t, "new name", resp.Name)

	repo.AssertExpectations(t)
}

func TestUpdateTask_MissingID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	handler := handlers.UpdateTask(logger, repo)

	req := httptest.NewRequest(http.MethodPatch, "/tasks/", bytes.NewBufferString(`{}`))
	req = withChiURLParam(req, "id", "") // явно пустой id
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	repo.AssertNotCalled(t, "UpdateTask", mock.Anything, mock.Anything)
}

func TestUpdateTask_InvalidJSONBody(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	handler := handlers.UpdateTask(logger, repo)

	req := httptest.NewRequest(http.MethodPatch, "/tasks/123", bytes.NewBufferString(`{invalid json`))
	req = withChiURLParam(req, "id", "123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	repo.AssertNotCalled(t, "UpdateTask", mock.Anything, mock.Anything)
}

func TestUpdateTask_NotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	repo.
		On("UpdateTask", mock.Anything, mock.Anything).
		Return(nil, domain.ErrNotFound)

	handler := handlers.UpdateTask(logger, repo)

	req := httptest.NewRequest(http.MethodPatch, "/tasks/unknown", bytes.NewBufferString(`{"name":"x"}`))
	req = withChiURLParam(req, "id", "unknown")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	repo.AssertExpectations(t)
}

func TestUpdateTask_RepositoryError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	repo.
		On("UpdateTask", mock.Anything, mock.Anything).
		Return(nil, errors.New("db is down"))

	handler := handlers.UpdateTask(logger, repo)

	req := httptest.NewRequest(http.MethodPatch, "/tasks/123", bytes.NewBufferString(`{"name":"x"}`))
	req = withChiURLParam(req, "id", "123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	repo.AssertExpectations(t)
}

func TestUpdateTask_WithPayload(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	repo.
		On(
			"UpdateTask",
			mock.Anything,
			mock.MatchedBy(func(p domain.UpdateTaskParams) bool {
				return p.ID == "123" && string(p.Payload) == `{"key":"value"}`
			}),
		).
		Return(&domain.Task{ID: "123"}, nil)

	handler := handlers.UpdateTask(logger, repo)

	body := `{"payload":{"key":"value"}}`
	req := httptest.NewRequest(http.MethodPatch, "/tasks/123", bytes.NewBufferString(body))
	req = withChiURLParam(req, "id", "123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	repo.AssertExpectations(t)
}

func TestUpdateTask_PayloadArray(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	repo.
		On(
			"UpdateTask",
			mock.Anything,
			mock.MatchedBy(func(p domain.UpdateTaskParams) bool {
				return p.ID == "123" && string(p.Payload) == `[1,2,3]`
			}),
		).
		Return(&domain.Task{ID: "123"}, nil)

	handler := handlers.UpdateTask(logger, repo)

	body := `{"payload":[1,2,3]}`
	req := httptest.NewRequest(http.MethodPatch, "/tasks/123", bytes.NewBufferString(body))
	req = withChiURLParam(req, "id", "123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	repo.AssertExpectations(t)
}

func TestUpdateTask_AllFields(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	repo.
		On(
			"UpdateTask",
			mock.Anything,
			mock.MatchedBy(func(p domain.UpdateTaskParams) bool {
				return p.ID == "123" &&
					p.Name != nil && *p.Name == "renamed" &&
					p.Schedule != nil && *p.Schedule == "0 * * * *" &&
					p.Timezone != nil && *p.Timezone == "UTC" &&
					p.MaxRetries != nil && *p.MaxRetries == 5 &&
					len(p.Tags) == 2 && p.Tags[0] == "a" && p.Tags[1] == "b" &&
					p.TargetHost != nil && *p.TargetHost == "host-1"
			}),
		).
		Return(&domain.Task{ID: "123", Name: "renamed"}, nil)

	handler := handlers.UpdateTask(logger, repo)

	body := `{
		"name": "renamed",
		"schedule": "0 * * * *",
		"timezone": "UTC",
		"max_retries": 5,
		"tags": ["a", "b"],
		"target_host": "host-1"
	}`
	req := httptest.NewRequest(http.MethodPatch, "/tasks/123", bytes.NewBufferString(body))
	req = withChiURLParam(req, "id", "123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	repo.AssertExpectations(t)
}

func TestUpdateTask_EmptyBody(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	repo.
		On(
			"UpdateTask",
			mock.Anything,
			mock.MatchedBy(func(p domain.UpdateTaskParams) bool {
				return p.ID == "123" &&
					p.Name == nil &&
					p.Payload == nil &&
					p.Schedule == nil &&
					p.Timezone == nil &&
					p.MaxRetries == nil &&
					p.Tags == nil &&
					p.TargetHost == nil
			}),
		).
		Return(&domain.Task{ID: "123"}, nil)

	handler := handlers.UpdateTask(logger, repo)

	req := httptest.NewRequest(http.MethodPatch, "/tasks/123", bytes.NewBufferString(`{}`))
	req = withChiURLParam(req, "id", "123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	repo.AssertExpectations(t)
}

func TestDeleteTask_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	repo.
		On("DeleteTask", mock.Anything, "123").
		Return(nil)

	handler := handlers.DeleteTask(logger, repo)

	req := httptest.NewRequest(http.MethodDelete, "/tasks/123", nil)
	req = withChiURLParam(req, "id", "123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNoContent, rr.Code)
	require.Empty(t, rr.Body.Bytes())
	repo.AssertExpectations(t)
}

func TestDeleteTask_MissingID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	handler := handlers.DeleteTask(logger, repo)

	req := httptest.NewRequest(http.MethodDelete, "/tasks/", nil)
	req = withChiURLParam(req, "id", "") // явно пустой id
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	repo.AssertNotCalled(t, "DeleteTask", mock.Anything, mock.Anything)
}

func TestDeleteTask_NotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	repo.
		On("DeleteTask", mock.Anything, "unknown").
		Return(domain.ErrNotFound)

	handler := handlers.DeleteTask(logger, repo)

	req := httptest.NewRequest(http.MethodDelete, "/tasks/unknown", nil)
	req = withChiURLParam(req, "id", "unknown")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	repo.AssertExpectations(t)
}

func TestDeleteTask_RepositoryError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := new(mocks.TaskWriter)

	repo.
		On("DeleteTask", mock.Anything, "123").
		Return(errors.New("db is down"))

	handler := handlers.DeleteTask(logger, repo)

	req := httptest.NewRequest(http.MethodDelete, "/tasks/123", nil)
	req = withChiURLParam(req, "id", "123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	repo.AssertExpectations(t)
}

