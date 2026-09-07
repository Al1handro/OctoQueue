package repository_test

import (
	"OctoQueue/internal/domain"
	"OctoQueue/internal/storage/repository"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testDSN    string = "postgres://postgres:password@localhost:5432/app?sslmode=disable"
	testLogger        = slog.New(slog.NewTextHandler(io.Discard, nil))
)

func TestMain(m *testing.M) {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s, err := repository.NewStorage(ctx, testDSN, testLogger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration tests skipped: cannot reach test database at %s: %v\n", testDSN, err)
		os.Exit(0)
	}
	s.Close()

	os.Exit(m.Run())
}

func ptr[T any](v T) *T {
	return &v
}

func newTestStorage(t *testing.T) *repository.Storage {
	t.Helper()

	s, err := repository.NewStorage(context.Background(), testDSN, testLogger)
	require.NoError(t, err, "could not connect to test db")
	t.Cleanup(s.Close)

	return s
}

func createTestUser(t *testing.T, s *repository.Storage) *domain.User {
	t.Helper()

	safeName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	u := &domain.User{
		Email:     fmt.Sprintf("test-%s-%d@example.com", safeName, time.Now().UnixNano()),
		Password:  "hashedpassword",
		Role:      domain.RoleUser,
		CreatedAt: time.Now(),
	}

	repo := repository.NewUserRepository(s.Pool())
	require.NoError(t, repo.CreateUser(context.Background(), u), "could not create user")

	t.Cleanup(func() {
		_, err := s.Pool().Exec(context.Background(), "DELETE FROM users WHERE id = $1", u.ID)
		if err != nil {
			t.Logf("warning: failed to delete user %s: %v", u.ID, err)
		}
	})

	return u
}

func trackTask(t *testing.T, s *repository.Storage, task *domain.Task) {
	t.Helper()

	t.Cleanup(func() {
		_, err := s.Pool().Exec(context.Background(), "DELETE FROM tasks WHERE id = $1", task.ID)
		if err != nil {
			t.Logf("warning: failed to delete task %s: %v", task.ID, err)
		}
	})
}

func TestStorage_CreateTask(t *testing.T) {
	s := newTestStorage(t)
	user := createTestUser(t, s)

	schedule := "0 0 * * *"
	createdBy := "test-user"
	targetHost := "example.com"

	tests := []struct {
		name    string
		p       domain.CreateTaskParams
		want    func(t *testing.T, got *domain.Task)
		wantErr bool
		errMsg  string
	}{
		{
			name: "successful creation of http_call task",
			p: domain.CreateTaskParams{
				Name:       "Daily backup",
				Type:       "http_call",
				Payload:    []byte(`{"url":"https://example.com/api","method":"POST"}`),
				Schedule:   &schedule,
				Timezone:   "UTC",
				MaxRetries: 3,
				Tags:       []string{"production", "critical"},
				CreatedBy:  &createdBy,
				TargetHost: &targetHost,
				UserID:     user.ID,
			},
			want: func(t *testing.T, got *domain.Task) {
				t.Helper()
				assert.NotEmpty(t, got.ID, "expected non-empty ID")
				assert.Equal(t, "pending", got.Status)
				assert.Len(t, got.Tags, 2)
				assert.ElementsMatch(t, []string{"production", "critical"}, got.Tags)
				assert.False(t, got.CreatedAt.IsZero(), "expected non-zero CreatedAt")
				assert.NotNil(t, got.Schedule)
				assert.Equal(t, schedule, *got.Schedule)
				assert.NotNil(t, got.TargetHost)
				assert.Equal(t, targetHost, *got.TargetHost)
				assert.JSONEq(t, `{"url":"https://example.com/api","method":"POST"}`, string(got.Payload))
			},
		},
		{
			name: "one-time task with run_at",
			p: domain.CreateTaskParams{
				Name:      "Deploy v2",
				Type:      "shell",
				Payload:   []byte(`{"command":"kubectl apply -f deploy.yaml"}`),
				Tags:      []string{"deployment", "test"},
				Timezone:  "UTC",
				NextRunAt: ptr(time.Now().Add(5 * time.Minute)),
				UserID:    user.ID,
			},
			want: func(t *testing.T, got *domain.Task) {
				t.Helper()
				assert.NotNil(t, got.NextRunAt, "expected non-nil NextRunAt")
				assert.Nil(t, got.Schedule, "expected nil Schedule for one-time task")
			},
		},
		{
			name: "task with retry configuration",
			p: domain.CreateTaskParams{
				Name:       "Retry task",
				Type:       "http_call",
				Payload:    []byte(`{"url":"https://example.com"}`),
				Schedule:   &schedule,
				Timezone:   "UTC",
				MaxRetries: 5,
				UserID:     user.ID,
			},
			want: func(t *testing.T, got *domain.Task) {
				t.Helper()
				assert.Equal(t, 5, got.MaxRetries)
				assert.NotNil(t, got.RetryDelay)
				assert.NotNil(t, got.Timeout)
				assert.Equal(t, 0, got.Retries, "new task should have 0 retries")
			},
		},
		{
			name: "task with empty tags",
			p: domain.CreateTaskParams{
				Name:     "Simple task",
				Type:     "shell",
				Payload:  []byte(`{"command":"echo hello"}`),
				Timezone: "UTC",
				UserID:   user.ID,
			},
			want: func(t *testing.T, got *domain.Task) {
				t.Helper()
				assert.Empty(t, got.Tags, "expected empty tags")
				assert.Nil(t, got.Schedule, "expected nil schedule")
				assert.Nil(t, got.TargetHost, "expected nil target_host")
				assert.Nil(t, got.CreatedBy, "expected nil created_by")
				assert.Equal(t, "pending", got.Status)
				assert.NotNil(t, got.NextRunAt, "expected NextRunAt to be set for one-time task")
			},
		},
		{
			name: "task with timezone",
			p: domain.CreateTaskParams{
				Name:     "Timezone task",
				Type:     "http_call",
				Payload:  []byte(`{"url":"https://example.com"}`),
				Schedule: &schedule,
				Timezone: "America/New_York",
				UserID:   user.ID,
			},
			want: func(t *testing.T, got *domain.Task) {
				t.Helper()
				assert.Equal(t, "America/New_York", got.Timezone)
				assert.NotNil(t, got.Schedule)
			},
		},
		{
			name: "task with multiple tags",
			p: domain.CreateTaskParams{
				Name:     "Multi-tag task",
				Type:     "shell",
				Payload:  []byte(`{"command":"ls"}`),
				Tags:     []string{"dev", "staging", "prod", "critical", "urgent"},
				Timezone: "UTC",
				UserID:   user.ID,
			},
			want: func(t *testing.T, got *domain.Task) {
				t.Helper()
				assert.Len(t, got.Tags, 5)
				assert.ElementsMatch(t, []string{"dev", "staging", "prod", "critical", "urgent"}, got.Tags)
			},
		},
		{
			name: "task with null created_by and target_host",
			p: domain.CreateTaskParams{
				Name:     "Minimal task",
				Type:     "shell",
				Payload:  []byte(`{"command":"pwd"}`),
				Timezone: "UTC",
				UserID:   user.ID,
			},
			want: func(t *testing.T, got *domain.Task) {
				t.Helper()
				assert.Nil(t, got.CreatedBy, "expected nil created_by")
				assert.Nil(t, got.TargetHost, "expected nil target_host")
				assert.False(t, got.CreatedAt.IsZero(), "expected non-zero CreatedAt")
				assert.False(t, got.UpdatedAt.IsZero(), "expected non-zero UpdatedAt")
				assert.Nil(t, got.DeletedAt, "expected nil deleted_at for new task")
				assert.Nil(t, got.LastRunAt, "expected nil last_run_at for new task")
				assert.Nil(t, got.StartedAt, "expected nil started_at for new task")
			},
		},
		{
			name: "task with invalid user_id",
			p: domain.CreateTaskParams{
				Name:     "Invalid user task",
				Type:     "shell",
				Payload:  []byte(`{"command":"test"}`),
				Timezone: "UTC",
				UserID:   uuid.Nil,
			},
			wantErr: true,
			errMsg:  "violates foreign key constraint",
		},
		{
			name: "task with empty name",
			p: domain.CreateTaskParams{
				Name:     "",
				Type:     "shell",
				Payload:  []byte(`{"command":"test"}`),
				Timezone: "UTC",
				UserID:   user.ID,
			},
			wantErr: true,
			errMsg:  "violates not-null constraint",
		},
		{
			name: "task with empty type",
			p: domain.CreateTaskParams{
				Name:     "Test task",
				Type:     "",
				Payload:  []byte(`{"command":"test"}`),
				Timezone: "UTC",
				UserID:   user.ID,
			},
			wantErr: true,
			errMsg:  "violates not-null constraint",
		},
		{
			name: "task with nil payload",
			p: domain.CreateTaskParams{
				Name:     "Nil payload task",
				Type:     "shell",
				Payload:  nil,
				Timezone: "UTC",
				UserID:   user.ID,
			},
			wantErr: true,
			errMsg:  "violates not-null constraint",
		},
		{
			name: "task with very long name",
			p: domain.CreateTaskParams{
				Name:     string(make([]byte, 256)),
				Type:     "shell",
				Payload:  []byte(`{"command":"test"}`),
				Timezone: "UTC",
				UserID:   user.ID,
			},
			wantErr: true,
			errMsg:  "value too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.CreateTask(context.Background(), tt.p)
			
			if tt.wantErr {
				require.Error(t, err, "CreateTask() expected error")
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg, "unexpected error message")
				}
				return
			}
			
			require.NoError(t, err, "CreateTask() unexpected error")
			require.NotNil(t, got)

			trackTask(t, s, got)

			if tt.want != nil {
				tt.want(t, got)
			}
		})
	}
}

// Проверка уникальности и конкурентности
func TestStorage_CreateTask_Duplicate(t *testing.T) {
	s := newTestStorage(t)
	user := createTestUser(t, s)

	// Создаем задачу
	task1, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
		Name:     "Duplicate task",
		Type:     "shell",
		Payload:  []byte(`{"command":"test"}`),
		Timezone: "UTC",
		UserID:   user.ID,
	})
	require.NoError(t, err)
	trackTask(t, s, task1)

	// Пытаемся создать задачу с тем же именем (если есть unique constraint)
	task2, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
		Name:     "Duplicate task",
		Type:     "shell",
		Payload:  []byte(`{"command":"test2"}`),
		Timezone: "UTC",
		UserID:   user.ID,
	})
	
	if err == nil {
		// Если дубликаты разрешены
		trackTask(t, s, task2)
		assert.NotEqual(t, task1.ID, task2.ID, "tasks should have different IDs")
	} else {
		// Если есть unique constraint
		assert.Contains(t, err.Error(), "duplicate key")
	}
}

// Корректность сохранения всех полей
func TestStorage_CreateTask_FieldPersistence(t *testing.T) {
	s := newTestStorage(t)
	user := createTestUser(t, s)

	now := time.Now().UTC().Truncate(time.Second)
	schedule := "*/5 * * * *"
	createdBy := "test-user"
	targetHost := "target.example.com"

	params := domain.CreateTaskParams{
		Name:       "Full field task",
		Type:       "http_call",
		Payload:    []byte(`{"url":"https://example.com","method":"GET","headers":{"Authorization":"Bearer token"}}`),
		Schedule:   &schedule,
		Timezone:   "Europe/London",
		NextRunAt:  &now,
		MaxRetries: 7,
		Tags:       []string{"production", "monitoring", "api"},
		CreatedBy:  &createdBy,
		TargetHost: &targetHost,
		UserID:     user.ID,
	}

	got, err := s.CreateTask(context.Background(), params)
	require.NoError(t, err)
	trackTask(t, s, got)

	assert.Equal(t, params.Name, got.Name)
	assert.Equal(t, params.Type, got.Type)
	assert.JSONEq(t, string(params.Payload), string(got.Payload))
	assert.Equal(t, params.Schedule, got.Schedule)
	assert.Equal(t, params.Timezone, got.Timezone)
	assert.Equal(t, params.NextRunAt.Unix(), got.NextRunAt.Unix())
	assert.Equal(t, params.MaxRetries, got.MaxRetries)
	assert.ElementsMatch(t, params.Tags, got.Tags)
	assert.Equal(t, params.CreatedBy, got.CreatedBy)
	assert.Equal(t, params.TargetHost, got.TargetHost)
	assert.Equal(t, params.UserID, got.UserID)
	assert.Equal(t, "pending", got.Status)
	assert.Equal(t, 0, got.Retries)
	assert.NotNil(t, got.CreatedAt)
	assert.NotNil(t, got.UpdatedAt)
	assert.Nil(t, got.DeletedAt)
}

// Тест на проверку значений по умолчанию
func TestStorage_CreateTask_DefaultValues(t *testing.T) {
	s := newTestStorage(t)
	user := createTestUser(t, s)

	// Создаем задачу с минимальным набором полей
	got, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
		Name:     "Minimal task",
		Type:     "shell",
		Payload:  []byte(`{"command":"echo hello"}`),
		Timezone: "UTC",
		UserID:   user.ID,
	})
	require.NoError(t, err)
	trackTask(t, s, got)

	// Проверяем значения по умолчанию
	assert.Equal(t, "pending", got.Status)
	assert.Equal(t, 0, got.Retries)
	assert.NotNil(t, got.NextRunAt, "NextRunAt should be set by default")
	assert.Nil(t, got.LastRunAt)
	assert.Nil(t, got.StartedAt)
	assert.Nil(t, got.DeletedAt)
	assert.Nil(t, got.Schedule)
	assert.Nil(t, got.TargetHost)
	assert.Nil(t, got.CreatedBy)
	assert.Empty(t, got.Tags)
	assert.NotNil(t, got.CreatedAt)
	assert.NotNil(t, got.UpdatedAt)
}

// Тест на конкурентное создание задач
func TestStorage_CreateTask_Concurrent(t *testing.T) {
	s := newTestStorage(t)
	user := createTestUser(t, s)

	const numTasks = 10
	var wg sync.WaitGroup
	tasks := make([]*domain.Task, numTasks)
	errors := make([]error, numTasks)

	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			
			task, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
				Name:     fmt.Sprintf("Concurrent task %d", i),
				Type:     "shell",
				Payload:  []byte(fmt.Sprintf(`{"command":"echo %d"}`, i)),
				Timezone: "UTC",
				UserID:   user.ID,
			})
			
			tasks[i] = task
			errors[i] = err
		}(i)
	}

	wg.Wait()

	// Проверяем, что все задачи созданы успешно
	for i := 0; i < numTasks; i++ {
		require.NoError(t, errors[i], "task %d should be created without error", i)
		require.NotNil(t, tasks[i], "task %d should not be nil", i)
		assert.NotEmpty(t, tasks[i].ID, "task %d should have non-empty ID", i)
		trackTask(t, s, tasks[i])
	}

	// Проверяем, что все ID уникальны
	ids := make(map[string]bool)
	for _, task := range tasks {
		assert.False(t, ids[task.ID], "duplicate task ID found: %s", task.ID)
		ids[task.ID] = true
	}
}

func TestStorage_GetTaskByID(t *testing.T) {
	s := newTestStorage(t)
	user := createTestUser(t, s)

	schedule := "0 0 * * *"
	createdBy := "test-get-by-id"

	task, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
		Name:      "Get task by ID",
		Type:      "http_call",
		Payload:   []byte(`{"url":"https://example.com/api","method":"GET"}`),
		Schedule:  &schedule,
		Timezone:  "UTC",
		CreatedBy: &createdBy,
		Tags:      []string{"test", "integration"},
		UserID:    user.ID,
	})
	require.NoError(t, err, "could not create task for GetTaskByID test")
	require.NotNil(t, task, "created task should not be nil")
	trackTask(t, s, task)

	t.Run("successful retrieval", func(t *testing.T) {
		got, err := s.GetTaskByID(context.Background(), task.ID)
		require.NoError(t, err, "GetTaskByID() should not return error")
		require.NotNil(t, got, "GetTaskByID() should return task")

		assert.Equal(t, task.ID, got.ID, "ID mismatch")
		assert.Equal(t, task.Name, got.Name, "Name mismatch")
		assert.Equal(t, task.Type, got.Type, "Type mismatch")
		assert.Equal(t, "pending", got.Status, "Status should be pending")
		assert.Equal(t, task.Timezone, got.Timezone, "Timezone mismatch")
		assert.JSONEq(t, string(task.Payload), string(got.Payload), "Payload mismatch")

		require.NotNil(t, got.Schedule, "Schedule should not be nil")
		assert.Equal(t, schedule, *got.Schedule, "Schedule mismatch")

		require.NotNil(t, got.CreatedBy, "CreatedBy should not be nil")
		assert.Equal(t, createdBy, *got.CreatedBy, "CreatedBy mismatch")

		assert.Len(t, got.Tags, 2, "Tags length mismatch")
		assert.ElementsMatch(t, []string{"test", "integration"}, got.Tags, "Tags mismatch")

		assert.False(t, got.CreatedAt.IsZero(), "CreatedAt should be set")
		assert.False(t, got.UpdatedAt.IsZero(), "UpdatedAt should be set")
	})

	t.Run("task not found", func(t *testing.T) {
		nonExistentID := "00000000-0000-0000-0000-000000000000"
		got, err := s.GetTaskByID(context.Background(), nonExistentID)

		require.Error(t, err, "GetTaskByID() should return error for non-existent task")
		assert.Nil(t, got, "GetTaskByID() should return nil for non-existent task")
		assert.ErrorIs(t, err, domain.ErrTaskNotFound, "Error should be ErrTaskNotFound")
	})

	t.Run("soft deleted task not returned", func(t *testing.T) {
		deletedTask, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
			Name:     "Task to delete",
			Type:     "http_call",
			Payload:  []byte(`{}`),
			Timezone: "UTC",
			Tags:     []string{"delete-test"},
			UserID:   user.ID,
		})
		require.NoError(t, err, "could not create task for deletion test")
		require.NotNil(t, deletedTask)
		trackTask(t, s, deletedTask)

		err = s.DeleteTask(context.Background(), deletedTask.ID)
		require.NoError(t, err, "DeleteTask() should not return error")

		got, err := s.GetTaskByID(context.Background(), deletedTask.ID)
		require.Error(t, err, "GetTaskByID() should return error for deleted task")
		assert.Nil(t, got, "GetTaskByID() should return nil for deleted task")
		assert.ErrorIs(t, err, domain.ErrTaskNotFound, "Error should be ErrTaskNotFound")
	})

	t.Run("invalid UUID format", func(t *testing.T) {
		got, err := s.GetTaskByID(context.Background(), "not-a-valid-uuid")
		require.Error(t, err, "GetTaskByID() should return error for invalid UUID")
		assert.Nil(t, got, "GetTaskByID() should return nil for invalid UUID")
	})
}

func TestStorage_ListTasks(t *testing.T) {
	s := newTestStorage(t)
	user := createTestUser(t, s)

	schedule := "0 0 * * *"
	createdBy := "test-list"

	create := func(name, taskType string, tags []string) *domain.Task {
		t.Helper()

		task, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
			Name:      name,
			Type:      taskType,
			Payload:   []byte(`{"url":"https://example.com/api","method":"GET"}`),
			Schedule:  &schedule,
			Timezone:  "UTC",
			CreatedBy: &createdBy,
			Tags:      tags,
			UserID:    user.ID,
		})
		require.NoError(t, err, "could not create task (%q)", taskType)
		trackTask(t, s, task)

		return task
	}

	_ = create("Task A", "http_call", []string{"api", "test"})
	taskB := create("Task B", "email", []string{"email"})
	_ = create("Task C", "http_call", []string{"api"})

	t.Run("list all tasks", func(t *testing.T) {
		got, err := s.ListTasks(context.Background(), domain.ListTasksParams{
			Limit:  10,
			UserID: &user.ID,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(got), 3)
	})

	t.Run("filter by type", func(t *testing.T) {
		taskType := "http_call"
		got, err := s.ListTasks(context.Background(), domain.ListTasksParams{
			Type:   &taskType,
			Limit:  10,
			UserID: &user.ID,
		})
		require.NoError(t, err)
		require.Len(t, got, 2)

		for _, task := range got {
			assert.Equal(t, "http_call", task.Type)
		}
	})

	t.Run("filter by tags", func(t *testing.T) {
		got, err := s.ListTasks(context.Background(), domain.ListTasksParams{
			Tags:   []string{"email"},
			Limit:  10,
			UserID: &user.ID,
		})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, taskB.ID, got[0].ID)
	})

	t.Run("limit works", func(t *testing.T) {
		got, err := s.ListTasks(context.Background(), domain.ListTasksParams{
			Limit:  2,
			UserID: &user.ID,
		})
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("offset works", func(t *testing.T) {
		first, err := s.ListTasks(context.Background(), domain.ListTasksParams{
			Limit:  1,
			UserID: &user.ID,
		})
		require.NoError(t, err)

		second, err := s.ListTasks(context.Background(), domain.ListTasksParams{
			Limit:  1,
			Offset: 1,
			UserID: &user.ID,
		})
		require.NoError(t, err)

		require.NotEmpty(t, first)
		require.NotEmpty(t, second)
		assert.NotEqual(t, first[0].ID, second[0].ID, "offset did not change result set")
	})

	t.Run("returns empty slice when no matches", func(t *testing.T) {
		taskType := "unknown-type"
		got, err := s.ListTasks(context.Background(), domain.ListTasksParams{
			Type:   &taskType,
			Limit:  10,
			UserID: &user.ID,
		})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	// TODO: add more tests for filtering
}

func TestStorage_UpdateTask(t *testing.T) {
	s := newTestStorage(t)
	user := createTestUser(t, s)

	tests := []struct {
		name    string
		setup   func(t *testing.T, s *repository.Storage) *domain.Task
		p       domain.UpdateTaskParams
		check   func(t *testing.T, before, got *domain.Task)
		wantErr error
	}{
		{
			name: "update name and tags",
			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				schedule := "0 0 * * *"
				createdBy := "test"

				task, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
					Name:      "old_name",
					Type:      "http_call",
					Payload:   []byte(`{"url":"https://example.com/api","method":"GET"}`),
					Schedule:  &schedule,
					Timezone:  "UTC",
					CreatedBy: &createdBy,
					Tags:      []string{"a", "b"},
					UserID:    user.ID,
				})
				require.NoError(t, err)
				trackTask(t, s, task)

				return task
			},
			p: domain.UpdateTaskParams{
				Name: ptr("new_name"),
				Tags: []string{"x", "y"},
			},
			check: func(t *testing.T, before, got *domain.Task) {
				assert.Equal(t, "new_name", got.Name)
				assert.True(t, cmp.Equal(got.Tags, []string{"x", "y"}), "tags mismatch: %v", got.Tags)

				// unchanged fields
				assert.Equal(t, before.Type, got.Type, "type changed unexpectedly")
				assert.JSONEq(t, string(before.Payload), string(got.Payload), "payload changed unexpectedly")
				assert.Equal(t, "UTC", got.Timezone, "timezone changed unexpectedly")
			},
		},
		{
			name: "update only max retries does not touch others",
			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				schedule := "0 0 * * *"
				createdBy := "test"

				task, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
					Name:      "keep",
					Type:      "http_call",
					Payload:   []byte(`{"x":1}`),
					Schedule:  &schedule,
					Timezone:  "UTC",
					CreatedBy: &createdBy,
					Tags:      []string{"keep"},
					UserID:    user.ID,
				})
				require.NoError(t, err)
				trackTask(t, s, task)

				return task
			},
			p: domain.UpdateTaskParams{
				MaxRetries: ptr(99),
			},
			check: func(t *testing.T, before, got *domain.Task) {
				assert.Equal(t, 99, got.MaxRetries)
				assert.Equal(t, before.Name, got.Name, "name changed unexpectedly")
				require.NotEmpty(t, got.Tags, "tags changed unexpectedly")
				assert.Equal(t, "keep", got.Tags[0], "tags changed unexpectedly")
			},
		},
		{
			name: "task not found",
			p: domain.UpdateTaskParams{
				ID:   "00000000-0000-0000-0000-000000000000",
				Name: ptr("x"),
			},
			wantErr: domain.ErrTaskNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var before *domain.Task
			if tt.setup != nil {
				before = tt.setup(t, s)
				tt.p.ID = before.ID
			}

			got, err := s.UpdateTask(context.Background(), tt.p)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err, "UpdateTask error")

			if tt.check != nil {
				tt.check(t, before, got)
			}
		})
	}
}

func TestStorage_UpdateTaskStatus(t *testing.T) {
	s := newTestStorage(t)
	user := createTestUser(t, s)

	tests := []struct {
		name      string
		setup     func(t *testing.T, s *repository.Storage) *domain.Task
		status    string
		nextRunAt *time.Time
		check     func(t *testing.T, before, after *domain.Task)
		wantErr   bool
	}{
		{
			name: "set running status does not touch last_run_at",
			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				schedule := "0 0 * * *"
				createdBy := "test"

				task, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
					Name:      "Update task status",
					Type:      "http_call",
					Payload:   []byte(`{}`),
					Schedule:  &schedule,
					Timezone:  "UTC",
					CreatedBy: &createdBy,
					Tags:      []string{"a"},
					UserID:    user.ID,
				})
				require.NoError(t, err)
				trackTask(t, s, task)

				return task
			},
			status:    "running",
			nextRunAt: ptr(time.Now().Add(time.Hour)),
			check: func(t *testing.T, before, after *domain.Task) {
				assert.Equal(t, "running", after.Status)
				assert.Equal(t, before.LastRunAt, after.LastRunAt, "last_run_at changed unexpectedly")
			},
		},
		{
			name: "completed sets last_run_at",
			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				task, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
					Name:     "Update task status",
					Type:     "http_call",
					Payload:  []byte(`{}`),
					Tags:     []string{"a"},
					Timezone: "UTC",
					UserID:   user.ID,
				})
				require.NoError(t, err)
				trackTask(t, s, task)

				return task
			},
			status: "completed",
			check: func(t *testing.T, before, after *domain.Task) {
				assert.Equal(t, "completed", after.Status)
				require.NotNil(t, after.LastRunAt, "expected last_run_at to be set")
				assert.False(t, after.LastRunAt.Before(before.CreatedAt), "last_run_at seems invalid")
			},
		},
		{
			name: "failed sets last_run_at",
			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				task, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
					Name:     "Update task status",
					Type:     "http_call",
					Payload:  []byte(`{}`),
					Tags:     []string{"a"},
					Timezone: "UTC",
					UserID:   user.ID,
				})
				require.NoError(t, err)
				trackTask(t, s, task)

				return task
			},
			status: "failed",
			check: func(t *testing.T, before, after *domain.Task) {
				assert.Equal(t, "failed", after.Status)
				require.NotNil(t, after.LastRunAt, "expected last_run_at set")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := tt.setup(t, s)

			before, err := s.GetTaskByID(context.Background(), task.ID)
			require.NoError(t, err, "get before")

			err = s.UpdateTaskStatus(context.Background(), task.ID, tt.status, tt.nextRunAt)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err, "UpdateTaskStatus error")

			after, err := s.GetTaskByID(context.Background(), task.ID)
			require.NoError(t, err, "get after")

			if tt.check != nil {
				tt.check(t, before, after)
			}
		})
	}
}

func TestStorage_DeleteTask(t *testing.T) {
	s := newTestStorage(t)
	user := createTestUser(t, s)

	tests := []struct {
		name    string
		setup   func(t *testing.T, s *repository.Storage) *domain.Task
		wantErr bool
	}{
		{
			name: "delete existing task",
			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				task, err := s.CreateTask(context.Background(), domain.CreateTaskParams{
					Name:     "to be deleted",
					Type:     "http_call",
					Payload:  []byte(`{}`),
					Tags:     []string{"test"},
					Timezone: "UTC",
					UserID:   user.ID,
				})
				require.NoError(t, err)
				return task
			},
		},
		{
			name: "delete not existing task",
			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				return &domain.Task{ID: "00000000-0000-0000-0000-000000000000"}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := tt.setup(t, s)
			trackTask(t, s, task)

			// NOTE: DeleteTask on a non-existent ID does not itself error
			// (it's a soft delete affecting 0 rows) — the failure surfaces
			// on the follow-up GetTaskByID lookup below. This mirrors the
			// original test's behavior; don't "fix" it without checking
			// the repository implementation first.
			gotErr := s.DeleteTask(context.Background(), task.ID)
			if gotErr != nil {
				require.True(t, tt.wantErr, "DeleteTask() failed: %v", gotErr)
				return
			}

			_, err := s.GetTaskByID(context.Background(), task.ID)
			require.Error(t, err)

			if tt.wantErr {
				require.Contains(t, err.Error(), "no rows in result set")
			}
		})
	}
}
