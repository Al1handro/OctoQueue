package repository_test

import (
	"OctoQueue/internal/domain"
	"OctoQueue/internal/storage/repository"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func Test_pgUserRepo_CreateUser(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	st, err := repository.NewStorage(ctx, testDSN, log)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	userRepo := repository.NewUserRepository(st.Pool())

	created := make([]*domain.User, 0)

	t.Cleanup(func() {
		for _, user := range created {
			if user.ID == uuid.Nil {
				continue
			}
			_, err := st.Pool().Exec(ctx, "DELETE FROM users WHERE id = $1", user.ID)
			if err != nil {
				t.Errorf("failed to clean up user %s: %v", user.ID, err)
			}
		}
	})

	duplicateEmail := "duplicate@test.com"
	seedUser := &domain.User{
		Email:     duplicateEmail,
		Password:  "hashedpassword",
		Role:      domain.RoleUser,
		CreatedAt: time.Now(),
	}
	if err := userRepo.CreateUser(context.Background(), seedUser); err != nil {
		t.Fatalf("failed to seed user for duplicate test: %v", err)
	}
	created = append(created, seedUser)

	tests := []struct {
		name    string
		p       *domain.User
		want    func(t *testing.T, got *domain.User)
		wantErr error
	}{
		{
			name: "successful creation",
			p: &domain.User{
				Email:     "example@test.com",
				Password:  "hashedpassword",
				Role:      domain.RoleUser,
				CreatedAt: time.Now(),
			},
			want: func(t *testing.T, got *domain.User) {
				t.Helper()
				assert.NotEmpty(t, got.ID, "expected non-empty ID")
				assert.False(t, got.CreatedAt.IsZero(), "expected non-zero CreatedAt")
			},
			wantErr: nil,
		},
		{
			name: "duplicate email",
			p: &domain.User{
				Email:     duplicateEmail,
				Password:  "hashedpassword",
				Role:      domain.RoleUser,
				CreatedAt: time.Now(),
			},
			wantErr: repository.ErrDuplicateEmail,
		},
		{
			name: "admin role is stored correctly",
			p: &domain.User{
				Email:     "admin@test.com",
				Password:  "hashedpassword",
				Role:      domain.RoleAdmin,
				CreatedAt: time.Now(),
			},
			want: func(t *testing.T, got *domain.User) {
				t.Helper()
				assert.NotEmpty(t, got.ID, "expected non-empty ID")
				assert.Equal(t, domain.RoleAdmin, got.Role, "expected role to be preserved")
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := userRepo.CreateUser(t.Context(), tt.p)
			created = append(created, tt.p)

			if tt.wantErr != nil {
				if !errors.Is(gotErr, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, gotErr)
				}
				return
			}

			if gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			}

			if tt.want != nil {
				tt.want(t, tt.p)
			}
		})
	}
}

// func Test_pgUserRepo_GetUser(t *testing.T) {
// 	tests := []struct {
// 		name string // description of this test case
// 		// Named input parameters for target function.
// 		id      string
// 		want    *domain.User
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			// TODO: construct the receiver type.
// 			var s pgUserRepo
// 			got, gotErr := s.GetUser(context.Background(), tt.id)
// 			if gotErr != nil {
// 				if !tt.wantErr {
// 					t.Errorf("GetUser() failed: %v", gotErr)
// 				}
// 				return
// 			}
// 			if tt.wantErr {
// 				t.Fatal("GetUser() succeeded unexpectedly")
// 			}
// 			// TODO: update the condition below to compare got with tt.want.
// 			if true {
// 				t.Errorf("GetUser() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func Test_pgUserRepo_GetByEmail(t *testing.T) {
// 	tests := []struct {
// 		name string // description of this test case
// 		// Named input parameters for target function.
// 		email   string
// 		want    *domain.User
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			// TODO: construct the receiver type.
// 			var s pgUserRepo
// 			got, gotErr := s.GetByEmail(context.Background(), tt.email)
// 			if gotErr != nil {
// 				if !tt.wantErr {
// 					t.Errorf("GetByEmail() failed: %v", gotErr)
// 				}
// 				return
// 			}
// 			if tt.wantErr {
// 				t.Fatal("GetByEmail() succeeded unexpectedly")
// 			}
// 			// TODO: update the condition below to compare got with tt.want.
// 			if true {
// 				t.Errorf("GetByEmail() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }
