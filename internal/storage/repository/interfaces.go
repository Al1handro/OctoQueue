package repository

import (
	"OctoQueue/internal/domain"
	"context"
	"time"
)

//go:generate go run github.com/vektra/mockery/v2@latest --name=UserRepository --output=./mocks
type UserRepository interface {
	CreateUser(ctx context.Context, user *domain.User) error
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUser(ctx context.Context, id string) (*domain.User, error)
	// GetByID(ctx context.Context, id string) (*domain.User, error)
}

//go:generate go run github.com/vektra/mockery/v2@latest --name=TaskRepository --output=./mocks
type TaskRepository interface {
	CreateTask(ctx context.Context, p domain.CreateTaskParams) (*domain.Task, error)
	GetTaskByID(ctx context.Context, id string) (*domain.Task, error)
	ListTasks(ctx context.Context, p domain.ListTasksParams) ([]*domain.Task, error)
}

//go:generate go run github.com/vektra/mockery/v2@latest --name=TaskWriter --output=./mocks
type TaskWriter interface {
	UpdateTask(ctx context.Context, p domain.UpdateTaskParams) (*domain.Task, error)
	UpdateTaskStatus(ctx context.Context, id, status string, nextRunAt *time.Time) error
	DeleteTask(ctx context.Context, id string) error
}

//go:generate go run github.com/vektra/mockery/v2@latest --name=TaskScheduler --output=./mocks
type TaskScheduler interface {
	AcquirePendingTasks(ctx context.Context, workerID string, limit int) ([]*domain.Task, error)
}

//go:generate go run github.com/vektra/mockery/v2@latest --name=ExecutionTracker --output=./mocks
type ExecutionTracker interface {
	CreateExecution(ctx context.Context, p domain.CreateExecutionParams) (*domain.TaskExecution, error)
	FinishExecution(ctx context.Context, p domain.FinishExecutionParams) (*domain.TaskExecution, error)
	ListExecutions(ctx context.Context, taskID string, limit int) ([]*domain.TaskExecution, error)
}
	
//go:generate go run github.com/vektra/mockery/v2@latest --name=LockManager --output=./mocks
type LockManager interface {	
	AcquireLock(ctx context.Context, taskID, workerID string, ttl time.Duration) (*domain.TaskLock, error)
	ReleaseLock(ctx context.Context, taskID, workerID string) error
	CleanExpiredLocks(ctx context.Context) (int64, error)
}

