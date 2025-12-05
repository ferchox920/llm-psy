package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"clone-llm/internal/domain"
)

type MessageRepository interface {
	Create(ctx context.Context, message domain.Message) error
	ListBySessionID(ctx context.Context, sessionID string) ([]domain.Message, error)
}

type PgMessageRepository struct {
	pool *pgxpool.Pool
}

func NewPgMessageRepository(pool *pgxpool.Pool) *PgMessageRepository {
	return &PgMessageRepository{pool: pool}
}

// TODO: implementar Create y ListBySessionID.
func (r *PgMessageRepository) Create(ctx context.Context, message domain.Message) error {
	return nil
}

func (r *PgMessageRepository) ListBySessionID(ctx context.Context, sessionID string) ([]domain.Message, error) {
	return nil, nil
}
