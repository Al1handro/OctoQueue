package repository_test

import (
	"OctoQueue/internal/domain"
	"OctoQueue/internal/storage/repository"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDSN = "postgres://postgres:password@localhost:5432/app?sslmode=disable"

func ptr[T any](v T) *T {
	return &v
}

func testUserRepository(t *testing.T, storage *repository.Storage) *domain.User {
	domainUser := &domain.User{
		Email:     fmt.Sprintf("test%d@example.com", time.Now().Unix()),
		Password:  "hashedpassword",
		Role:      domain.RoleUser,
		CreatedAt: time.Now(),
	}

	s := repository.NewUserRepository(storage.Pool())
	err := s.CreateUser(context.Background(), domainUser)
	if err != nil {
		t.Fatalf("could not create user: %v", err)
	}
	return domainUser
}

func TestStorage_CreateTask(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	s, err := repository.NewStorage(ctx, testDSN, log)
	if err != nil {
		t.Fatalf("could not connect to test db: %v", err)
	}

	t.Cleanup(func() {
		s.Close()
	})

	user := testUserRepository(t, s)

	created := make([]*domain.Task, 0)

	t.Cleanup(func() {
		for _, task := range created {
			_, err := s.Pool().Exec(ctx, "DELETE FROM tasks WHERE id = $1", task.ID)
			if err != nil {
				t.Logf("warning: failed to delete task %s: %v", task.ID, err)
			}
		}
		_, err := s.Pool().Exec(ctx, "DELETE FROM users WHERE id = $1", user.ID)
		if err != nil {
			t.Logf("warning: failed to delete user %s: %v", user.ID, err)
		}
	})

	schedule := "0 0 * * *"
	createdBy := "test-user"
	targetHost := "example.com"

	tests := []struct {
		name    string
		p       domain.CreateTaskParams
		want    func(t *testing.T, got *domain.Task)
		wantErr error
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
				NextRunAt: func() *time.Time { t := time.Now().Add(5 * time.Minute); return &t }(),
				UserID:    user.ID,
			},
			want: func(t *testing.T, got *domain.Task) {
				t.Helper()
				assert.NotNil(t, got.NextRunAt, "expected non-nil NextRunAt")
				assert.Nil(t, got.Schedule, "expected nil Schedule for one-time task")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.CreateTask(t.Context(), tt.p)

			require.NoError(t, err, "CreateTask() unexpected error")
			require.NotNil(t, got)

			if tt.want != nil {
				tt.want(t, got)
			}

			created = append(created, got)
		})
	}
}

func TestStorage_GetTaskByID(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	s, err := repository.NewStorage(ctx, testDSN, log)
	require.NoError(t, err, "could not connect to test db")

	t.Cleanup(func() {
		s.Close()
	})

	user := testUserRepository(t, s)

	schedule := "0 0 * * *"
	createdBy := "test-get-by-id"

	task, err := s.CreateTask(ctx, domain.CreateTaskParams{
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

	t.Cleanup(func() {
		_, err := s.Pool().Exec(ctx, "DELETE FROM tasks WHERE id = $1", task.ID)
		if err != nil {
			t.Logf("warning: failed to delete task %s: %v", task.ID, err)
		}
		_, err = s.Pool().Exec(ctx, "DELETE FROM users WHERE id = $1", user.ID)
		if err != nil {
			t.Logf("warning: failed to delete user %s: %v", user.ID, err)
		}
	})

	t.Run("successful retrieval", func(t *testing.T) {
		got, err := s.GetTaskByID(ctx, task.ID)
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
		got, err := s.GetTaskByID(ctx, nonExistentID)

		require.Error(t, err, "GetTaskByID() should return error for non-existent task")
		assert.Nil(t, got, "GetTaskByID() should return nil for non-existent task")
		assert.ErrorIs(t, err, domain.ErrTaskNotFound, "Error should be ErrTaskNotFound")
	})

	t.Run("soft deleted task not returned", func(t *testing.T) {
		deletedTask, err := s.CreateTask(ctx, domain.CreateTaskParams{
			Name:     "Task to delete",
			Type:     "http_call",
			Payload:  []byte(`{}`),
			Timezone: "UTC",
			Tags:     []string{"delete-test"},
			UserID:   user.ID,
		})
		require.NoError(t, err, "could not create task for deletion test")
		require.NotNil(t, deletedTask)

		err = s.DeleteTask(ctx, deletedTask.ID)
		require.NoError(t, err, "DeleteTask() should not return error")

		got, err := s.GetTaskByID(ctx, deletedTask.ID)
		require.Error(t, err, "GetTaskByID() should return error for deleted task")
		assert.Nil(t, got, "GetTaskByID() should return nil for deleted task")
		assert.ErrorIs(t, err, domain.ErrTaskNotFound, "Error should be ErrTaskNotFound")

		_, _ = s.Pool().Exec(ctx, "DELETE FROM tasks WHERE id = $1", deletedTask.ID)
	})

	t.Run("invalid UUID format", func(t *testing.T) {
		got, err := s.GetTaskByID(ctx, "not-a-valid-uuid")
		require.Error(t, err, "GetTaskByID() should return error for invalid UUID")
		assert.Nil(t, got, "GetTaskByID() should return nil for invalid UUID")
	})
}

func TestStorage_ListTasks(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	s, err := repository.NewStorage(ctx, testDSN, log)
	if err != nil {
		t.Fatalf("could not connect to test db: %v", err)
	}
	user := testUserRepository(t, s)

	schedule := "0 0 * * *"
	createdBy := "test-list"

	created := make([]*domain.Task, 0)

	t.Cleanup(func() {
		for _, task := range created {
			_, err := s.Pool().Exec(ctx, "DELETE FROM tasks WHERE id = $1", task.ID)
			if err != nil {
				t.Logf("warning: failed to delete task %s: %v", task.ID, err)
			}
		}
		_, err := s.Pool().Exec(ctx, "DELETE FROM users WHERE id = $1", user.ID)
		if err != nil {
			t.Logf("warning: failed to delete user %s: %v", user.ID, err)
		}
	})

	create := func(name, taskType string, tags []string) *domain.Task {
		t.Helper()

		task, err := s.CreateTask(ctx, domain.CreateTaskParams{
			Name:      name,
			Type:      taskType,
			Payload:   []byte(`{"url":"https://example.com/api","method":"GET"}`),
			Schedule:  &schedule,
			Timezone:  "UTC",
			CreatedBy: &createdBy,
			Tags:      tags,
			UserID:    user.ID,
		})
		if err != nil {
			t.Fatalf("could not create task (%q): %+v", taskType, err)
		}

		created = append(created, task)

		return task
	}

	taskA := create("Task A", "http_call", []string{"api", "test"})
	taskB := create("Task B", "email", []string{"email"})
	taskC := create("Task C", "http_call", []string{"api"})

	t.Run("list all tasks", func(t *testing.T) {
		got, err := s.ListTasks(ctx, domain.ListTasksParams{
			Limit:  10,
			UserID: &user.ID,
		})
		if err != nil {
			t.Fatalf("ListTasks() error = %v", err)
		}

		if len(got) < 3 {
			t.Fatalf("expected at least 3 tasks, got %d", len(got))
		}
	})

	t.Run("filter by type", func(t *testing.T) {
		taskType := "http_call"

		got, err := s.ListTasks(ctx, domain.ListTasksParams{
			Type:   &taskType,
			Limit:  10,
			UserID: &user.ID,
		})

		if err != nil {
			t.Fatalf("ListTasks() error = %v", err)
		}

		if len(got) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(got))
		}

		for _, task := range got {
			if task.Type != "http_call" {
				t.Errorf("unexpected task type: %s", task.Type)
			}
		}
	})

	t.Run("filter by tags", func(t *testing.T) {
		got, err := s.ListTasks(ctx, domain.ListTasksParams{
			Tags:   []string{"email"},
			Limit:  10,
			UserID: &user.ID,
		})
		if err != nil {
			t.Fatalf("ListTasks() error = %v", err)
		}

		if len(got) != 1 {
			t.Fatalf("expected 1 task, got %d", len(got))
		}

		if got[0].ID != taskB.ID {
			t.Fatalf("unexpected task returned")
		}
	})

	t.Run("limit works", func(t *testing.T) {
		got, err := s.ListTasks(ctx, domain.ListTasksParams{
			Limit:  2,
			UserID: &user.ID,
		})
		if err != nil {
			t.Fatalf("ListTasks() error = %v", err)
		}

		if len(got) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(got))
		}
	})

	t.Run("offset works", func(t *testing.T) {
		first, err := s.ListTasks(ctx, domain.ListTasksParams{
			Limit:  1,
			UserID: &user.ID,
		})
		if err != nil {
			t.Fatalf("ListTasks() error = %v", err)
		}

		second, err := s.ListTasks(ctx, domain.ListTasksParams{
			Limit:  1,
			Offset: 1,
			UserID: &user.ID,
		})
		if err != nil {
			t.Fatalf("ListTasks() error = %v", err)
		}

		if len(first) == 0 || len(second) == 0 {
			t.Fatal("expected non-empty results")
		}

		if first[0].ID == second[0].ID {
			t.Fatal("offset did not change result set")
		}
	})

	t.Run("returns empty slice when no matches", func(t *testing.T) {
		taskType := "unknown-type"

		got, err := s.ListTasks(ctx, domain.ListTasksParams{
			Type:   &taskType,
			Limit:  10,
			UserID: &user.ID,
		})
		if err != nil {
			t.Fatalf("ListTasks() error = %v", err)
		}

		if len(got) != 0 {
			t.Fatalf("expected empty result, got %d tasks", len(got))
		}
	})

	_ = taskA
	_ = taskC

	// TODO: add more tests for filtering

}

func TestStorage_UpdateTask(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	s, err := repository.NewStorage(ctx, testDSN, log)
	if err != nil {
		t.Fatalf("could not connect to test db: %v", err)
	}

	created := make([]*domain.Task, 0)

	t.Cleanup(func() {
		s.Close()
	})

	user := testUserRepository(t, s)

	tests := []struct {
		name string

		setup func(t *testing.T, s *repository.Storage) *domain.Task
		p     domain.UpdateTaskParams

		check   func(t *testing.T, before, got *domain.Task)
		wantErr error
	}{
		{
			name: "update name and tags",

			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				schedule := "0 0 * * *"
				createdBy := "test"

				create := func(name, taskType string, tags []string) *domain.Task {
					t.Helper()

					task, err := s.CreateTask(t.Context(), domain.CreateTaskParams{
						Name:      name,
						Type:      taskType,
						Payload:   []byte(`{"url":"https://example.com/api","method":"GET"}`),
						Schedule:  &schedule,
						Timezone:  "UTC",
						CreatedBy: &createdBy,
						Tags:      tags,
						UserID:    user.ID,
					})
					if err != nil {
						t.Fatalf("could not create task (%q): %+v", taskType, err)
					}

					created = append(created, task)
					return task
				}

				return create("old_name", "http_call", []string{"a", "b"})
			},

			p: domain.UpdateTaskParams{
				Name: ptr("new_name"),
				Tags: []string{"x", "y"},
			},

			check: func(t *testing.T, before, got *domain.Task) {
				if got.Name != "new_name" {
					t.Errorf("name = %s, want %s", got.Name, "new_name")
				}

				if !cmp.Equal(got.Tags, []string{"x", "y"}) {
					t.Errorf("tags mismatch: %v", got.Tags)
				}

				// unchanged fields
				if got.Type != before.Type {
					t.Errorf("type changed: %s", got.Type)
				}

				if string(got.Payload) != string(before.Payload) {
					t.Errorf("payload changed unexpectedly")
				}

				if got.Timezone != "UTC" {
					t.Errorf("timezone changed: %s", got.Timezone)
				}
			},
		},

		{
			name: "update only max retries does not touch others",

			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				schedule := "0 0 * * *"
				createdBy := "test"

				task, err := s.CreateTask(t.Context(), domain.CreateTaskParams{
					Name:      "keep",
					Type:      "http_call",
					Payload:   []byte(`{"x":1}`),
					Schedule:  &schedule,
					Timezone:  "UTC",
					CreatedBy: &createdBy,
					Tags:      []string{"keep"},
					UserID:    user.ID,
				})
				if err != nil {
					t.Fatalf("create task: %v", err)
				}

				created = append(created, task)
				return task
			},

			p: domain.UpdateTaskParams{
				MaxRetries: ptr(99),
			},

			check: func(t *testing.T, before, got *domain.Task) {
				if got.MaxRetries != 99 {
					t.Errorf("max_retries = %d", got.MaxRetries)
				}

				if got.Name != before.Name {
					t.Errorf("name changed unexpectedly")
				}

				if len(got.Tags) == 0 || got.Tags[0] != "keep" {
					t.Errorf("tags changed unexpectedly")
				}
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

			got, err := s.UpdateTask(ctx, tt.p)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("UpdateTask error: %v", err)
			}

			if tt.check != nil {
				tt.check(t, before, got)
			}
		})
	}

	t.Cleanup(func() {
		for _, task := range created {
			_, err := s.Pool().Exec(ctx, "DELETE FROM tasks WHERE id = $1", task.ID)
			if err != nil {
				t.Logf("warning: failed to delete task %s: %v", task.ID, err)
			}
		}
		_, err = s.Pool().Exec(ctx, "DELETE FROM users WHERE id = $1", user.ID)
		if err != nil {
			t.Logf("warning: failed to delete user %s: %v", user.ID, err)
		}
	})
}

func TestStorage_UpdateTaskStatus(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	s, err := repository.NewStorage(context.Background(), testDSN, log)
	if err != nil {
		t.Fatalf("could not connect to test db: %v", err)
	}
	user := testUserRepository(t, s)
	created := make([]*domain.Task, 0)

	tests := []struct {
		name string
		dsn  string

		setup func(t *testing.T, s *repository.Storage) (task *domain.Task)

		status    string
		nextRunAt *time.Time

		check func(t *testing.T, before *domain.Task, after *domain.Task)

		wantErr bool
	}{
		{
			name: "set running status does not touch last_run_at",
			dsn:  testDSN,

			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				schedule := "0 0 * * *"
				createdBy := "test"

				task, err := s.CreateTask(t.Context(), domain.CreateTaskParams{
					Name:      "Update task status",
					Type:      "http_call",
					Payload:   []byte(`{}`),
					Schedule:  &schedule,
					Timezone:  "UTC",
					CreatedBy: &createdBy,
					Tags:      []string{"a"},
					UserID:    user.ID,
				})
				if err != nil {
					t.Fatalf("create: %v", err)
				}
				return task
			},

			status:    "running",
			nextRunAt: ptrTime(time.Now().Add(time.Hour)),

			check: func(t *testing.T, before, after *domain.Task) {
				if after.Status != "running" {
					t.Errorf("status = %s", after.Status)
				}
				if after.LastRunAt != before.LastRunAt {
					t.Errorf("last_run_at changed unexpectedly")
				}
			},
		},

		{
			name: "completed sets last_run_at",
			dsn:  testDSN,

			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				task, err := s.CreateTask(t.Context(), domain.CreateTaskParams{
					Name:     "Update task status",
					Type:     "http_call",
					Payload:  []byte(`{}`),
					Tags:     []string{"a"},
					Timezone: "UTC",
					UserID:   user.ID,
				})
				if err != nil {
					t.Fatalf("create: %v", err)
				}
				return task
			},

			status:    "completed",
			nextRunAt: nil,

			check: func(t *testing.T, before, after *domain.Task) {
				if after.Status != "completed" {
					t.Errorf("status = %s", after.Status)
				}

				if after.LastRunAt == nil {
					t.Fatal("expected last_run_at to be set")
				}

				if after.LastRunAt.Before(before.CreatedAt) {
					t.Errorf("last_run_at seems invalid")
				}
			},
		},

		{
			name: "failed sets last_run_at",
			dsn:  testDSN,

			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				task, err := s.CreateTask(t.Context(), domain.CreateTaskParams{
					Name:     "Update task status",
					Type:     "http_call",
					Payload:  []byte(`{}`),
					Tags:     []string{"a"},
					Timezone: "UTC",
					UserID:   user.ID,
				})
				if err != nil {
					t.Fatalf("create: %v", err)
				}
				return task
			},

			status:    "failed",
			nextRunAt: nil,

			check: func(t *testing.T, before, after *domain.Task) {
				if after.Status != "failed" {
					t.Errorf("status = %s", after.Status)
				}
				if after.LastRunAt == nil {
					t.Fatal("expected last_run_at set")
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			s, err := repository.NewStorage(t.Context(), tt.dsn, log)
			if err != nil {
				t.Fatalf("new storage: %v", err)
			}

			var task *domain.Task
			task = tt.setup(t, s)

			// fetch before state
			before, err := s.GetTaskByID(t.Context(), task.ID)
			if err != nil {
				t.Fatalf("get before: %v", err)
			}

			err = s.UpdateTaskStatus(t.Context(), task.ID, tt.status, tt.nextRunAt)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("UpdateTaskStatus error: %v", err)
			}

			after, err := s.GetTaskByID(t.Context(), task.ID)
			if err != nil {
				t.Fatalf("get after: %v", err)
			}

			if tt.check != nil {
				tt.check(t, before, after)
			}
		})
	}

	t.Cleanup(func() {
		for _, task := range created {
			_, err := s.Pool().Exec(ctx, "DELETE FROM tasks WHERE id = $1", task.ID)
			if err != nil {
				t.Logf("warning: failed to delete task %s: %v", task.ID, err)
			}
		}
	})

	_, _ = s.Pool().Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func TestStorage_DeleteTask(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	s, err := repository.NewStorage(ctx, testDSN, log)
	if err != nil {
		t.Fatalf("could not connect to test db: %v", err)
	}
	user := testUserRepository(t, s)

	var created []*domain.Task

	tests := []struct {
		name    string
		dsn     string
		log     *slog.Logger
		setup   func(*testing.T, *repository.Storage) *domain.Task
		wantErr bool
	}{
		{
			name: "delete existing task",
			dsn:  testDSN,
			log:  log,
			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				task, err := s.CreateTask(t.Context(), domain.CreateTaskParams{
					Name:     "to be deleted",
					Type:     "http_call",
					Payload:  []byte(`{}`),
					Tags:     []string{"test"},
					Timezone: "UTC",
					UserID:   user.ID,
				})
				if err != nil {
					t.Fatalf("create task: %v", err)
				}
				return task
			},
			wantErr: false,
		},
		{
			name: "delete not existing task",
			dsn:  testDSN,
			log:  log,
			setup: func(t *testing.T, s *repository.Storage) *domain.Task {
				return &domain.Task{ID: "00000000-0000-0000-0000-000000000000"}
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := tt.setup(t, s)
			created = append(created, task)

			gotErr := s.DeleteTask(t.Context(), task.ID)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("DeleteTask() failed: %v", gotErr)
				}
				return
			}

			_, err = s.GetTaskByID(t.Context(), task.ID)
			require.Error(t, err)

			if tt.wantErr {
				require.Contains(t, err.Error(), "no rows in result set")
			}
		})
	}

	t.Cleanup(func() {
		for _, task := range created {
			_, err := s.Pool().Exec(ctx, "DELETE FROM tasks WHERE id = $1", task.ID)
			if err != nil {
				t.Logf("warning: failed to delete task %s: %v", task.ID, err)
			}
		}
		_, err = s.Pool().Exec(ctx, "DELETE FROM users WHERE id = $1", user.ID)
		if err != nil {
			t.Logf("warning: failed to delete user %s: %v", user.ID, err)
		}
	})
}
