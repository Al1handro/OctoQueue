package auth

import (
	"context"
	"fmt"
	"time"

	"OctoQueue/internal/domain"
	"OctoQueue/internal/storage"

	// "github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	users     storage.UserRepository
	jwtSecret string
}

func NewAuthService(users storage.UserRepository, jwtSecret string) *AuthService {
	return &AuthService{users: users, jwtSecret: jwtSecret}
}

// TODO: добавить пакет domain и переместить туда все статические структуры
func (s *AuthService) Register(ctx context.Context, email, password string, role domain.Role) (*domain.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &domain.User{
		ID:        string(uuid.New()),
		Email:     email,
		Password:  string(hash),
		Role:      role,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("email already taken: %w", domain.ErrInvalidRequest)
	}
	return u, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, error) {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return "", domain.ErrUnauthorized
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		return "", domain.ErrUnauthorized
	}
	return s.generateToken(u.ID, u.Role)
}

// TODO: UUID необходимо получить из бд, т.к. в Postgres есть функция gen_random_uuid()
// func (s *AuthService) generateToken(userID string, role domain.Role) (string, error) {
// 	claims := jwt.MapClaims{
// 		"user_id": userID.String(),
// 		"role":    string(role),
// 		"exp":     time.Now().Add(24 * time.Hour).Unix(),
// 	}
// 	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
// 	return token.SignedString([]byte(s.jwtSecret))
// }
