package repository_test

import (
	"OctoQueue/internal/domain"
	"OctoQueue/internal/storage/repository"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
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
			wantErr: domain.ErrDuplicateEmail,
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

func Test_pgUserRepo_GetUser(t *testing.T) {
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

	existingUser := &domain.User{
		Email:     "getuser@test.com",
		Password:  "hashedpassword",
		Role:      domain.RoleUser,
		CreatedAt: time.Now(),
	}
	if err := userRepo.CreateUser(ctx, existingUser); err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	created = append(created, existingUser)

	tests := []struct {
		name    string
		id      string
		want    func(t *testing.T, got *domain.User)
		wantErr error
	}{
		{
			name: "successful fetch",
			id:   existingUser.ID.String(),
			want: func(t *testing.T, got *domain.User) {
				t.Helper()
				assert.Equal(t, existingUser.ID, got.ID)
				assert.Equal(t, existingUser.Email, got.Email)
				assert.Equal(t, existingUser.Password, got.Password)
				assert.Equal(t, existingUser.Role, got.Role)
				assert.False(t, got.CreatedAt.IsZero(), "expected non-zero CreatedAt")
			},
			wantErr: nil,
		},
		{
			name:    "user not found",
			id:      uuid.New().String(),
			wantErr: domain.ErrUserNotFound,
		},
		{
			name:    "invalid uuid format",
			id:      "not-a-uuid",
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := userRepo.GetUser(t.Context(), tt.id)

			if tt.name == "invalid uuid format" {
				assert.Error(t, gotErr)
				assert.False(t, errors.Is(gotErr, domain.ErrUserNotFound),
					"invalid uuid should not be reported as not found")
				assert.Nil(t, got)
				return
			}

			if tt.wantErr != nil {
				if !errors.Is(gotErr, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, gotErr)
				}
				assert.Nil(t, got)
				return
			}

			if gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			}

			if tt.want != nil {
				tt.want(t, got)
			}
		})
	}
}

func Test_pgUserRepo_GetByEmail(t *testing.T) {
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

	existingUser := &domain.User{
		Email:     "getbyemail@test.com",
		Password:  "hashedpassword",
		Role:      domain.RoleUser,
		CreatedAt: time.Now(),
	}
	if err := userRepo.CreateUser(ctx, existingUser); err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	created = append(created, existingUser)

	tests := []struct {
		name    string
		email   string
		want    func(t *testing.T, got *domain.User)
		wantErr error
	}{
		{
			name:  "successful fetch",
			email: existingUser.Email,
			want: func(t *testing.T, got *domain.User) {
				t.Helper()
				assert.Equal(t, existingUser.ID, got.ID)
				assert.Equal(t, existingUser.Email, got.Email)
				assert.Equal(t, existingUser.Password, got.Password)
				assert.Equal(t, existingUser.Role, got.Role)
				assert.False(t, got.CreatedAt.IsZero(), "expected non-zero CreatedAt")
			},
			wantErr: nil,
		},
		{
			name:    "user not found",
			email:   "nonexistent@test.com",
			wantErr: domain.ErrUserNotFound,
		},
		{
			name:    "empty email",
			email:   "",
			wantErr: domain.ErrUserNotFound,
		},
		{
			name:    "case sensitivity check",
			email:   strings.ToUpper(existingUser.Email),
			wantErr: domain.ErrUserNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := userRepo.GetByEmail(t.Context(), tt.email)

			if tt.wantErr != nil {
				if !errors.Is(gotErr, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, gotErr)
				}
				assert.Nil(t, got)
				return
			}

			if gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			}

			if tt.want != nil {
				tt.want(t, got)
			}
		})
	}
}
