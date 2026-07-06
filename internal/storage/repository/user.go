package repository

import (
	"OctoQueue/internal/domain"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgUserRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *pgUserRepo {
	return &pgUserRepo{pool: pool}
}

func (s *pgUserRepo) Pool() *pgxpool.Pool {
	return s.pool
}

func (s *pgUserRepo) Close() {
	s.pool.Close()
}

func (s *pgUserRepo) CreateUser(ctx context.Context, user *domain.User) error {
	const op = "storage.repository.CreateUser"

	var id uuid.UUID
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, role, created_at) 
		 VALUES ($1, $2, $3, $4) 
		 RETURNING id`,
		user.Email, user.Password, string(user.Role), user.CreatedAt,
	).Scan(&id)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrDuplicateEmail
		}
		return fmt.Errorf("%s: creating user: %w", op, err)
	}

	user.ID = id
	return nil
}

func (s *pgUserRepo) GetUser(ctx context.Context, id string) (*domain.User, error) {
	const op = "storage.repository.GetUser"
	var u domain.User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, role, created_at
			FROM users
			WHERE id = $1
		`, id).Scan(&u.ID, &u.Email, &u.Password, &u.Role, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, domain.ErrUserNotFound)
		}
		return nil, fmt.Errorf("%s: select: %w", op, err)
	}
	return &u, nil
}

func (s *pgUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	const op = "storage.repository.GetByEmail"

	var u domain.User
	err := s.pool.QueryRow(ctx, `
	SELECT id, email, password_hash, role, created_at
		FROM users
		WHERE email = $1
	`, email).Scan(&u.ID, &u.Email, &u.Password, &u.Role, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, domain.ErrUserNotFound)
		}
		return nil, fmt.Errorf("%s: select: %w", op, err)
	}
	return &u, nil
}
