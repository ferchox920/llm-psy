package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
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

func (r *PgSessionRepository) Create(ctx context.Context, session domain.Session) error {
	const query = `
		INSERT INTO sessions (id, user_id, token, expires_at, trust_level, intimacy_level, respect_level, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.pool.Exec(ctx, query,
		session.ID,
		session.UserID,
		session.Token,
		session.ExpiresAt,
		session.Relationship.Trust,
		session.Relationship.Intimacy,
		session.Relationship.Respect,
		session.CreatedAt,
	)
	return err
}

func (r *PgSessionRepository) GetByID(ctx context.Context, id string) (domain.Session, error) {
	const query = `
		SELECT id, user_id, token, expires_at, trust_level, intimacy_level, respect_level, created_at
		FROM sessions
		WHERE id = $1
	`
	var session domain.Session
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&session.ID,
		&session.UserID,
		&session.Token,
		&session.ExpiresAt,
		&session.Relationship.Trust,
		&session.Relationship.Intimacy,
		&session.Relationship.Respect,
		&session.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, err
	}
	return session, err
}
