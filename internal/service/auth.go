package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"OctoQueue/internal/domain"
	"OctoQueue/internal/lib/logger/sl"
	"OctoQueue/internal/storage/repository"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrEmailTaken = errors.New("Email already registered")

type Auth struct {
	users     repository.UserRepository
	jwtSecret string
	log       *slog.Logger
}

func NewAuthService(users repository.UserRepository, jwtSecret string, log *slog.Logger) AuthService {
	return &Auth{users: users, jwtSecret: jwtSecret, log: log}
}

func (s *Auth) Register(ctx context.Context, email, password string, role domain.Role) (*domain.User, error) {
	const op = "internal.service.Register"
	log := s.log.With(slog.String("op", op), slog.String("request_id", middleware.GetReqID(ctx)))

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
			log.Info("registration failed: email already taken", slog.String("email", email))
			return nil, ErrEmailTaken
		}
		log.Error("failed to create user in db", sl.Err(err), slog.String("email", email))
		return nil, fmt.Errorf("creating user: %w", err)
	}

	log.Info("user successfully registered", slog.String("user_id", u.ID.String()))
	return u, nil
}

func (s *Auth) Login(ctx context.Context, email, password string) (string, error) {
	const op = "internal.service.Login"
	log := s.log.With(slog.String("op", op), slog.String("request_id", middleware.GetReqID(ctx)))

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			log.Warn("login failed: user not found", slog.String("email", email))
			return "", domain.ErrUnauthorized
		}
		log.Error("db error during login", sl.Err(err), slog.String("email", email))
		return "", fmt.Errorf("getting user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		log.Warn("login failed: invalid password", slog.String("user_id", u.ID.String()))
		return "", domain.ErrUnauthorized
	}

	token, err := s.generateToken(u.ID.String(), u.Role)
	if err != nil {
		log.Error("failed to generate jwt token", sl.Err(err), slog.String("user_id", u.ID.String()))
		return "", err
	}

	log.Info("user successfully logged in", slog.String("user_id", u.ID.String()))

	return token, nil
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
