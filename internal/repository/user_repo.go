package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"clone-llm/internal/domain"
)

// UserRepository define el contrato de persistencia para usuarios.
type UserRepository interface {
	Create(ctx context.Context, user domain.User) error
	GetByID(ctx context.Context, id string) (domain.User, error)
}

// PgUserRepository implementa UserRepository usando pgxpool.
type PgUserRepository struct {
	pool *pgxpool.Pool
}

func NewPgUserRepository(pool *pgxpool.Pool) *PgUserRepository {
	return &PgUserRepository{pool: pool}
}

func (r *PgUserRepository) Create(ctx context.Context, user domain.User) error {
	const query = `
		INSERT INTO users (id, email, display_name, created_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err := r.pool.Exec(ctx, query,
		user.ID,
		user.Email,
		user.DisplayName,
		user.CreatedAt,
	)
	return err
}

func (r *PgUserRepository) GetByID(ctx context.Context, id string) (domain.User, error) {
	const query = `
		SELECT id, email, display_name, created_at
		FROM users
		WHERE id = $1
	`
	var u domain.User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&u.ID,
		&u.Email,
		&u.DisplayName,
		&u.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, err
	}
	return u, err
}
