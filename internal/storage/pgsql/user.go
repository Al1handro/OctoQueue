package pgsql

import (
	"OctoQueue/internal/domain"
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Storage) CreateUser(ctx context.Context, p *domain.User) (string, error) {
	const op = "storage.pgsql.CreateUser"

	row := s.Pool().QueryRow(ctx, `
		INSERT INTO users (email, password, role, created_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id;`,
		p.Email, p.Password, p.Role, p.CreatedAt,
	)
	
	u, err := scanUser(row)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return u.ID, nil
}

func (s *Storage) GetUser(ctx context.Context, id string) (*domain.User, error) {
	const op = "storage.pgsql.GetUser"
	
	var u domain.User
	err := s.Pool().QueryRow(ctx, `
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

func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.Email, &u.Password, &u.Role, &u.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found: %w", domain.ErrNotFound)
		}
		return nil, err
	}
	return &u, nil
}