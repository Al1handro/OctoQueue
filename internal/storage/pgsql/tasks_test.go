package pgsql_test

import (
	"OctoQueue/internal/storage"
	"OctoQueue/internal/storage/pgsql"
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

const testDSN = "postgres://postgres:password@localhost:5432?sslmode=disable"

func TestStorage_CreateTask(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	s, err := pgsql.NewStorage(context.Background(), testDSN, log)
	if err != nil {
		t.Fatalf("could not connect to test db: %v", err)
	}

	schedule := "0 0 * * *"
	createdBy := "test-user"
	targetHost := "example.com"

	tests := []struct {
		name    string
		p       storage.CreateTaskParams
		want    func(t *testing.T, got *storage.Task)
		wantErr bool
	}{
		{
			name: "successful creation of http_call task",
			p: storage.CreateTaskParams{
				Name:       "Daily backup",
				Type:       "http_call",
				Payload:    []byte(`{"url":"https://example.com/api","method":"POST"}`),
				Schedule:   &schedule,
				Timezone:   "UTC",
				MaxRetries: 3,
				Tags:       []string{"production", "critical"},
				CreatedBy:  &createdBy,
				TargetHost: &targetHost,
			},
			want: func(t *testing.T, got *storage.Task) {
				t.Helper()
				if got.ID == "" {
					t.Error("expected non-empty ID")
				}
				if got.Name != "Daily backup" {
					t.Errorf("Name = %q, want %q", got.Name, "Daily backup")
				}
				if got.Type != "http_call" {
					t.Errorf("Type = %q, want %q", got.Type, "http_call")
				}
				if got.Status != "pending" {
					t.Errorf("Status = %q, want %q", got.Status, "pending")
				}
				if got.MaxRetries != 3 {
					t.Errorf("MaxRetries = %d, want 3", got.MaxRetries)
				}
				if len(got.Tags) != 2 {
					t.Errorf("Tags len = %d, want 2", len(got.Tags))
				}
				if got.CreatedAt.IsZero() {
					t.Error("expected non-zero CreatedAt")
				}
			},
		},
		{
			name: "one-time task with run_at",
			p: storage.CreateTaskParams{
				Name:      "Deploy v2",
				Type:      "shell",
				Payload:   []byte(`{"command":"kubectl apply -f deploy.yaml"}`),
				Tags: []string{"deployment", "test"},
				Timezone:  "UTC",
				NextRunAt: func() *time.Time { t := time.Now().Add(5 * time.Minute); return &t }(),
			},
			want: func(t *testing.T, got *storage.Task) {
				t.Helper()
				if got.NextRunAt == nil {
					t.Error("expected non-nil NextRunAt")
				}
				if got.Schedule != nil {
					t.Error("expected nil Schedule for one-time task")
				}
			},
		},
		{
			name: "An empty name is an error.",
			p: storage.CreateTaskParams{
				Name:    "",
				Type:    "http_call",
				Payload: []byte(`{}`),
			},
			wantErr: true,
		},
		{
			name: "An invalid type is an error.",
			p: storage.CreateTaskParams{
				Name:    "Bad task",
				Type:    "unknown_type",
				Payload: []byte(`{}`),
			},
			wantErr: true,
		},
	}


	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				_, _ = s.Pool().Exec(context.Background(), "DELETE FROM tasks WHERE created_by = 'test-user' OR name LIKE '%test%'")
			})

			got, err := s.CreateTask(context.Background(), tt.p)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("CreateTask() unexpected error: %v", err)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("CreateTask() succeeded unexpectedly")
			}

			if tt.want != nil {
				tt.want(t, got)
			}
		})
	}
}

func TestStorage_GetTaskByID(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	s, err := pgsql.NewStorage(context.Background(), testDSN, log)
	if err != nil {
		t.Fatalf("could not connect to test db: %v", err)
	}

	schedule := "0 0 * * *"
	createdBy := "test-get-by-id"

	created, err := s.CreateTask(context.Background(), storage.CreateTaskParams{
		Name:      "Get task by ID",
		Type:      "http_call",
		Payload:   []byte(`{"url":"https://example.com/api","method":"GET"}`),
		Schedule:  &schedule,
		Timezone:  "UTC",
		CreatedBy: &createdBy,
		Tags:       []string{"test"},
	})
	if err != nil {
		t.Fatalf("could not create task for GetTaskByID test: %v", err)
	}

	t.Cleanup(func() {
		_, _ = s.Pool().Exec(context.Background(), "DELETE FROM tasks WHERE id = $1", created.ID)
	})

	got, err := s.GetTaskByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetTaskByID() failed: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
	if got.Name != created.Name {
		t.Errorf("Name = %q, want %q", got.Name, created.Name)
	}
	if got.Type != created.Type {
		t.Errorf("Type = %q, want %q", got.Type, created.Type)
	}
	if got.Schedule == nil || *got.Schedule != schedule {
		t.Errorf("Schedule = %v, want %v", got.Schedule, schedule)
	}
	if got.CreatedBy == nil || *got.CreatedBy != createdBy {
		t.Errorf("CreatedBy = %v, want %v", got.CreatedBy, createdBy)
	}
	if got.Timezone != created.Timezone {
		t.Errorf("Timezone = %q, want %q", got.Timezone, created.Timezone)
	}
	if got.Status != "pending" {
		t.Errorf("Status = %q, want %q", got.Status, "pending")
	}
	if string(got.Payload) != string(created.Payload) {
		t.Errorf("Payload = %q, want %q", string(got.Payload), string(created.Payload))
	}
	if string(got.Tags[0]) != string(created.Tags[0]) {
		t.Errorf("Tags[0] = %q, want %q", string(got.Tags[0]), string(created.Tags[0]))
	}
}

func TestStorage_ListTasks(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	s, err := pgsql.NewStorage(context.Background(), testDSN, log)
	if err != nil {
		t.Fatalf("could not connect to test db: %v", err)
	}


	schedule := "0 0 * * *"
	createdBy := "test-list"

	created := make([]*storage.Task, 0)

	create := func(name, taskType string, tags []string) *storage.Task {
		t.Helper()

		task, err := s.CreateTask(context.Background(), storage.CreateTaskParams{
			Name:      name,
			Type:      taskType,
			Payload:   []byte(`{"url":"https://example.com/api","method":"GET"}`),
			Schedule:  &schedule,
			Timezone:  "UTC",
			CreatedBy: &createdBy,
			Tags:      tags,
		})
		if err != nil {
			t.Fatalf("could not create task: %v", err)
		}

		created = append(created, task)

		return task
	}

	taskA := create("Task A", "http_call", []string{"api", "test"})
	taskB := create("Task B", "email", []string{"email"})
	taskC := create("Task C", "http_call", []string{"api"})

	t.Run("list all tasks", func(t *testing.T) {
		got, err := s.ListTasks(context.Background(), storage.ListTasksParams{
			Limit: 10,
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

		got, err := s.ListTasks(context.Background(), storage.ListTasksParams{
			Type:  &taskType,
			Limit: 10,
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
		got, err := s.ListTasks(context.Background(), storage.ListTasksParams{
			Tags:  []string{"email"},
			Limit: 10,
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
		got, err := s.ListTasks(context.Background(), storage.ListTasksParams{
			Limit: 2,
		})
		if err != nil {
			t.Fatalf("ListTasks() error = %v", err)
		}

		if len(got) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(got))
		}
	})

	t.Run("offset works", func(t *testing.T) {
		first, err := s.ListTasks(context.Background(), storage.ListTasksParams{
			Limit: 1,
		})
		if err != nil {
			t.Fatalf("ListTasks() error = %v", err)
		}

		second, err := s.ListTasks(context.Background(), storage.ListTasksParams{
			Limit:  1,
			Offset: 1,
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

		got, err := s.ListTasks(context.Background(), storage.ListTasksParams{
			Type:  &taskType,
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("ListTasks() error = %v", err)
		}

		if len(got) != 0 {
			t.Fatalf("expected empty result, got %d tasks", len(got))
		}
	})

	t.Cleanup(func() {
		for _, task := range created {
			_, _ = s.Pool().Exec(
				context.Background(),
				"DELETE FROM tasks WHERE id = $1",
				task.ID,
			)
		}
	})

	_ = taskA
	_ = taskC

	// TODO: add more tests for filtering
}
