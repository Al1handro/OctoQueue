package service_test

import (
	"OctoQueue/internal/domain"
	"OctoQueue/internal/service"
	"OctoQueue/internal/storage/repository/mocks"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
)

const testJWTSecret = "test-secret"

func TestAuth_Register(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name      string
		email     string
		password  string
		role      domain.Role
		repoErr   error
		wantErr   error
		wantUser  bool
		checkHash bool
	}{
		{
			name:      "successful registration",
			email:     "user@example.com",
			password:  "secret123",
			role:      domain.RoleUser,
			repoErr:   nil,
			wantErr:   nil,
			wantUser:  true,
			checkHash: true,
		},
		{
			name:      "the email is already busy",
			email:     "taken@example.com",
			password:  "secret123",
			role:      domain.RoleUser,
			repoErr:   domain.ErrDuplicateEmail,
			wantErr:   service.ErrEmailTaken,
			wantUser:  false,
			checkHash: false,
		},
		{
			name:      "database error",
			email:     "user@example.com",
			password:  "secret123",
			role:      domain.RoleUser,
			repoErr:   errors.New("connection refused"),
			wantErr:   errors.New("any"),
			wantUser:  false,
			checkHash: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewUserRepository(t)
			repo.On("CreateUser", mock.Anything, mock.AnythingOfType("*domain.User")).
				Run(func(args mock.Arguments) {
					u := args.Get(1).(*domain.User)
					if u.Password == tt.password {
						t.Error("password passed to repository must already be hashed")
					}
				}).
				Return(tt.repoErr)
			svc := service.NewAuthService(repo, testJWTSecret, logger)
			user, err := svc.Register(context.Background(), tt.email, tt.password, tt.role)

			if tt.wantErr == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if tt.wantErr != nil && err == nil {
				t.Fatalf("expected error, got nil")
			}

			if tt.repoErr != nil && !errors.Is(tt.repoErr, domain.ErrDuplicateEmail) {
				if errors.Is(err, service.ErrEmailTaken) {
					t.Error("db error should not be mapped to ErrEmailTaken")
				}
			}

			if tt.wantUser {
				if user == nil {
					t.Fatal("expected user, got nil")
				}
				if user.ID != uuid.Nil {
					t.Errorf("expected user ID to be zero, got %s", user.ID)
				}
				if user.Email != tt.email {
					t.Errorf("expected email %s, got %s", tt.email, user.Email)
				}
				if user.Role != tt.role {
					t.Errorf("expected role %s, got %s", tt.role, user.Role)
				}
				if user.CreatedAt.IsZero() {
					t.Error("expected CreatedAt to be set")
				}
			} else {
				if user != nil {
					t.Errorf("expected nil user, got %+v", user)
				}
			}

			if tt.checkHash && user != nil {
				if user.Password == tt.password {
					t.Error("password must be hashed, not stored as plaintext")
				}
				if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(tt.password)); err != nil {
					t.Error("password hash is invalid")
				}
			}
		})
	}
}

func TestAuth_Login(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	validPassword := "secret123"
	hash, err := bcrypt.GenerateFromPassword([]byte(validPassword), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	existingUser := &domain.User{
		ID:        uuid.New(),
		Email:     "user@example.com",
		Password:  string(hash),
		Role:      domain.RoleUser,
		CreatedAt: time.Now().UTC(),
	}

	tests := []struct {
		name      string
		email     string
		password  string
		repoUser  *domain.User
		repoErr   error
		wantErr   error
		wantToken bool
	}{
		{
			name:      "successful login",
			email:     existingUser.Email,
			password:  validPassword,
			repoUser:  existingUser,
			repoErr:   nil,
			wantErr:   nil,
			wantToken: true,
		},
		{
			name:      "user not found",
			email:     "unknown@example.com",
			password:  validPassword,
			repoUser:  nil,
			repoErr:   domain.ErrNotFound,
			wantErr:   domain.ErrUnauthorized,
			wantToken: false,
		},
		{
			name:      "wrong password",
			email:     existingUser.Email,
			password:  "wrong-password",
			repoUser:  existingUser,
			repoErr:   nil,
			wantErr:   domain.ErrUnauthorized,
			wantToken: false,
		},
		{
			name:      "database error",
			email:     existingUser.Email,
			password:  validPassword,
			repoUser:  nil,
			repoErr:   errors.New("connection refused"),
			wantErr:   errors.New("any"),
			wantToken: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewUserRepository(t)
			repo.On("GetByEmail", mock.Anything, tt.email).
				Return(tt.repoUser, tt.repoErr)

			svc := service.NewAuthService(repo, testJWTSecret, logger)
			token, err := svc.Login(context.Background(), tt.email, tt.password)

			if tt.wantErr == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.wantErr != nil && err == nil {
				t.Fatal("expected error, got nil")
			}

			if errors.Is(tt.wantErr, domain.ErrUnauthorized) {
				if !errors.Is(err, domain.ErrUnauthorized) {
					t.Errorf("expected ErrUnauthorized, got %v", err)
				}
			}

			if tt.wantToken {
				if token == "" {
					t.Error("expected non-empty token")
				}
			} else if token != "" {
				t.Errorf("expected empty token, got %q", token)
			}
		})
	}
}
