package repository

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"clone-llm/internal/domain"
)

type CharacterRepository interface {
	Create(ctx context.Context, character domain.Character) error
	Update(ctx context.Context, character domain.Character) error
	ListByProfileID(ctx context.Context, profileID uuid.UUID) ([]domain.Character, error)
	FindByName(ctx context.Context, profileID uuid.UUID, name string) (*domain.Character, error)
}

type PgCharacterRepository struct {
	pool *pgxpool.Pool
}

func NewPgCharacterRepository(pool *pgxpool.Pool) *PgCharacterRepository {
	return &PgCharacterRepository{pool: pool}
}

func (r *PgCharacterRepository) Create(ctx context.Context, character domain.Character) error {
	const query = `
		INSERT INTO characters (id, clone_profile_id, name, relation, archetype, bond_status, trust, intimacy, respect, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := r.pool.Exec(ctx, query,
		character.ID,
		character.CloneProfileID,
		character.Name,
		character.Relation,
		character.Archetype,
		character.BondStatus,
		character.Relationship.Trust,
		character.Relationship.Intimacy,
		character.Relationship.Respect,
		character.CreatedAt,
		character.UpdatedAt,
	)
	return err
}

func (r *PgCharacterRepository) Update(ctx context.Context, character domain.Character) error {
	const query = `
		UPDATE characters
		SET name = $1, relation = $2, archetype = $3, bond_status = $4, trust = $5, intimacy = $6, respect = $7, updated_at = $8
		WHERE id = $9
	`
	_, err := r.pool.Exec(ctx, query,
		character.Name,
		character.Relation,
		character.Archetype,
		character.BondStatus,
		character.Relationship.Trust,
		character.Relationship.Intimacy,
		character.Relationship.Respect,
		character.UpdatedAt,
		character.ID,
	)
	return err
}

func (r *PgCharacterRepository) ListByProfileID(ctx context.Context, profileID uuid.UUID) ([]domain.Character, error) {
	const query = `
		SELECT id, clone_profile_id, name, relation, archetype, bond_status, trust, intimacy, respect, created_at, updated_at
		FROM characters
		WHERE clone_profile_id = $1
		ORDER BY intimacy DESC, trust DESC, name ASC
	`
	rows, err := r.pool.Query(ctx, query, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chars []domain.Character
	for rows.Next() {
		var c domain.Character
		if err := rows.Scan(
			&c.ID,
			&c.CloneProfileID,
			&c.Name,
			&c.Relation,
			&c.Archetype,
			&c.BondStatus,
			&c.Relationship.Trust,
			&c.Relationship.Intimacy,
			&c.Relationship.Respect,
			&c.CreatedAt,
			&c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		chars = append(chars, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return chars, nil
}

func (r *PgCharacterRepository) FindByName(ctx context.Context, profileID uuid.UUID, name string) (*domain.Character, error) {
	const query = `
		SELECT id, clone_profile_id, name, relation, archetype, bond_status, trust, intimacy, respect, created_at, updated_at
		FROM characters
		WHERE clone_profile_id = $1 AND LOWER(name) = LOWER($2)
	`
	var c domain.Character
	err := r.pool.QueryRow(ctx, query, profileID, strings.TrimSpace(name)).Scan(
		&c.ID,
		&c.CloneProfileID,
		&c.Name,
		&c.Relation,
		&c.Archetype,
		&c.BondStatus,
		&c.Relationship.Trust,
		&c.Relationship.Intimacy,
		&c.Relationship.Respect,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}
