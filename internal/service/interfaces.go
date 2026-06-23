package service

import (
	"OctoQueue/internal/domain"
	"context"
)

type AuthService interface {
    Register(ctx context.Context, email, password string, role domain.Role) (*domain.User, error)
    Login(ctx context.Context, email, password string) (string, error)
}