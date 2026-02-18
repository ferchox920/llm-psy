package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
)

func TestMergeDedupMemories(t *testing.T) {
	idA := uuid.New()
	idB := uuid.New()
	idC := uuid.New()

	primary := []domain.NarrativeMemory{
		{ID: idA, Content: "A"},
		{ID: idB, Content: "B"},
	}
	secondary := []domain.NarrativeMemory{
		{ID: idB, Content: "B-dup"},
		{ID: idC, Content: "C"},
	}

	got := mergeDedupMemories(primary, secondary)
	if len(got) != 3 {
		t.Fatalf("expected 3 memories, got %d", len(got))
	}
	if got[0].ID != idA || got[1].ID != idB || got[2].ID != idC {
		t.Fatalf("order mismatch, got IDs %v %v %v", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestRerankPromptContainsConflictException(t *testing.T) {
	if !strings.Contains(rerankJudgePrompt, "EXCEPCION CRITICA") {
		t.Fatalf("rerankJudgePrompt missing critical exception for conflicto reciente")
	}
	if !strings.Contains(strings.ToUpper(rerankJudgePrompt), "CONFLICTO RECIENTE") {
		t.Fatalf("rerankJudgePrompt should mention CONFLICTO RECIENTE")
	}
	if !strings.Contains(strings.ToUpper(rerankJudgePrompt), "INSULTO DIRECTO") {
		t.Fatalf("rerankJudgePrompt should mention INSULTO DIRECTO")
	}
}
func TestBuildNarrativeContext_WorkingMemoryPriorityWhenSearchEmpty(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	profileID := uuid.New()

	wmMemories := []domain.NarrativeMemory{
		{ID: uuid.New(), CloneProfileID: profileID, Content: "Conflicto reciente 1", EmotionCategory: "IRA", HappenedAt: now},
		{ID: uuid.New(), CloneProfileID: profileID, Content: "Conflicto reciente 2", EmotionCategory: "IRA", HappenedAt: now.Add(-time.Minute)},
	}

	svc := newNarrativeServiceTestHarness(wmMemories, nil)
	text, err := svc.BuildNarrativeContext(ctx, profileID, "hablar de tostadas y nubes")
	if err != nil {
		t.Fatalf("BuildNarrativeContext returned error: %v", err)
	}

	for _, content := range []string{"Conflicto reciente 1", "Conflicto reciente 2"} {
		if !strings.Contains(text, content) {
			t.Fatalf("expected context to include %q; got %q", content, text)
		}
	}
}

func TestBuildNarrativeContext_MergeDedupKeepsUnique(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	profileID := uuid.New()

	dupID := uuid.New()
	wmMemories := []domain.NarrativeMemory{
		{ID: dupID, CloneProfileID: profileID, Content: "WM dup", EmotionCategory: "IRA", HappenedAt: now},
		{ID: uuid.New(), CloneProfileID: profileID, Content: "WM only", EmotionCategory: "IRA", HappenedAt: now.Add(-time.Minute)},
	}

	searchMemories := []repository.ScoredMemory{
		{NarrativeMemory: domain.NarrativeMemory{ID: dupID, CloneProfileID: profileID, Content: "WM dup", EmotionCategory: "IRA", HappenedAt: now.Add(-2 * time.Minute)}, Similarity: 0.9, Score: 0.9},
		{NarrativeMemory: domain.NarrativeMemory{ID: uuid.New(), CloneProfileID: profileID, Content: "Search unique", EmotionCategory: "TRISTEZA", HappenedAt: now.Add(-3 * time.Minute)}, Similarity: 0.85, Score: 0.85},
	}

	svc := newNarrativeServiceTestHarness(wmMemories, searchMemories)
	text, err := svc.BuildNarrativeContext(ctx, profileID, "mensaje cualquiera")
	if err != nil {
		t.Fatalf("BuildNarrativeContext returned error: %v", err)
	}

	for _, content := range []string{"WM dup", "WM only", "Search unique"} {
		if count := strings.Count(text, content); count != 1 {
			t.Fatalf("expected content %q once, got count=%d in %q", content, count, text)
		}
	}
}

func TestBuildNarrativeContext_IgnoresLowImpactWhenNoneReturned(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	profileID := uuid.New()

	// Working memory repo fake returns empty to simulate below-threshold rows.
	wmMemories := []domain.NarrativeMemory{}
	searchMemories := []repository.ScoredMemory{
		{NarrativeMemory: domain.NarrativeMemory{ID: uuid.New(), CloneProfileID: profileID, Content: "Search kept", EmotionCategory: "ALEGRIA", HappenedAt: now}, Similarity: 0.9, Score: 0.9},
	}

	svc := newNarrativeServiceTestHarness(wmMemories, searchMemories)
	text, err := svc.BuildNarrativeContext(ctx, profileID, "mensaje normal")
	if err != nil {
		t.Fatalf("BuildNarrativeContext returned error: %v", err)
	}

	if !strings.Contains(text, "Search kept") {
		t.Fatalf("expected context to include %q; got %q", "Search kept", text)
	}
	if strings.Contains(text, "low impact") {
		t.Fatalf("unexpected low impact memory present: %q", text)
	}
}

func TestBuildNarrativeContext_SilentEvocationStillUsesWorkingMemory(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	profileID := uuid.New()

	wmMemories := []domain.NarrativeMemory{
		{ID: uuid.New(), CloneProfileID: profileID, Content: "Insulto reciente", EmotionCategory: "IRA", EmotionalIntensity: 85, HappenedAt: now},
	}
	charID := uuid.New()
	charRepo := &fakeCharacterRepo{chars: []domain.Character{
		{ID: charID, CloneProfileID: uuid.Nil, Name: "TestUser", Relationship: domain.RelationshipVectors{Trust: 50, Intimacy: 50, Respect: 50}},
	}}

	svc := newNarrativeServiceTestHarnessWithLLM(wmMemories, nil, charRepo, fakeSilentLLM{})
	text, err := svc.BuildNarrativeContext(ctx, profileID, "hola solo pasaba a saludar")
	if err != nil {
		t.Fatalf("BuildNarrativeContext returned error: %v", err)
	}
	if !strings.Contains(text, "Insulto reciente") {
		t.Fatalf("expected working memory to appear even with silent evocation; got %q", text)
	}
	if !strings.Contains(strings.ToUpper(text), "IRA") {
		t.Fatalf("expected emotion category to be present; got %q", text)
	}
}

func TestBuildNarrativeContext_StateInternalUsesNormalizedIntensity(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	profileID := uuid.New()

	wmMemories := []domain.NarrativeMemory{
		{ID: uuid.New(), CloneProfileID: profileID, Content: "Dato trivial", EmotionCategory: "NEUTRAL", EmotionalIntensity: 20, HappenedAt: now},
		// Intensidad en escala 0-10 (8 -> 80 normalizado) debe ganar sobre 20/100
		{ID: uuid.New(), CloneProfileID: profileID, Content: "Conflicto leve", EmotionCategory: "IRA", EmotionalIntensity: 8, HappenedAt: now.Add(-time.Minute)},
	}
	charID := uuid.New()
	charRepo := &fakeCharacterRepo{chars: []domain.Character{
		{ID: charID, CloneProfileID: uuid.Nil, Name: "TestUser", Relationship: domain.RelationshipVectors{Trust: 50, Intimacy: 50, Respect: 50}},
	}}

	svc := newNarrativeServiceTestHarnessWithLLM(wmMemories, nil, charRepo, fakeSilentLLM{})
	text, err := svc.BuildNarrativeContext(ctx, profileID, "hola, clima y tostadas")
	if err != nil {
		t.Fatalf("BuildNarrativeContext returned error: %v", err)
	}
	if !strings.Contains(text, "[ESTADO INTERNO]") {
		t.Fatalf("expected ESTADO INTERNO section; got %q", text)
	}
	if !strings.Contains(strings.ToUpper(text), "IRA") {
		t.Fatalf("expected IRA to be highlighted in ESTADO INTERNO; got %q", text)
	}
}

func TestBuildNarrativeContext_InternalStateDoesNotImplyPastConversation(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	profileID := uuid.New()

	wmMemories := []domain.NarrativeMemory{
		{ID: uuid.New(), CloneProfileID: profileID, Content: "Conflicto leve", EmotionCategory: "IRA", EmotionalIntensity: 8, HappenedAt: now},
	}
	charID := uuid.New()
	charRepo := &fakeCharacterRepo{chars: []domain.Character{
		{ID: charID, CloneProfileID: uuid.Nil, Name: "TestUser", Relationship: domain.RelationshipVectors{Trust: 50, Intimacy: 50, Respect: 50}},
	}}
	svc := newNarrativeServiceTestHarnessWithLLM(wmMemories, nil, charRepo, fakeSilentLLM{})
	text, err := svc.BuildNarrativeContext(ctx, profileID, "input trivial")
	if err != nil {
		t.Fatalf("BuildNarrativeContext returned error: %v", err)
	}
	if !strings.Contains(text, "[ESTADO INTERNO]") {
		t.Fatalf("expected ESTADO INTERNO section; got %q", text)
	}
	if strings.Contains(strings.ToLower(text), "intercambio anterior") || strings.Contains(strings.ToLower(text), "conflicto reciente") {
		t.Fatalf("internal state should not imply prior conversation; got %q", text)
	}
}

func TestBuildNarrativeContext_NotConfigured(t *testing.T) {
	var svc *NarrativeService
	_, err := svc.BuildNarrativeContext(context.Background(), uuid.New(), "hola")
	if !errors.Is(err, ErrNarrativeServiceNotConfigured) {
		t.Fatalf("expected ErrNarrativeServiceNotConfigured, got %v", err)
	}
}

func TestBuildNarrativeContext_InvalidProfileID(t *testing.T) {
	svc := newNarrativeServiceTestHarness(nil, nil)
	_, err := svc.BuildNarrativeContext(context.Background(), uuid.Nil, "hola")
	if !errors.Is(err, ErrNarrativeInvalidInput) {
		t.Fatalf("expected ErrNarrativeInvalidInput, got %v", err)
	}
}

// --- fakes ---

type fakeLLM struct{}

func (f fakeLLM) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return []float32{1.0, 0.0}, nil
}
func (f fakeLLM) Generate(ctx context.Context, prompt string) (string, error) {
	// Simple echo keeps evocation non-empty and judge "use": true
	if strings.Contains(prompt, `"use"`) {
		return `{"use": true, "reason": "ok"}`, nil
	}
	return "evocacion simple", nil
}

type fakeCharacterRepo struct {
	chars []domain.Character
}

func (f *fakeCharacterRepo) Create(ctx context.Context, character domain.Character) error {

	f.chars = append(f.chars, character)
	return nil
}

func (f *fakeCharacterRepo) Update(ctx context.Context, character domain.Character) error { return nil }

func (f *fakeCharacterRepo) ListByProfileID(ctx context.Context, profileID uuid.UUID) ([]domain.Character, error) {
	return f.chars, nil
}

func (f *fakeCharacterRepo) FindByName(ctx context.Context, profileID uuid.UUID, name string) (*domain.Character, error) {
	for i := range f.chars {
		if f.chars[i].CloneProfileID == profileID && strings.EqualFold(f.chars[i].Name, name) {
			return &f.chars[i], nil
		}
	}
	return nil, nil
}

type fakeMemoryRepo struct {
	wm     []domain.NarrativeMemory
	search []repository.ScoredMemory
}

func (f fakeMemoryRepo) Create(ctx context.Context, memory domain.NarrativeMemory) error { return nil }

func (f fakeMemoryRepo) Search(ctx context.Context, profileID uuid.UUID, queryEmbedding pgvector.Vector, k int, emotionalWeightFactor float64) ([]repository.ScoredMemory, error) {
	return f.search, nil
}

func (f fakeMemoryRepo) ListByCharacter(ctx context.Context, characterID uuid.UUID) ([]domain.NarrativeMemory, error) {
	return nil, nil
}

func (f fakeMemoryRepo) GetRecentHighImpactByProfile(ctx context.Context, profileID uuid.UUID, limit int, minImportance int, minEmotionalIntensity int) ([]domain.NarrativeMemory, error) {
	return f.wm, nil
}

func newNarrativeServiceTestHarness(wm []domain.NarrativeMemory, search []repository.ScoredMemory) *NarrativeService {
	charID := uuid.New()
	charRepo := &fakeCharacterRepo{chars: []domain.Character{
		{ID: charID, CloneProfileID: uuid.Nil, Name: "TestUser", Relationship: domain.RelationshipVectors{Trust: 50, Intimacy: 50, Respect: 50}},
	}}

	return newNarrativeServiceTestHarnessWithLLM(wm, search, charRepo, fakeLLM{})
}

func newNarrativeServiceTestHarnessWithLLM(wm []domain.NarrativeMemory, search []repository.ScoredMemory, charRepo *fakeCharacterRepo, llm llmClientWithEmbedding) *NarrativeService {
	return &NarrativeService{
		characterRepo: charRepo,
		memoryRepo:    fakeMemoryRepo{wm: wm, search: search},
		llmClient:     llm,
	}
}

type fakeSilentLLM struct{}

func (f fakeSilentLLM) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return []float32{1, 0}, nil
}
func (f fakeSilentLLM) Generate(ctx context.Context, prompt string) (string, error) {
	// Si es el juez, devolvemos use=true; evocation queda vacia para simular silencio.
	if strings.Contains(prompt, `"use"`) {
		return `{"use": true, "reason": "ok"}`, nil
	}
	return "", nil
}
