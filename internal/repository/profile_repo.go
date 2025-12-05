package repository

import (
	"context"

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

// TODO: implementar Create y GetByUserID.
func (r *PgProfileRepository) Create(ctx context.Context, profile domain.CloneProfile) error {
	return nil
}

func (r *PgProfileRepository) GetByUserID(ctx context.Context, userID string) (domain.CloneProfile, error) {
	return domain.CloneProfile{}, nil
}
