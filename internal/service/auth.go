package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"OctoQueue/internal/domain"
	"OctoQueue/internal/storage/repository"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrEmailTaken = errors.New("Email already registered")

type Auth struct {
	users     repository.UserRepository
	jwtSecret string
}

func NewAuthService(users repository.UserRepository, jwtSecret string) AuthService {
	return &Auth{users: users, jwtSecret: jwtSecret}
}

func (s *Auth) Register(ctx context.Context, email, password string, role domain.Role) (*domain.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	u := &domain.User{
		Email:     email,
		Password:  string(hash),
		Role:      role,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.users.CreateUser(ctx, u); err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("creating user: %w", err)
	}
	return u, nil
}

func (s *Auth) Login(ctx context.Context, email, password string) (string, error) {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return "", domain.ErrUnauthorized
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		return "", domain.ErrUnauthorized
	}
	return s.generateToken(u.ID.String(), u.Role)
}

func (s *Auth) generateToken(userID string, role domain.Role) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"role":    string(role),
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}
