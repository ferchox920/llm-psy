package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"clone-llm/internal/domain"
)

type SessionRepository interface {
	Create(ctx context.Context, session domain.Session) error
	GetByID(ctx context.Context, id string) (domain.Session, error)
}

type PgSessionRepository struct {
	pool *pgxpool.Pool
}

func NewPgSessionRepository(pool *pgxpool.Pool) *PgSessionRepository {
	return &PgSessionRepository{pool: pool}
}

// TODO: implementar Create y GetByID.
func (r *PgSessionRepository) Create(ctx context.Context, session domain.Session) error {
	return nil
}

func (r *PgSessionRepository) GetByID(ctx context.Context, id string) (domain.Session, error) {
	return domain.Session{}, nil
}
