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
func (m *memoryCharacterRepo) Update(ctx context.Context, character domain.Character) error {
	return nil
}

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

func normalizeASCII(s string) string {
	mapping := map[rune]rune{
		'\u00e1': 'a', '\u00e0': 'a', '\u00e4': 'a', '\u00e2': 'a',
		'\u00c1': 'A', '\u00c0': 'A', '\u00c4': 'A', '\u00c2': 'A',
		'\u00e9': 'e', '\u00e8': 'e', '\u00eb': 'e', '\u00ea': 'e',
		'\u00c9': 'E', '\u00c8': 'E', '\u00cb': 'E', '\u00ca': 'E',
		'\u00ed': 'i', '\u00ec': 'i', '\u00ef': 'i', '\u00ee': 'i',
		'\u00cd': 'I', '\u00cc': 'I', '\u00cf': 'I', '\u00ce': 'I',
		'\u00f3': 'o', '\u00f2': 'o', '\u00f6': 'o', '\u00f4': 'o',
		'\u00d3': 'O', '\u00d2': 'O', '\u00d6': 'O', '\u00d4': 'O',
		'\u00fa': 'u', '\u00f9': 'u', '\u00fc': 'u', '\u00fb': 'u',
		'\u00da': 'U', '\u00d9': 'U', '\u00dc': 'U', '\u00db': 'U',
		'\u00f1': 'n', '\u00d1': 'N',
	}
	mapFn := func(r rune) rune {
		if v, ok := mapping[r]; ok {
			return v
		}
		return r
	}
	return strings.ToLower(strings.Map(mapFn, s))
}

func (m *memoryMemoryRepo) Create(ctx context.Context, memory domain.NarrativeMemory) error {
	m.memories = append(m.memories, memory)
	return nil
}

// Mock de Search: Filtra por string básico en lugar de vector
func (m *memoryMemoryRepo) Search(ctx context.Context, profileID uuid.UUID, queryEmbedding pgvector.Vector, k int, emotionalWeightFactor float64) ([]repository.ScoredMemory, error) {
	normFilter := normalizeASCII(m.filter)
	if strings.TrimSpace(normFilter) == "" {
		return []repository.ScoredMemory{}, nil
	}

	var results []repository.ScoredMemory
	for _, mem := range m.memories {
		if mem.CloneProfileID == profileID &&
			strings.Contains(normalizeASCII(mem.Content), normFilter) {
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

func (m *memoryMemoryRepo) GetRecentHighImpactByProfile(ctx context.Context, profileID uuid.UUID, limit int, minImportance int, minEmotionalIntensity int) ([]domain.NarrativeMemory, error) {
	var out []domain.NarrativeMemory
	for _, mem := range m.memories {
		if mem.CloneProfileID != profileID {
			continue
		}
		if mem.Importance >= minImportance || mem.EmotionalIntensity >= minEmotionalIntensity {
			out = append(out, mem)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
