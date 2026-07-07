package handlers_test

import (
	"OctoQueue/internal/domain"
	"OctoQueue/internal/http-server/handlers"
	"net/http"
	"net/http/httptest"
	"strings"

	"OctoQueue/internal/storage/repository/mocks"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCreateTask(t *testing.T) {
	tests := []struct {
		name    string
		p       domain.CreateTaskParams
		st      mocks.TaskRepository
		want    http.HandlerFunc
		wantErr error
	}{
		{
			name: "An invalid type is an error.",
			p: domain.CreateTaskParams{
				Name:    "Bad task",
				Type:    "unknown_type",
				Tags:    []string{"test"},
				Payload: []byte(`{}`),
				UserID:  user.ID,
			},
			wantErr: domain.ErrNotFound,
		},
	} // TODO: add more test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewTaskRepository(t)

			repo.
				On(
					"CreateTask",
					tt.log,
					mock.Anything,
				).
				Return(
					&domain.Task{
						ID: mock.Anything,
					},
					nil,
				)

			handler := handlers.CreateTask(
				slog.Default(),
				repo,
			)

			body := `{
				"Name":       "Daily backup",
				"Type":       "http_call",
				"Payload":    {"url":"https://example.com/api","method":"POST"},
				"Schedule":   "0 0 * * *",
				"Timezone":   "UTC",
				"MaxRetries": 3,
				"Tags":       "test",
				"CreatedBy":  "test_user",
				"TargetHost": "example.com",
			}`

			req := httptest.NewRequest(
				http.MethodPost,
				"/tasks",
				strings.NewReader(body),
			)

			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req) 

			// Check the status code

			assert.Equal(
				t,
				http.StatusCreated,
				rr.Code,
			)
		})
	}
}
