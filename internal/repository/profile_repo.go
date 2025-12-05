package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"clone-llm/internal/domain"
)

type ProfileRepository interface {
	Create(ctx context.Context, profile domain.CloneProfile) error
	GetByUserID(ctx context.Context, userID string) (domain.CloneProfile, error)
}

type PgProfileRepository struct {
	pool *pgxpool.Pool
}

func NewPgProfileRepository(pool *pgxpool.Pool) *PgProfileRepository {
	return &PgProfileRepository{pool: pool}
}

func (r *PgProfileRepository) Create(ctx context.Context, profile domain.CloneProfile) error {
	const query = `
		INSERT INTO clone_profiles (id, user_id, name, bio, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.pool.Exec(ctx, query,
		profile.ID,
		profile.UserID,
		profile.Name,
		profile.Bio,
		profile.CreatedAt,
	)
	return err
}

func (r *PgProfileRepository) GetByUserID(ctx context.Context, userID string) (domain.CloneProfile, error) {
	const query = `
		SELECT id, user_id, name, bio, created_at
		FROM clone_profiles
		WHERE user_id = $1
	`
	var profile domain.CloneProfile
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&profile.ID,
		&profile.UserID,
		&profile.Name,
		&profile.Bio,
		&profile.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CloneProfile{}, err
	}
	return profile, err
}
