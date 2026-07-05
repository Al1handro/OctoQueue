package handlers

import (
	"OctoQueue/internal/storage/repository"
	"log/slog"
	"net/http"
)

type TaskRequest interface {
	Status(log *slog.Logger) http.HandlerFunc
	GetTask(log *slog.Logger, st repository.TaskRepository) http.HandlerFunc
	ListTasks(log *slog.Logger, st repository.TaskRepository) http.HandlerFunc
}

type UpdateTasks interface {
	CreateTask(log *slog.Logger, st repository.TaskRepository) http.HandlerFunc
	UpdateTask(log *slog.Logger, st repository.TaskWriter) http.HandlerFunc
	DeleteTask(log *slog.Logger, st repository.TaskWriter) http.HandlerFunc
}

type TaskExecutionRequest interface {
	GetTaskExecutions(log *slog.Logger, st repository.ExecutionTracker) http.HandlerFunc
}