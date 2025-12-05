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

func (r *PgMessageRepository) Create(ctx context.Context, message domain.Message) error {
	const query = `
		INSERT INTO messages (id, user_id, session_id, content, role, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	var sessionID interface{}
	if message.SessionID != "" {
		sessionID = message.SessionID
	}

	_, err := r.pool.Exec(ctx, query,
		message.ID,
		message.UserID,
		sessionID,
		message.Content,
		message.Role,
		message.CreatedAt,
	)
	return err
}

func (r *PgMessageRepository) ListBySessionID(ctx context.Context, sessionID string) ([]domain.Message, error) {
	const query = `
		SELECT id, user_id, session_id, content, role, created_at
		FROM messages
		WHERE session_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.pool.Query(ctx, query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []domain.Message
	for rows.Next() {
		var msg domain.Message
		var sessionIDValue *string

		err = rows.Scan(
			&msg.ID,
			&msg.UserID,
			&sessionIDValue,
			&msg.Content,
			&msg.Role,
			&msg.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		if sessionIDValue != nil {
			msg.SessionID = *sessionIDValue
		}
		messages = append(messages, msg)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}
