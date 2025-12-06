package repository

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgxpool"

	"clone-llm/internal/domain"
)

type TraitRepository interface {
	Upsert(ctx context.Context, trait domain.Trait) error
	FindByProfileID(ctx context.Context, profileID string) ([]domain.Trait, error)
	FindByCategory(ctx context.Context, profileID, category string) ([]domain.Trait, error)
}

type PgTraitRepository struct {
	pool *pgxpool.Pool
}

func NewPgTraitRepository(pool *pgxpool.Pool) *PgTraitRepository {
	return &PgTraitRepository{pool: pool}
}

func (r *PgTraitRepository) Upsert(ctx context.Context, trait domain.Trait) error {
	const query = `
		INSERT INTO traits (id, profile_id, category, trait, value, confidence, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (profile_id, category, trait)
		DO UPDATE SET
			value = EXCLUDED.value,
			confidence = EXCLUDED.confidence,
			updated_at = EXCLUDED.updated_at
	`

	var confidence interface{}
	if trait.Confidence != nil {
		confidence = *trait.Confidence
	}

	_, err := r.pool.Exec(ctx, query,
		trait.ID,
		trait.ProfileID,
		trait.Category,
		trait.Trait,
		trait.Value,
		confidence,
		trait.CreatedAt,
		trait.UpdatedAt,
	)
	return err
}

func (r *PgTraitRepository) FindByProfileID(ctx context.Context, profileID string) ([]domain.Trait, error) {
	const query = `
		SELECT id, profile_id, category, trait, value, confidence, created_at, updated_at
		FROM traits
		WHERE profile_id = $1
		ORDER BY category, trait
	`

	rows, err := r.pool.Query(ctx, query, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var traits []domain.Trait
	for rows.Next() {
		var t domain.Trait
		var confidence sql.NullFloat64

		if err := rows.Scan(
			&t.ID,
			&t.ProfileID,
			&t.Category,
			&t.Trait,
			&t.Value,
			&confidence,
			&t.CreatedAt,
			&t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if confidence.Valid {
			val := confidence.Float64
			t.Confidence = &val
		}
		traits = append(traits, t)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return traits, nil
}

func (r *PgTraitRepository) FindByCategory(ctx context.Context, profileID, category string) ([]domain.Trait, error) {
	const query = `
		SELECT id, profile_id, category, trait, value, confidence, created_at, updated_at
		FROM traits
		WHERE profile_id = $1 AND category = $2
		ORDER BY trait
	`

	rows, err := r.pool.Query(ctx, query, profileID, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var traits []domain.Trait
	for rows.Next() {
		var t domain.Trait
		var confidence sql.NullFloat64

		if err := rows.Scan(
			&t.ID,
			&t.ProfileID,
			&t.Category,
			&t.Trait,
			&t.Value,
			&confidence,
			&t.CreatedAt,
			&t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if confidence.Valid {
			val := confidence.Float64
			t.Confidence = &val
		}
		traits = append(traits, t)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return traits, nil
}
