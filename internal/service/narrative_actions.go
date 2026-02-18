package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"

	"clone-llm/internal/domain"
)

func (s *NarrativeService) CreateRelation(
	ctx context.Context,
	profileID uuid.UUID,
	name, relation, bondStatus string,
	rel domain.RelationshipVectors,
) error {
	if s == nil || s.characterRepo == nil {
		return ErrNarrativeServiceNotConfigured
	}
	if profileID == uuid.Nil {
		return ErrNarrativeInvalidInput
	}

	name = strings.TrimSpace(name)
	relation = strings.TrimSpace(relation)

	if name == "" {
		return fmt.Errorf("CreateRelation: name vacío")
	}
	if relation == "" {
		return fmt.Errorf("CreateRelation: relation vacío")
	}

	now := time.Now().UTC()
	char := domain.Character{
		ID:             uuid.New(),
		CloneProfileID: profileID,
		Name:           name,
		Relation:       relation,

		// No dupliques Relation en Archetype: o lo definís aparte o lo dejás vacío por ahora.
		Archetype: strings.TrimSpace(""),

		BondStatus:   strings.TrimSpace(bondStatus),
		Relationship: rel,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return s.characterRepo.Create(ctx, char)
}

func (s *NarrativeService) InjectMemory(
	ctx context.Context,
	profileID uuid.UUID,
	content string,
	importance, emotionalWeight, emotionalIntensity int,
	emotionCategory string,
) error {
	if s == nil || s.memoryRepo == nil || s.llmClient == nil {
		return ErrNarrativeServiceNotConfigured
	}
	if profileID == uuid.Nil {
		return ErrNarrativeInvalidInput
	}

	text := strings.TrimSpace(content)
	if text == "" {
		return nil
	}

	embed, err := s.llmClient.CreateEmbedding(ctx, text)
	if err != nil {
		return err
	}

	// Clamps consistentes
	if importance < 1 {
		importance = 1
	}
	if importance > 10 {
		importance = 10
	}

	if emotionalWeight < 1 {
		emotionalWeight = 1
	}
	if emotionalWeight > 10 {
		emotionalWeight = 10
	}

	if emotionalIntensity < 0 {
		emotionalIntensity = 0
	}
	if emotionalIntensity > 100 {
		emotionalIntensity = 100
	}

	category := strings.TrimSpace(emotionCategory)
	if category == "" {
		category = "NEUTRAL"
	}

	now := time.Now().UTC()
	mem := domain.NarrativeMemory{
		ID:                 uuid.New(),
		CloneProfileID:     profileID,
		RelatedCharacterID: nil, // TODO: permitir asociarlo cuando haya parsing de interlocutor

		Content:   text,
		Embedding: pgvector.NewVector(embed),

		Importance:         importance,
		EmotionalWeight:    emotionalWeight,
		EmotionalIntensity: emotionalIntensity,
		EmotionCategory:    category,
		SentimentLabel:     category,

		HappenedAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return s.memoryRepo.Create(ctx, mem)
}
