package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
)

type mockProfileRepo struct {
	profile domain.CloneProfile
	err     error
}

func (m *mockProfileRepo) Create(ctx context.Context, profile domain.CloneProfile) error {
	return errors.New("not implemented")
}

func (m *mockProfileRepo) GetByID(ctx context.Context, id string) (domain.CloneProfile, error) {
	return m.profile, m.err
}

func (m *mockProfileRepo) GetByUserID(ctx context.Context, userID string) (domain.CloneProfile, error) {
	return m.profile, m.err
}

type mockTraitRepo struct {
	upsertCount int
	lastTrait   domain.Trait
	allTraits   []domain.Trait
	err         error
}

func (m *mockTraitRepo) Upsert(ctx context.Context, trait domain.Trait) error {
	m.upsertCount++
	m.lastTrait = trait
	m.allTraits = append(m.allTraits, trait)
	return m.err
}

func (m *mockTraitRepo) FindByProfileID(ctx context.Context, profileID string) ([]domain.Trait, error) {
	return nil, errors.New("not implemented")
}

func (m *mockTraitRepo) FindByCategory(ctx context.Context, profileID, category string) ([]domain.Trait, error) {
	return nil, errors.New("not implemented")
}

func TestAnalysisServiceHappyPath(t *testing.T) {
	llmClient := &llm.MockClient{
		Response: `{"traits": [{"trait": "extraversion", "value": 80, "confidence": 0.9}]}`,
	}
	profileRepo := &mockProfileRepo{profile: domain.CloneProfile{ID: "profile-1"}}
	traitRepo := &mockTraitRepo{}

	svc := NewAnalysisService(llmClient, traitRepo, profileRepo, zap.NewNop())

	err := svc.AnalyzeAndPersist(context.Background(), "user-1", "hola mundo")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if traitRepo.upsertCount != 1 {
		t.Fatalf("expected upsert called once, got %d", traitRepo.upsertCount)
	}

	if traitRepo.lastTrait.Category != domain.TraitCategoryBigFive {
		t.Fatalf("expected category %s, got %s", domain.TraitCategoryBigFive, traitRepo.lastTrait.Category)
	}

	if traitRepo.lastTrait.Value != 80 {
		t.Fatalf("expected value 80, got %d", traitRepo.lastTrait.Value)
	}

	if traitRepo.lastTrait.ProfileID != "profile-1" {
		t.Fatalf("expected profile id profile-1, got %s", traitRepo.lastTrait.ProfileID)
	}

	if traitRepo.lastTrait.CreatedAt.IsZero() || traitRepo.lastTrait.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps to be set")
	}
}

func TestAnalysisServiceInvalidJSON(t *testing.T) {
	llmClient := &llm.MockClient{
		Response: `Lo siento, no puedo procesar...`,
	}
	profileRepo := &mockProfileRepo{profile: domain.CloneProfile{ID: "profile-1"}}
	traitRepo := &mockTraitRepo{}

	svc := NewAnalysisService(llmClient, traitRepo, profileRepo, zap.NewNop())

	err := svc.AnalyzeAndPersist(context.Background(), "user-1", "hola mundo")
	if err == nil {
		t.Fatalf("expected error due to invalid JSON, got nil")
	}

	if traitRepo.upsertCount != 0 {
		t.Fatalf("expected upsert not called, got %d", traitRepo.upsertCount)
	}
}

func TestAnalysisServiceCleansMarkdown(t *testing.T) {
	llmClient := &llm.MockClient{
		Response: "```json\n{\"traits\": [{\"trait\": \"openness\", \"value\": 70, \"confidence\": 0.8}]}\n```",
	}
	profileRepo := &mockProfileRepo{profile: domain.CloneProfile{ID: "profile-2"}}
	traitRepo := &mockTraitRepo{}

	svc := NewAnalysisService(llmClient, traitRepo, profileRepo, zap.NewNop())

	if err := svc.AnalyzeAndPersist(context.Background(), "user-2", "texto con markdown"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if traitRepo.upsertCount != 1 {
		t.Fatalf("expected upsert called once, got %d", traitRepo.upsertCount)
	}

	if traitRepo.lastTrait.Trait != "openness" {
		t.Fatalf("expected trait openness, got %s", traitRepo.lastTrait.Trait)
	}

	if traitRepo.lastTrait.Value != 70 {
		t.Fatalf("expected value 70, got %d", traitRepo.lastTrait.Value)
	}

	if traitRepo.lastTrait.Confidence == nil || *traitRepo.lastTrait.Confidence != 0.8 {
		t.Fatalf("expected confidence 0.8, got %v", traitRepo.lastTrait.Confidence)
	}

	if traitRepo.lastTrait.CreatedAt.After(time.Now().UTC().Add(1 * time.Minute)) {
		t.Fatalf("unexpected created_at timestamp")
	}
}

func TestAnalysisServiceSkipsInvalidTraitsAndClampsValues(t *testing.T) {
	llmClient := &llm.MockClient{
		Response: `{
			"traits": [
				{"trait": "unknown_trait", "value": 85, "confidence": 0.9},
				{"trait": "neuroticism", "value": 140, "confidence": 1.4},
				{"trait": "agreeableness", "value": -7, "confidence": -0.3}
			]
		}`,
	}
	profileRepo := &mockProfileRepo{profile: domain.CloneProfile{ID: "profile-3"}}
	traitRepo := &mockTraitRepo{}

	svc := NewAnalysisService(llmClient, traitRepo, profileRepo, zap.NewNop())
	if err := svc.AnalyzeAndPersist(context.Background(), "user-3", "texto"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if traitRepo.upsertCount != 2 {
		t.Fatalf("expected 2 valid traits persisted, got %d", traitRepo.upsertCount)
	}

	if traitRepo.allTraits[0].Trait != "neuroticism" || traitRepo.allTraits[0].Value != 100 {
		t.Fatalf("expected neuroticism value clamped to 100, got trait=%s value=%d", traitRepo.allTraits[0].Trait, traitRepo.allTraits[0].Value)
	}
	if traitRepo.allTraits[0].Confidence == nil || *traitRepo.allTraits[0].Confidence != 1 {
		t.Fatalf("expected confidence clamped to 1, got %v", traitRepo.allTraits[0].Confidence)
	}

	if traitRepo.allTraits[1].Trait != "agreeableness" || traitRepo.allTraits[1].Value != 0 {
		t.Fatalf("expected agreeableness value clamped to 0, got trait=%s value=%d", traitRepo.allTraits[1].Trait, traitRepo.allTraits[1].Value)
	}
	if traitRepo.allTraits[1].Confidence == nil || *traitRepo.allTraits[1].Confidence != 0 {
		t.Fatalf("expected confidence clamped to 0, got %v", traitRepo.allTraits[1].Confidence)
	}
}

func TestAnalysisServiceParsesWrappedJSONObject(t *testing.T) {
	llmClient := &llm.MockClient{
		Response: `Analisis:
{"traits":[{"trait":"openness","value":72,"confidence":0.6}],"emotional_intensity":18,"emotion_category":"NEUTRAL"}
fin`,
	}
	profileRepo := &mockProfileRepo{profile: domain.CloneProfile{ID: "profile-4"}}
	traitRepo := &mockTraitRepo{}

	svc := NewAnalysisService(llmClient, traitRepo, profileRepo, zap.NewNop())
	if err := svc.AnalyzeAndPersist(context.Background(), "user-4", "texto envuelto"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if traitRepo.upsertCount != 1 {
		t.Fatalf("expected upsert called once, got %d", traitRepo.upsertCount)
	}
}
