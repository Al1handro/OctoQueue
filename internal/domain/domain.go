package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type СontextKey string

const (
	CtxUserID СontextKey = "user_id"
	CtxRole   СontextKey = "role"
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUserNotFound = fmt.Errorf("user: %w", ErrNotFound)
	ErrTaskNotFound = fmt.Errorf("task: %w", ErrNotFound)

	ErrInvalidTaskName = errors.New("invalid task name")
	ErrInvalidTaskType = errors.New("invalid task type")

	ErrForbidden         = errors.New("forbidden")
	ErrInvalidRequest    = errors.New("invalid request")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrUserAlreadyExists = errors.New("user already exists")

	ErrDuplicateEmail = errors.New("email already taken")
	ErrDuplicateTask  = errors.New("duplicate task")

	ErrTaskStatusPending = errors.New("task status: pending")
	TaskStatusRunning    = errors.New("task status: running")
	TaskStatusSuccess    = errors.New("task status: success")
	TaskStatusFailed     = errors.New("task status: failed")
)

var ValidTaskTypes = map[string]bool{
	"http_call": true,
	"shell":     true,
	"email":     true,
	"grpc":      true,
	"kafka":     true,
}

type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
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
	UserID     uuid.UUID
}

type TaskExecution struct {
	ID         *string
	TaskID     *string
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
	UserID     uuid.UUID
}

type ListTasksParams struct {
	Status *string
	Type   *string
	Tags   []string
	Limit  int
	Offset int
	UserID *uuid.UUID
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
