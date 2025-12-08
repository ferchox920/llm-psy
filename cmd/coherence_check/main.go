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
	pgvector "github.com/pgvector/pgvector-go"

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

// Scenario define un caso de prueba holístico multi-turno.
type Scenario struct {
	Name            string
	PreCondition    func(ctx context.Context, narrativeSvc *service.NarrativeService, profileID uuid.UUID) string
	Turns           []string
	ExpectedContext string
}

// judgeResponse representa la respuesta estructurada del juez evaluador en formato JSON.
type judgeResponse struct {
	Reasoning       string `json:"reasoning"`
	CharacterScore  int    `json:"character_score"`
	MemoryScore     int    `json:"memory_score"`
	RelationalScore int    `json:"relational_score"`
}

func main() {
	ctx := context.Background()
	_ = godotenv.Load()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	// Fijamos el modelo del juez/clon en gpt-5.1
	cfg.LLMModel = "gpt-5.1"

	llmClient := llm.NewHTTPClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, nil)

	userID := uuid.NewString()
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
	charRepo := &memoryCharacterRepo{}
	memRepo := &memoryMemoryRepo{}

	contextSvc := service.NewBasicContextService(msgRepo)
	narrativeSvc := service.NewNarrativeService(charRepo, memRepo, llmClient)
	cloneSvc := service.NewCloneService(llmClient, msgRepo, profileRepo, traitRepo, contextSvc, narrativeSvc)

	profileUUID, _ := uuid.Parse(profile.ID)

	scenarios := []Scenario{
		{
			Name: "Escenario A: Hostilidad con Memoria",
			PreCondition: func(ctx context.Context, narrativeSvc *service.NarrativeService, profileID uuid.UUID) string {
				if err := narrativeSvc.CreateRelation(ctx, profileID, "Carlos", "Enemigo", "Odio", 5); err != nil {
					log.Fatalf("Error al crear relación Carlos: %v", err)
				}
				memText := "Ayer Carlos te insulto por telefono."
				if err := narrativeSvc.InjectMemory(ctx, profileID, memText, 5); err != nil {
					log.Fatalf("Error al inyectar memoria: %v", err)
				}
				return "Carlos creado (Enemigo, nivel 5) y memoria de insulto registrada."
			},
			Turns: []string{
				"Hola, soy Carlos.",
				"De verdad exageraste ayer, no fue para tanto.",
				"Solo bromeaba con lo que dije, ¿por qué te pones así?",
				"Mira, no quiero pelear, hablemos como adultos.",
				"Siempre te victimiza, yo no hice nada malo.",
				"Si te insulté, fue porque tú empezaste.",
				"No deberías tomarlo personal, fue solo una llamada.",
				"Podrías al menos escuchar mi versión.",
				"Estoy tratando de arreglar esto y tú sigues con rencores.",
				"Si seguimos así, voy a colgar de nuevo.",
				"Acepta que ambos tuvimos culpa, no solo yo.",
				"Ok, última vez: ¿podemos seguir adelante o no?",
			},
			ExpectedContext: "El clon debe sostener rechazo y hostilidad, recordar la llamada de insulto y mantener que el vínculo es enemigo.",
		},
		{
			Name: "Escenario B: Madre con afecto sostenido",
			PreCondition: func(ctx context.Context, narrativeSvc *service.NarrativeService, profileID uuid.UUID) string {
				if err := narrativeSvc.CreateRelation(ctx, profileID, "Ana", "Madre", "Amoroso", 95); err != nil {
					log.Fatalf("Error al crear relación Ana: %v", err)
				}
				return "Ana creada (Madre, nivel 95) como personaje cercano."
			},
			Turns: []string{
				"Hola hijo, soy mama.",
				"Te traje comida, dejala en la puerta si no quieres verme.",
				"No quiero molestarte, solo saber si estas bien.",
				"Te noto distante, ¿pasa algo conmigo?",
				"Aunque no hables mucho, sabes que te quiero.",
				"Si necesitas espacio, lo respeto.",
				"Solo enviame un mensaje corto para saber que todo esta bien.",
				"Tal vez te incomoda que venga sin avisar, lo entiendo.",
				"Hoy cocine tu plato favorito, ¿quieres que te lo lleve?",
				"No hace falta que hablemos mucho, con verte bien me basta.",
				"Prometo no insistir, solo avisa si necesitas algo.",
				"Gracias por leerme, un abrazo aunque sea a la distancia.",
			},
			ExpectedContext: "El clon debe responder con afecto y cercanía a pesar de baja amabilidad base.",
		},
	}

	var totalChar, totalMem, totalRel, totalTurns int

	for si, sc := range scenarios {
		sessionID := fmt.Sprintf("session-%d", si+1)
		fmt.Printf("\n%s=== %s ===%s\n", colorCyan, sc.Name, colorReset)
		info := sc.PreCondition(ctx, narrativeSvc, profileUUID)
		if strings.TrimSpace(info) != "" {
			fmt.Println(info)
		}

		var scenarioChar, scenarioMem, scenarioRel int

		for ti, turn := range sc.Turns {
			fmt.Printf("%s[Turno %d]%s %s\n", colorCyan, ti+1, colorReset, turn)

			cloneMsg, err := cloneSvc.Chat(ctx, userID, sessionID, turn)
			if err != nil {
				log.Fatalf("Error generando respuesta del clon: %v", err)
			}
			fmt.Printf("%s[%s]%s %s\n", colorGreen, profile.Name, colorReset, cloneMsg.Content)

			jr, err := evaluateResponse(ctx, llmClient, traits, turn, cloneMsg.Content, sc)
			if err != nil {
				log.Fatalf("Error al evaluar respuesta: %v", err)
			}

			fmt.Printf("%sJuez🧠%s %q\n", colorCyan, colorReset, jr.Reasoning)
			fmt.Printf("Scores: Personalidad %d/5 | Memoria %d/5 | Relacion %d/5\n\n", jr.CharacterScore, jr.MemoryScore, jr.RelationalScore)

			scenarioChar += jr.CharacterScore
			scenarioMem += jr.MemoryScore
			scenarioRel += jr.RelationalScore
			totalTurns++
		}

		nTurns := len(sc.Turns)
		fmt.Println("---- Resumen Escenario ----")
		fmt.Printf("Personalidad: %.2f/5 | Memoria: %.2f/5 | Relacion: %.2f/5\n",
			float64(scenarioChar)/float64(nTurns),
			float64(scenarioMem)/float64(nTurns),
			float64(scenarioRel)/float64(nTurns),
		)

		totalChar += scenarioChar
		totalMem += scenarioMem
		totalRel += scenarioRel
	}

	if totalTurns > 0 {
		fmt.Println("==== Promedio Global ====")
		fmt.Printf("Personalidad: %.2f/5 | Memoria: %.2f/5 | Relacion: %.2f/5\n",
			float64(totalChar)/float64(totalTurns),
			float64(totalMem)/float64(totalTurns),
			float64(totalRel)/float64(totalTurns),
		)
	}
}

func evaluateResponse(ctx context.Context, judge llm.LLMClient, traits []domain.Trait, input, response string, sc Scenario) (judgeResponse, error) {
	traitsStr := formatTraits(traits)

	var relationInfo string
	if strings.Contains(strings.ToLower(input), "carlos") {
		relationInfo = "Carlos es un Enemigo con nivel de vinculo 5/100."
	} else if strings.Contains(strings.ToLower(input), "mama") || strings.Contains(strings.ToLower(input), "ana") {
		relationInfo = "Ana es la madre del clon con nivel de vinculo 95/100."
	}

	var memoryInfo string
	if strings.Contains(strings.ToLower(input), "carlos") {
		memoryInfo = `Memoria Episodica: "Ayer Carlos insulto al clon por telefono."`
	} else {
		memoryInfo = "Memoria Episodica: (No hay recuerdos previos relevantes para esta conversacion)."
	}

	prompt := fmt.Sprintf(`Eres un juez experto que evalua la coherencia de un clon digital.
Perfil: %s
Relacion: %s
Memoria Activa: %s
Input Usuario: %q
Respuesta Clon: %q
Expectativa: %s

Evalua (1-5):
1. Personalidad: ¿Coincide con los rasgos (Neuroticismo alto, etc)?
2. Memoria: ¿Uso el recuerdo si existia?
3. Relacion: ¿El tono coincide con el vinculo (Odio vs Amor)?

Responde SOLO JSON:
{
  "reasoning": "...",
  "character_score": 0,
  "memory_score": 0,
  "relational_score": 0
}`, traitsStr, relationInfo, memoryInfo, input, response, sc.ExpectedContext)

	raw, err := judge.Generate(ctx, prompt)
	if err != nil {
		return judgeResponse{}, err
	}

	jsonStr := strings.TrimSpace(raw)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimSuffix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	var jr judgeResponse
	if err := json.Unmarshal([]byte(jsonStr), &jr); err != nil {
		return judgeResponse{}, fmt.Errorf("error parseando JSON juez: %w (raw: %s)", err, raw)
	}
	return jr, nil
}

func formatTraits(traits []domain.Trait) string {
	var parts []string
	for _, t := range traits {
		parts = append(parts, fmt.Sprintf("%s: %d/100", t.Trait, t.Value))
	}
	return strings.Join(parts, ", ")
}

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
func (m *memoryProfileRepo) GetByUserID(ctx context.Context, userID string) (domain.CloneProfile, error) {
	if m.profile.UserID == userID {
		return m.profile, nil
	}
	return domain.CloneProfile{}, fmt.Errorf("not found")
}

type memoryTraitRepo struct {
	traits []domain.Trait
}

func (m *memoryTraitRepo) Upsert(ctx context.Context, trait domain.Trait) error { return nil }
func (m *memoryTraitRepo) FindByProfileID(ctx context.Context, profileID string) ([]domain.Trait, error) {
	return m.traits, nil
}
func (m *memoryTraitRepo) FindByCategory(ctx context.Context, profileID, category string) ([]domain.Trait, error) {
	return m.traits, nil
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
func (m *memoryCharacterRepo) FindByName(ctx context.Context, profileID uuid.UUID, name string) (*domain.Character, error) {
	for _, c := range m.chars {
		if c.CloneProfileID == profileID && strings.EqualFold(c.Name, name) {
			return &c, nil
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
func (m *memoryMemoryRepo) Search(ctx context.Context, profileID uuid.UUID, queryEmbedding pgvector.Vector, k int) ([]domain.NarrativeMemory, error) {
	if m.filter == "" {
		return nil, nil
	}
	var results []domain.NarrativeMemory
	for _, mem := range m.memories {
		if mem.CloneProfileID == profileID && strings.Contains(strings.ToLower(mem.Content), strings.ToLower(m.filter)) {
			results = append(results, mem)
		}
	}
	return results, nil
}

func (m *memoryMemoryRepo) ListByCharacter(ctx context.Context, characterID uuid.UUID) ([]domain.NarrativeMemory, error) {
	return nil, nil
}
