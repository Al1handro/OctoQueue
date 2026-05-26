package storage

import (
	"context"
	"time"
)

//go:generate go run github.com/vektra/mockery/v2@latest --name=TaskRepository --output=./mocks
type TaskRepository interface {
	CreateTask(ctx context.Context, p CreateTaskParams) (*Task, error)
	GetTaskByID(ctx context.Context, id string) (*Task, error)
	ListTasks(ctx context.Context, p ListTasksParams) ([]*Task, error)
}

//go:generate go run github.com/vektra/mockery/v2@latest --name=TaskWriter --output=./mocks
type TaskWriter interface {
	UpdateTask(ctx context.Context, p UpdateTaskParams) (*Task, error)
	UpdateTaskStatus(ctx context.Context, id, status string, nextRunAt *time.Time) error
	DeleteTask(ctx context.Context, id string) error
}

//go:generate go run github.com/vektra/mockery/v2@latest --name=TaskScheduler --output=./mocks
type TaskScheduler interface {
	AcquirePendingTasks(ctx context.Context, workerID string, limit int) ([]*Task, error)
}

//go:generate go run github.com/vektra/mockery/v2@latest --name=ExecutionTracker --output=./mocks
type ExecutionTracker interface {
	CreateExecution(ctx context.Context, p CreateExecutionParams) (*TaskExecution, error)
	FinishExecution(ctx context.Context, p FinishExecutionParams) (*TaskExecution, error)
	ListExecutions(ctx context.Context, taskID string, limit int) ([]*TaskExecution, error)
}
	
//go:generate go run github.com/vektra/mockery/v2@latest --name=LockManager --output=./mocks
type LockManager interface {	
	AcquireLock(ctx context.Context, taskID, workerID string, ttl time.Duration) (*TaskLock, error)
	ReleaseLock(ctx context.Context, taskID, workerID string) error
	CleanExpiredLocks(ctx context.Context) (int64, error)
}

type Task struct {
	ID         string
	Name       string
	Type       string
	Payload    []byte
	Schedule   *string
	Timezone   string
	Status     string
	NextRunAt  *time.Time
	LastRunAt  *time.Time
	StartedAt  *time.Time
	Retries    int
	MaxRetries int
	RetryDelay time.Duration
	Timeout    time.Duration
	TargetHost *string
	Tags       []string
	CreatedBy  *string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  *time.Time
}

type TaskExecution struct {
	ID         string
	TaskID     string
	Status     string
	Attempt    int
	StartedAt  time.Time
	FinishedAt *time.Time
	Duration   *time.Duration
	Request    []byte
	Response   []byte
	ErrorCode  *string
	ErrorMsg   *string
	ErrorTrace *string
	WorkerID   *string
	CreatedAt  time.Time
}

type TaskLock struct {
	ID         int
	TaskID     string
	WorkerID   string
	AcquiredAt time.Time
	ExpiresAt  time.Time
}

type CreateTaskParams struct {
	Name       string
	Type       string
	Payload    []byte
	Schedule   *string
	Timezone   string
	NextRunAt  *time.Time
	MaxRetries int
	Tags       []string
	CreatedBy  *string
	TargetHost *string
}

type ListTasksParams struct {
	Status *string
	Type   *string
	Tags   []string
	Limit  int
	Offset int
}

type UpdateTaskParams struct {
	ID         string
	Name       *string
	Payload    []byte
	Schedule   *string
	Timezone   *string
	MaxRetries *int
	Tags       []string
	TargetHost *string
}

type CreateExecutionParams struct {
	TaskID   string
	Attempt  int
	WorkerID *string
	Request  []byte
}

type FinishExecutionParams struct {
	ID         string
	Status     string
	Response   []byte
	ErrorCode  *string
	ErrorMsg   *string
	ErrorTrace *string
}
