package service_test

import (
	"OctoQueue/internal/domain"
	"OctoQueue/internal/service"
	"OctoQueue/internal/storage/repository"
	"OctoQueue/internal/storage/repository/mocks"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
)

const testJWTSecret = "test-secret"

func TestAuth_Register(t *testing.T) {
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
			name:     "the email is already busy",
			email:    "taken@example.com",
			password: "secret123",
			role:     domain.RoleUser,
			repoErr:  repository.ErrDuplicateEmail,
			wantErr:  service.ErrEmailTaken,
			wantUser: false,
		},
		{
			name:     "database error",
			email:    "user@example.com",
			password: "secret123",
			role:     domain.RoleUser,
			repoErr:  errors.New("connection refused"),
			wantErr:  errors.New("any"),
			wantUser: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewUserRepository(t)
			repo.On("CreateUser", context.Background(), mock.AnythingOfType("*domain.User")).
				Return(tt.repoErr)
			svc := service.NewAuthService(repo, testJWTSecret)
			user, err := svc.Register(context.Background(), "user@example.com", "secret123", domain.RoleUser)

			if tt.wantErr == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if tt.wantErr != nil && err == nil {
				t.Fatalf("expected error, got nil")
			}

			if tt.wantErr != nil && !errors.Is(tt.wantErr, service.ErrEmailTaken) {
				if !errors.Is(err, service.ErrEmailTaken) {
					t.Errorf("expected ErrEmailTaken, got %v", err)
				}
			}

			if tt.repoErr != nil && !errors.Is(tt.repoErr, repository.ErrDuplicateEmail) {
				if errors.Is(err, service.ErrEmailTaken) {
					t.Error("db error should not be mapped to ErrEmailTaken")
				}
			}

			if tt.wantUser {
				if user == nil {
					t.Fatal("expected user, got nil")
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

// func TestAuth_Login(t *testing.T) {
// 	tests := []struct {
// 		name      string
// 		email     string
// 		password  string
// 		role      domain.Role
// 		repoErr   error
// 		wantErr   error
// 		wantUser  bool
// 		checkHash bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			// TODO: construct the receiver type.
// 			var s service.Auth
// 			got, gotErr := s.Login(context.Background(), tt.email, tt.password)
// 			if gotErr != nil {
// 				if !tt.wantErr {
// 					t.Errorf("Login() failed: %v", gotErr)
// 				}
// 				return
// 			}
// 			if tt.wantErr {
// 				t.Fatal("Login() succeeded unexpectedly")
// 			}
// 			// TODO: update the condition below to compare got with tt.want.
// 			if true {
// 				t.Errorf("Login() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }
