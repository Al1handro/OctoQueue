package repository

import (
	"OctoQueue/internal/domain"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDuplicateEmail = errors.New("email already taken")

type pgUserRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &pgUserRepo{pool: pool}
}

func (s *pgUserRepo) Pool() *pgxpool.Pool {	
	return s.pool
}

func (s *pgUserRepo) Close() {
	s.pool.Close()
}

func (s *pgUserRepo) CreateUser(ctx context.Context, user *domain.User) error {
	const op = "storage.pgsql.CreateUser"

	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, role, created_at) VALUES ($1,$2,$3,$4)`,
		user.Email, user.Password, string(user.Role), user.CreatedAt,
	)
	if err != nil {
        var pgErr *pgconn.PgError
        if errors.As(err, &pgErr) && pgErr.Code == "23505" {
            return ErrDuplicateEmail
        }
        return fmt.Errorf("%s: creating user: %w", op, err)
    }
    return nil
}

func (s *pgUserRepo) GetUser(ctx context.Context, id string) (*domain.User, error) {
	const op = "storage.pgsql.GetUser"
	
	var u domain.User
	err := s.pool.QueryRow(ctx, `
	SELECT id, email, password, role, created_at
		FROM users
		WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.Password, &u.Role, &u.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("%s: user not found: %w", op, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("%s: select: %w", op, err)
	}
	return &u, nil
}

func (s *pgUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	const op = "storage.pgsql.GetByEmail"		
	var u domain.User
	err := s.pool.QueryRow(ctx, `
	SELECT id, email, password_hash, role, created_at
		FROM users
		WHERE email = $1
	`, email).Scan(&u.ID, &u.Email, &u.Password, &u.Role, &u.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("%s: user not found: %w", op, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("%s: select: %w", op, err)
	}
	return &u, nil
}