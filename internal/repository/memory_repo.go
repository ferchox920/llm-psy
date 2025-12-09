package repository

import (
	"context"
	"database/sql"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"clone-llm/internal/domain"
)

type MemoryRepository interface {
	Create(ctx context.Context, memory domain.NarrativeMemory) error
	Search(ctx context.Context, profileID uuid.UUID, queryEmbedding pgvector.Vector, k int) ([]domain.NarrativeMemory, error)
	ListByCharacter(ctx context.Context, characterID uuid.UUID) ([]domain.NarrativeMemory, error)
}

type PgMemoryRepository struct {
	pool *pgxpool.Pool
}

func NewPgMemoryRepository(pool *pgxpool.Pool) *PgMemoryRepository {
	return &PgMemoryRepository{pool: pool}
}

func (r *PgMemoryRepository) Create(ctx context.Context, memory domain.NarrativeMemory) error {
	intensity := memory.EmotionalIntensity
	if intensity <= 0 {
		intensity = 10
	}
	category := strings.TrimSpace(memory.EmotionCategory)
	if category == "" {
		category = "NEUTRAL"
	}
	const query = `
		INSERT INTO narrative_memories (
			id, clone_profile_id, related_character_id, content, embedding, importance, emotional_weight, emotional_intensity, emotion_category, sentiment_label, happened_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	var related interface{}
	if memory.RelatedCharacterID != nil {
		related = *memory.RelatedCharacterID
	}

	_, err := r.pool.Exec(ctx, query,
		memory.ID,
		memory.CloneProfileID,
		related,
		memory.Content,
		memory.Embedding,
		memory.Importance,
		memory.EmotionalWeight,
		intensity,
		category,
		memory.SentimentLabel,
		memory.HappenedAt,
		memory.CreatedAt,
		memory.UpdatedAt,
	)
	return err
}

func (r *PgMemoryRepository) Search(ctx context.Context, profileID uuid.UUID, queryEmbedding pgvector.Vector, k int) ([]domain.NarrativeMemory, error) {
	if k <= 0 {
		k = 5
	}
	const query = `
		SELECT id, clone_profile_id, related_character_id, content, embedding, importance, emotional_weight, emotional_intensity, emotion_category, sentiment_label, happened_at, created_at, updated_at
		FROM narrative_memories
		WHERE clone_profile_id = $1
		ORDER BY embedding <=> $2
		LIMIT $3
	`
	rows, err := r.pool.Query(ctx, query, profileID, queryEmbedding, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMemories(rows)
}

func (r *PgMemoryRepository) ListByCharacter(ctx context.Context, characterID uuid.UUID) ([]domain.NarrativeMemory, error) {
	const query = `
		SELECT id, clone_profile_id, related_character_id, content, embedding, importance, emotional_weight, emotional_intensity, emotion_category, sentiment_label, happened_at, created_at, updated_at
		FROM narrative_memories
		WHERE related_character_id = $1
		ORDER BY happened_at DESC
	`
	rows, err := r.pool.Query(ctx, query, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMemories(rows)
}

func scanMemories(rows pgxRows) ([]domain.NarrativeMemory, error) {
	var memories []domain.NarrativeMemory
	for rows.Next() {
		var m domain.NarrativeMemory
		var related sql.NullString
		if err := rows.Scan(
			&m.ID,
			&m.CloneProfileID,
			&related,
			&m.Content,
			&m.Embedding,
			&m.Importance,
			&m.EmotionalWeight,
			&m.EmotionalIntensity,
			&m.EmotionCategory,
			&m.SentimentLabel,
			&m.HappenedAt,
			&m.CreatedAt,
			&m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if related.Valid {
			id, err := uuid.Parse(related.String)
			if err == nil {
				m.RelatedCharacterID = &id
			}
		}
		memories = append(memories, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return memories, nil
}

// pgxRows is a minimal interface to allow scanning from pgx rows and simplify testing.
type pgxRows interface {
	Next() bool
	Scan(...interface{}) error
	Err() error
	Close()
}
