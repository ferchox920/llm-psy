package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
)

type actionFakeCharacterRepo struct {
	created domain.Character
	err     error
}

func (f *actionFakeCharacterRepo) Create(ctx context.Context, c domain.Character) error {
	if f.err != nil {
		return f.err
	}
	f.created = c
	return nil
}

func (f *actionFakeCharacterRepo) Update(context.Context, domain.Character) error {
	return nil
}

func (f *actionFakeCharacterRepo) ListByProfileID(context.Context, uuid.UUID) ([]domain.Character, error) {
	return nil, nil
}

func (f *actionFakeCharacterRepo) FindByName(context.Context, uuid.UUID, string) (*domain.Character, error) {
	return nil, nil
}

type actionFakeMemoryRepo struct {
	created domain.NarrativeMemory
	err     error
}

func (f *actionFakeMemoryRepo) Create(ctx context.Context, m domain.NarrativeMemory) error {
	if f.err != nil {
		return f.err
	}
	f.created = m
	return nil
}

func (f *actionFakeMemoryRepo) Search(context.Context, uuid.UUID, pgvector.Vector, int, float64) ([]repository.ScoredMemory, error) {
	return nil, nil
}

func (f *actionFakeMemoryRepo) ListByCharacter(context.Context, uuid.UUID) ([]domain.NarrativeMemory, error) {
	return nil, nil
}

func (f *actionFakeMemoryRepo) GetRecentHighImpactByProfile(context.Context, uuid.UUID, int, int, int) ([]domain.NarrativeMemory, error) {
	return nil, nil
}

type actionFakeLLM struct {
	embedding []float32
	err       error
}

func (f actionFakeLLM) CreateEmbedding(context.Context, string) ([]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.embedding, nil
}

func (f actionFakeLLM) Generate(context.Context, string) (string, error) { return "", nil }

func TestCreateRelation_ValidationAndPersist(t *testing.T) {
	profileID := uuid.New()
	charRepo := &actionFakeCharacterRepo{}
	svc := &NarrativeService{characterRepo: charRepo}

	if err := svc.CreateRelation(context.Background(), profileID, " Ana ", " amiga ", " estable ", domain.RelationshipVectors{Trust: 10}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if charRepo.created.Name != "Ana" || charRepo.created.Relation != "amiga" || charRepo.created.BondStatus != "estable" {
		t.Fatalf("expected trimmed values, got %+v", charRepo.created)
	}
	if charRepo.created.CloneProfileID != profileID {
		t.Fatalf("expected profile id propagated")
	}
}

func TestCreateRelation_NotConfiguredAndInvalidInput(t *testing.T) {
	var svc *NarrativeService
	if err := svc.CreateRelation(context.Background(), uuid.New(), "a", "b", "", domain.RelationshipVectors{}); !errors.Is(err, ErrNarrativeServiceNotConfigured) {
		t.Fatalf("expected ErrNarrativeServiceNotConfigured, got %v", err)
	}

	svc = &NarrativeService{characterRepo: &actionFakeCharacterRepo{}}
	if err := svc.CreateRelation(context.Background(), uuid.Nil, "a", "b", "", domain.RelationshipVectors{}); !errors.Is(err, ErrNarrativeInvalidInput) {
		t.Fatalf("expected ErrNarrativeInvalidInput, got %v", err)
	}
	if err := svc.CreateRelation(context.Background(), uuid.New(), "", "b", "", domain.RelationshipVectors{}); err == nil {
		t.Fatalf("expected error on empty name")
	}
	if err := svc.CreateRelation(context.Background(), uuid.New(), "a", "", "", domain.RelationshipVectors{}); err == nil {
		t.Fatalf("expected error on empty relation")
	}
}

func TestInjectMemory_ValidationAndClamps(t *testing.T) {
	profileID := uuid.New()
	memRepo := &actionFakeMemoryRepo{}
	svc := &NarrativeService{
		memoryRepo: memRepo,
		llmClient:  actionFakeLLM{embedding: []float32{1, 2, 3}},
	}

	if err := svc.InjectMemory(context.Background(), profileID, "  evento  ", -2, 99, 140, " "); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	got := memRepo.created
	if got.Content != "evento" {
		t.Fatalf("expected trimmed content, got %q", got.Content)
	}
	if got.Importance != 1 || got.EmotionalWeight != 10 || got.EmotionalIntensity != 100 {
		t.Fatalf("expected clamped values, got importance=%d weight=%d intensity=%d", got.Importance, got.EmotionalWeight, got.EmotionalIntensity)
	}
	if got.EmotionCategory != "NEUTRAL" || got.SentimentLabel != "NEUTRAL" {
		t.Fatalf("expected neutral category defaults, got cat=%q sentiment=%q", got.EmotionCategory, got.SentimentLabel)
	}
}

func TestInjectMemory_NotConfiguredInvalidOrEmptyInput(t *testing.T) {
	var svc *NarrativeService
	if err := svc.InjectMemory(context.Background(), uuid.New(), "x", 1, 1, 1, "IRA"); !errors.Is(err, ErrNarrativeServiceNotConfigured) {
		t.Fatalf("expected ErrNarrativeServiceNotConfigured, got %v", err)
	}

	svc = &NarrativeService{memoryRepo: &actionFakeMemoryRepo{}, llmClient: actionFakeLLM{embedding: []float32{1}}}
	if err := svc.InjectMemory(context.Background(), uuid.Nil, "x", 1, 1, 1, "IRA"); !errors.Is(err, ErrNarrativeInvalidInput) {
		t.Fatalf("expected ErrNarrativeInvalidInput, got %v", err)
	}

	if err := svc.InjectMemory(context.Background(), uuid.New(), "   ", 1, 1, 1, "IRA"); err != nil {
		t.Fatalf("expected no-op nil error for empty content, got %v", err)
	}
}
