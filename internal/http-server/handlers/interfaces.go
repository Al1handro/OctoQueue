package handlers

import (
	"OctoQueue/internal/storage"
	"log/slog"
	"net/http"
)

//go:generate go run github.com/vektra/mockery/v2@latest --name=TaskRequest --output=./mocks
type TaskRequest interface {
	Status(log *slog.Logger) http.HandlerFunc
	GetTask(log *slog.Logger, st storage.TaskRepository) http.HandlerFunc
	ListTasks(log *slog.Logger, st storage.TaskRepository) http.HandlerFunc
}

//go:generate go run github.com/vektra/mockery/v2@latest --name=UpdateTasks --output=./mocks
type UpdateTasks interface {
	CreateTask(log *slog.Logger, st storage.TaskRepository) http.HandlerFunc
	UpdateTask(log *slog.Logger, st storage.TaskWriter) http.HandlerFunc
	DeleteTask(log *slog.Logger, st storage.TaskWriter) http.HandlerFunc
}

//go:generate go run github.com/vektra/mockery/v2@latest --name=TaskExecutionRequest --output=./mocks
type TaskExecutionRequest interface {
	GetTaskExecutions(log *slog.Logger, st storage.ExecutionTracker) http.HandlerFunc
}