package domain

import (
	"errors"
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
	ErrNotFound        = errors.New("not found")
	ErrForbidden       = errors.New("forbidden")
	ErrInvalidRequest  = errors.New("invalid request")
	ErrUnauthorized    = errors.New("unauthorized")
)

type User struct {
	ID        uuid.UUID    `json:"id"`
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
	UserID     *string
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
	UserID uuid.UUID
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
