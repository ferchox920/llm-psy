package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
)

// --- MOCKS DE REPOSITORIOS EN MEMORIA ---

type memoryMessageRepo struct {
	msgs []domain.Message
}

func newMemoryMessageRepo() *memoryMessageRepo { return &memoryMessageRepo{} }
func (m *memoryMessageRepo) Create(ctx context.Context, msg domain.Message) error {
	m.msgs = append(m.msgs, msg)
	return nil
}
func (m *memoryMessageRepo) ListBySessionID(ctx context.Context, sessionID string) ([]domain.Message, error) {
	var out []domain.Message
	for _, v := range m.msgs {
		if v.SessionID == sessionID {
			out = append(out, v)
		}
	}
	return out, nil
}

type memoryProfileRepo struct {
	profile domain.CloneProfile
}

func (m *memoryProfileRepo) Create(ctx context.Context, profile domain.CloneProfile) error {
	m.profile = profile
	return nil
}
func (m *memoryProfileRepo) GetByID(ctx context.Context, id string) (domain.CloneProfile, error) {
	if m.profile.ID == id {
		return m.profile, nil
	}
	return domain.CloneProfile{}, fmt.Errorf("not found")
}
func (m *memoryProfileRepo) GetByUserID(ctx context.Context, userID string) (domain.CloneProfile, error) {
	if m.profile.UserID == userID {
		return m.profile, nil
	}
	return domain.CloneProfile{}, fmt.Errorf("not found")
}

type memoryTraitRepo struct {
	traits []domain.Trait
}

func (m *memoryTraitRepo) Upsert(ctx context.Context, trait domain.Trait) error {
	for i := range m.traits {
		if m.traits[i].ID == trait.ID {
			m.traits[i] = trait
			return nil
		}
	}
	m.traits = append(m.traits, trait)
	return nil
}

func (m *memoryTraitRepo) FindByProfileID(ctx context.Context, profileID string) ([]domain.Trait, error) {
	var out []domain.Trait
	for _, t := range m.traits {
		// FIX: domain.Trait suele tener ProfileID (no CloneProfileID)
		if t.ProfileID == profileID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *memoryTraitRepo) FindByCategory(ctx context.Context, profileID, category string) ([]domain.Trait, error) {
	var out []domain.Trait
	for _, t := range m.traits {
		if t.ProfileID == profileID && strings.EqualFold(t.Category, category) {
			out = append(out, t)
		}
	}
	return out, nil
}

type memoryCharacterRepo struct {
	chars []domain.Character
}

func (m *memoryCharacterRepo) Create(ctx context.Context, character domain.Character) error {
	m.chars = append(m.chars, character)
	return nil
}
func (m *memoryCharacterRepo) Update(ctx context.Context, character domain.Character) error { return nil }

func (m *memoryCharacterRepo) ListByProfileID(ctx context.Context, profileID uuid.UUID) ([]domain.Character, error) {
	var out []domain.Character
	for _, c := range m.chars {
		if c.CloneProfileID == profileID {
			out = append(out, c)
		}
	}
	return out, nil
}

// FIX: devolver puntero al elemento real del slice, no a la copia del range
func (m *memoryCharacterRepo) FindByName(ctx context.Context, profileID uuid.UUID, name string) (*domain.Character, error) {
	for i := range m.chars {
		c := &m.chars[i]
		if c.CloneProfileID == profileID && strings.EqualFold(c.Name, name) {
			return c, nil
		}
	}
	return nil, nil
}

type memoryMemoryRepo struct {
	memories []domain.NarrativeMemory
	filter   string
}

func (m *memoryMemoryRepo) Create(ctx context.Context, memory domain.NarrativeMemory) error {
	m.memories = append(m.memories, memory)
	return nil
}

// Mock de Search: Filtra por string básico en lugar de vector
func (m *memoryMemoryRepo) Search(ctx context.Context, profileID uuid.UUID, queryEmbedding pgvector.Vector, k int, emotionalWeightFactor float64) ([]repository.ScoredMemory, error) {
	if strings.TrimSpace(m.filter) == "" {
		return nil, fmt.Errorf("memoryMemoryRepo.Search: filter vacío (test mal configurado)")
	}

	var results []repository.ScoredMemory
	for _, mem := range m.memories {
		if mem.CloneProfileID == profileID &&
			strings.Contains(strings.ToLower(mem.Content), strings.ToLower(m.filter)) {
			results = append(results, repository.ScoredMemory{
				NarrativeMemory: mem,
				Similarity:      1.0,
				Score:           1.0,
			})
		}
	}

	if k > 0 && len(results) > k {
		results = results[:k]
	}
	return results, nil
}

func (m *memoryMemoryRepo) ListByCharacter(ctx context.Context, characterID uuid.UUID) ([]domain.NarrativeMemory, error) {
	return nil, nil
}
