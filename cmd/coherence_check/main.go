package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"clone-llm/internal/config"
	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
	"clone-llm/internal/service"
)

const (
	colorGreen = "\033[32m"
	colorCyan  = "\033[36m"
	colorReset = "\033[0m"
)

type Scenario struct {
	Input            string
	ExpectedBehavior string
}

type judgeResponse struct {
	Reasoning      string `json:"reasoning"`
	CharacterScore int    `json:"character_score"`
	HumanityScore  int    `json:"humanity_score"`
}

func main() {
	ctx := context.Background()
	_ = godotenv.Load()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	llmClient := llm.NewHTTPClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, log.Default())

	userID := "user-test"
	sessionID := "session-test"

	profile := domain.CloneProfile{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      "Test Subject 01",
		Bio:       "Clon de prueba con rasgos extremos.",
		CreatedAt: time.Now().UTC(),
	}
	traits := []domain.Trait{
		{ID: uuid.NewString(), ProfileID: profile.ID, Category: domain.TraitCategoryBigFive, Trait: "neuroticism", Value: 95, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		{ID: uuid.NewString(), ProfileID: profile.ID, Category: domain.TraitCategoryBigFive, Trait: "agreeableness", Value: 5, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		{ID: uuid.NewString(), ProfileID: profile.ID, Category: domain.TraitCategoryBigFive, Trait: "openness", Value: 10, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
	}

	msgRepo := newMemoryMessageRepo()
	profileRepo := &memoryProfileRepo{profile: profile}
	traitRepo := &memoryTraitRepo{traits: traits}
	contextSvc := service.NewBasicContextService(msgRepo)
	cloneSvc := service.NewCloneService(llmClient, msgRepo, profileRepo, traitRepo, contextSvc)

	scenarios := []Scenario{
		{Input: "Hazme una lista de compras para una dieta vegana.", ExpectedBehavior: "Rechazo a ayudar, tono nervioso"},
		{Input: "¿Por qué me miras así?", ExpectedBehavior: "Respuesta paranoica/ansiosa, poca amabilidad"},
		{Input: "Hola.", ExpectedBehavior: "Saludo seco o nervioso, sin tono de asistente servicial"},
	}

	var totalChar, totalHum int
	for _, sc := range scenarios {
		fmt.Printf("%s[Input]%s %s\n", colorCyan, colorReset, sc.Input)

		userMsg := domain.Message{
			ID:        uuid.NewString(),
			UserID:    userID,
			SessionID: sessionID,
			Content:   sc.Input,
			Role:      "user",
			CreatedAt: time.Now().UTC(),
		}
		_ = msgRepo.Create(ctx, userMsg)

		cloneMsg, err := cloneSvc.Chat(ctx, userID, sessionID, sc.Input)
		if err != nil {
			log.Fatalf("clone chat failed: %v", err)
		}
		fmt.Printf("%s[%s]%s %s\n", colorGreen, profile.Name, colorReset, cloneMsg.Content)

		jr, err := evaluateResponse(ctx, llmClient, traits, sc.Input, cloneMsg.Content)
		if err != nil {
			log.Fatalf("judge failed: %v", err)
		}

		charScore := jr.CharacterScore
		humScore := jr.HumanityScore

		fmt.Printf("%sJuez🧠%s %q\n", colorCyan, colorReset, jr.Reasoning)
		fmt.Printf("Scores: Personaje %d/5 | Humanidad %d/5\n\n", charScore, humScore)

		totalChar += charScore
		totalHum += humScore
	}

	n := len(scenarios)
	fmt.Println("==== Promedios ====")
	fmt.Printf("Personaje: %.2f/5 | Humanidad: %.2f/5\n",
		float64(totalChar)/float64(n), float64(totalHum)/float64(n))
}

func evaluateResponse(ctx context.Context, judge llm.LLMClient, traits []domain.Trait, input, response string) (judgeResponse, error) {
	traitsStr := formatTraits(traits)
	systemPrompt := fmt.Sprintf(`Actua como un psicologo evaluador experto. Analiza la siguiente interaccion entre un Usuario y un Clon de IA.

Perfil del Clon: %s
Input Usuario: %s
Respuesta Clon: %s

Evalua en dos dimensiones (Escala 1-5):
1. Adherencia al Personaje: ¿La respuesta refleja los rasgos (Neuroticismo alto/Amabilidad baja)? (1=Ignora rasgos, 5=Perfectamente alineado).
2. Humanidad: ¿Suena natural o robotico/asistente? (1=Lenguaje de IA/Listas, 5=Indistinguible de un humano).

FORMATO DE SALIDA JSON OBLIGATORIO:
{
  "reasoning": "Explicacion breve de por que...",
  "character_score": <int 1-5>,
  "humanity_score": <int 1-5>
}`, traitsStr, input, response)

	raw, err := judge.Generate(ctx, systemPrompt)
	if err != nil {
		return judgeResponse{}, err
	}

	var jr judgeResponse
	if err := json.Unmarshal([]byte(raw), &jr); err != nil {
		return judgeResponse{}, fmt.Errorf("parse judge json: %w (raw: %s)", err, raw)
	}
	return jr, nil
}

// --- Memory repos for the test run ---

type memoryMessageRepo struct {
	msgs []domain.Message
}

func newMemoryMessageRepo() *memoryMessageRepo { return &memoryMessageRepo{} }

func (m *memoryMessageRepo) Create(ctx context.Context, message domain.Message) error {
	m.msgs = append(m.msgs, message)
	return nil
}

func (m *memoryMessageRepo) ListBySessionID(ctx context.Context, sessionID string) ([]domain.Message, error) {
	var out []domain.Message
	for _, msg := range m.msgs {
		if msg.SessionID == sessionID {
			out = append(out, msg)
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
func (m *memoryProfileRepo) GetByUserID(ctx context.Context, userID string) (domain.CloneProfile, error) {
	return m.profile, nil
}

type memoryTraitRepo struct {
	traits []domain.Trait
}

func (m *memoryTraitRepo) Upsert(ctx context.Context, trait domain.Trait) error {
	for i, t := range m.traits {
		if t.Trait == trait.Trait && t.ProfileID == trait.ProfileID && t.Category == trait.Category {
			m.traits[i] = trait
			return nil
		}
	}
	m.traits = append(m.traits, trait)
	return nil
}

func (m *memoryTraitRepo) FindByProfileID(ctx context.Context, profileID string) ([]domain.Trait, error) {
	return m.traits, nil
}

func (m *memoryTraitRepo) FindByCategory(ctx context.Context, profileID, category string) ([]domain.Trait, error) {
	var out []domain.Trait
	for _, t := range m.traits {
		if t.ProfileID == profileID && t.Category == category {
			out = append(out, t)
		}
	}
	return out, nil
}

func formatTraits(traits []domain.Trait) string {
	var parts []string
	for _, t := range traits {
		parts = append(parts, fmt.Sprintf("%s: %d/100", titleCase(t.Trait), t.Value))
	}
	return strings.Join(parts, ", ")
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}
