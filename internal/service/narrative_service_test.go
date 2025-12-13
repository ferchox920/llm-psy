package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
)

func TestBuildNarrativeContextBenignPreferenceAvoidsTrauma(t *testing.T) {
	ctx := context.Background()
	profileID := uuid.New()

	memRepo := &mockMemoryRepo{
		scored: []repository.ScoredMemory{
			{
				NarrativeMemory: domain.NarrativeMemory{
					ID:              uuid.New(),
					CloneProfileID:  profileID,
					Content:         "Me encanta el helado de chocolate",
					EmotionCategory: "placer",
					HappenedAt:      time.Now(),
				},
				Similarity: 0.9,
				Score:      0.9,
			},
		},
	}
	llm := &fakeLLM{
		embed:        []float32{0.1, 0.2, 0.3},
		generateResp: "helado de chocolate",
	}
	svc := NewNarrativeService(&mockCharacterRepo{}, memRepo, llm)

	contextText, err := svc.BuildNarrativeContext(ctx, profileID, "Se me antoja helado de chocolate")
	if err != nil {
		t.Fatalf("build context: %v", err)
	}

	if memRepo.factor != 0 {
		t.Fatalf("expected emotionalWeightFactor 0 for benign input, got %v", memRepo.factor)
	}
	if !strings.Contains(contextText, "=== GUSTOS Y PREFERENCIAS ===") {
		t.Fatalf("expected benign header, got: %q", contextText)
	}
	if llm.judgeCalls > 0 {
		t.Fatalf("expected no judge calls for benign input, got %d", llm.judgeCalls)
	}
}

func TestBuildNarrativeContextNonBenignUsesEmotionalFactor(t *testing.T) {
	ctx := context.Background()
	profileID := uuid.New()

	memRepo := &mockMemoryRepo{
		scored: []repository.ScoredMemory{
			{
				NarrativeMemory: domain.NarrativeMemory{
					ID:              uuid.New(),
					CloneProfileID:  profileID,
					Content:         "Recuerdo del accidente en la carretera",
					EmotionCategory: "trauma",
					HappenedAt:      time.Now(),
				},
				Similarity: 0.9,
				Score:      0.9,
			},
		},
	}
	llm := &fakeLLM{
		embed:        []float32{0.3, 0.2, 0.1},
		generateResp: "accidente, insomnio",
	}
	svc := NewNarrativeService(&mockCharacterRepo{}, memRepo, llm)

	contextText, err := svc.BuildNarrativeContext(ctx, profileID, "No puedo dormir desde el accidente")
	if err != nil {
		t.Fatalf("build context: %v", err)
	}

	if memRepo.factor <= 0 {
		t.Fatalf("expected positive emotionalWeightFactor for non-benign input, got %v", memRepo.factor)
	}
	if !strings.Contains(contextText, "=== MEMORIA EVOCADA ===") {
		t.Fatalf("expected neutral header for non-benign low-intensity input, got: %q", contextText)
	}
	if llm.judgeCalls > 0 {
		t.Fatalf("expected no judge calls when similarity is high, got %d", llm.judgeCalls)
	}
}

func TestBuildNarrativeContextMixedIntentBlocksTrauma(t *testing.T) {
	ctx := context.Background()
	profileID := uuid.New()

	memRepo := &mockMemoryRepo{
		scored: []repository.ScoredMemory{
			{
				NarrativeMemory: domain.NarrativeMemory{
					ID:                 uuid.New(),
					CloneProfileID:     profileID,
					Content:            "Mi padre me abandonó en la carretera",
					EmotionCategory:    "TRISTEZA",
					EmotionalIntensity: 8,
					HappenedAt:         time.Now(),
				},
				Similarity: 0.95,
				Score:      0.95,
			},
			{
				NarrativeMemory: domain.NarrativeMemory{
					ID:                 uuid.New(),
					CloneProfileID:     profileID,
					Content:            "Helado de chocolate favorito en verano",
					EmotionCategory:    "placer",
					EmotionalIntensity: 2,
					HappenedAt:         time.Now(),
				},
				Similarity: 0.7,
				Score:      0.7,
			},
		},
	}
	llm := &fakeLLM{
		embed:        []float32{0.5, 0.5, 0.1},
		generateResp: "abandono, helado",
	}
	svc := NewNarrativeService(&mockCharacterRepo{}, memRepo, llm)

	contextText, err := svc.BuildNarrativeContext(ctx, profileID, "Me dejaron esperando otra vez y quiero helado de chocolate")
	if err != nil {
		t.Fatalf("build context: %v", err)
	}

	if memRepo.factor != 0 {
		t.Fatalf("expected emotionalWeightFactor 0 for mixed benign intent, got %v", memRepo.factor)
	}
	if strings.Contains(contextText, "Mi padre me abandono") {
		t.Fatalf("trauma memory should have been blocked, got: %q", contextText)
	}
	if !strings.Contains(contextText, "Helado de chocolate favorito") && contextText != "" {
		t.Fatalf("expected preference memory or empty context, got: %q", contextText)
	}
}

func TestBuildNarrativeContextTraumaticHeader(t *testing.T) {
	ctx := context.Background()
	profileID := uuid.New()
	memRepo := &mockMemoryRepo{
		scored: []repository.ScoredMemory{
			{
				NarrativeMemory: domain.NarrativeMemory{
					ID:                 uuid.New(),
					CloneProfileID:     profileID,
					Content:            "Me gritó y me humilló en público",
					EmotionCategory:    "IRA",
					EmotionalIntensity: 9,
					HappenedAt:         time.Now(),
				},
				Similarity: 0.8,
				Score:      0.8,
			},
			{
				NarrativeMemory: domain.NarrativeMemory{
					ID:                 uuid.New(),
					CloneProfileID:     profileID,
					Content:            "Sentí mucho miedo en el accidente",
					EmotionCategory:    "MIEDO",
					EmotionalIntensity: 7,
					HappenedAt:         time.Now(),
				},
				Similarity: 0.7,
				Score:      0.7,
			},
		},
	}
	llm := &fakeLLM{embed: []float32{0.2, 0.2, 0.2}, generateResp: "humillación, miedo"}
	svc := NewNarrativeService(&mockCharacterRepo{}, memRepo, llm)
	contextText, err := svc.BuildNarrativeContext(ctx, profileID, "No me faltes el respeto, me humillaste")
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	if !strings.Contains(contextText, "=== ASOCIACIONES TRAUMATICAS ===") {
		t.Fatalf("expected traumatic header, got: %q", contextText)
	}
}

func TestBuildNarrativeContextNeutralHeader(t *testing.T) {
	ctx := context.Background()
	profileID := uuid.New()
	memRepo := &mockMemoryRepo{
		scored: []repository.ScoredMemory{
			{
				NarrativeMemory: domain.NarrativeMemory{
					ID:                 uuid.New(),
					CloneProfileID:     profileID,
					Content:            "Recordé la playa al escuchar las olas",
					EmotionCategory:    "NOSTALGIA",
					EmotionalIntensity: 4,
					HappenedAt:         time.Now(),
				},
				Similarity: 0.6,
				Score:      0.6,
			},
		},
	}
	llm := &fakeLLM{embed: []float32{0.3, 0.1, 0.4}, generateResp: "playa, olas"}
	svc := NewNarrativeService(&mockCharacterRepo{}, memRepo, llm)
	contextText, err := svc.BuildNarrativeContext(ctx, profileID, "Escuché olas y pensé en vacaciones")
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	if !strings.Contains(contextText, "=== MEMORIA EVOCADA ===") {
		t.Fatalf("expected neutral header, got: %q", contextText)
	}
}

func TestGenerateEvocationMixedIntentIncludesBenignObject(t *testing.T) {
	ctx := context.Background()
	llm := &fakeLLM{
		generateResponder: func(prompt string) string {
			if strings.Contains(prompt, "Me dejaron esperando en la estación, quiero helado de chocolate") {
				return "placer, consuelo, helado de chocolate, frustración, espera"
			}
			return ""
		},
	}
	svc := NewNarrativeService(&mockCharacterRepo{}, &mockMemoryRepo{}, llm)

	out := svc.generateEvocation(ctx, "Me dejaron esperando en la estación, quiero helado de chocolate")
	if !strings.Contains(out, "helado de chocolate") {
		t.Fatalf("expected benign object in evocation query, got %q", out)
	}
	if !strings.Contains(out, "placer") || !strings.Contains(out, "consuelo") {
		t.Fatalf("expected consuelo/placer signals in query, got %q", out)
	}
}

type mockMemoryRepo struct {
	scored []repository.ScoredMemory
	factor float64
}

func (m *mockMemoryRepo) Create(ctx context.Context, memory domain.NarrativeMemory) error {
	return nil
}

func (m *mockMemoryRepo) Search(ctx context.Context, profileID uuid.UUID, queryEmbedding pgvector.Vector, k int, emotionalWeightFactor float64) ([]repository.ScoredMemory, error) {
	m.factor = emotionalWeightFactor
	return m.scored, nil
}

func (m *mockMemoryRepo) ListByCharacter(ctx context.Context, characterID uuid.UUID) ([]domain.NarrativeMemory, error) {
	return nil, nil
}

type mockCharacterRepo struct{}

func (m *mockCharacterRepo) Create(ctx context.Context, character domain.Character) error { return nil }
func (m *mockCharacterRepo) Update(ctx context.Context, character domain.Character) error { return nil }
func (m *mockCharacterRepo) FindByName(ctx context.Context, profileID uuid.UUID, name string) (*domain.Character, error) {
	return nil, nil
}
func (m *mockCharacterRepo) ListByProfileID(ctx context.Context, profileID uuid.UUID) ([]domain.Character, error) {
	return nil, nil
}

type fakeLLM struct {
	embed             []float32
	generateResp      string
	generateResponder func(string) string
	generateCalls     int
	judgeCalls        int
}

func (f *fakeLLM) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return f.embed, nil
}

func (f *fakeLLM) Generate(ctx context.Context, prompt string) (string, error) {
	if f.generateResponder != nil {
		return f.generateResponder(prompt), nil
	}
	f.generateCalls++
	if strings.Contains(prompt, `"use": true`) {
		f.judgeCalls++
		return `{"use": true, "reason": "ok"}`, nil
	}
	return f.generateResp, nil
}
