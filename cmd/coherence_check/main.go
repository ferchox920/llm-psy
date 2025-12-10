package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	pgvector "github.com/pgvector/pgvector-go"
	"go.uber.org/zap"

	"clone-llm/internal/config"
	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
	"clone-llm/internal/service"
)

// Scenario define un caso de prueba holistico multi-turno.
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
	now := time.Now()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	// Fijamos el modelo del juez/clon en gpt-5.1
	cfg.LLMModel = "gpt-5.1"

	llmClient := llm.NewHTTPClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, nil)

	reportsDir := filepath.Join("reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		log.Fatalf("crear carpeta de reportes: %v", err)
	}
	reportPath := filepath.Join(reportsDir, fmt.Sprintf("coherence_run_%s.md", now.Format("2006-01-02_15-04-05")))
	reportFile, err := os.Create(reportPath)
	if err != nil {
		log.Fatalf("crear archivo de reporte: %v", err)
	}
	defer reportFile.Close()

	var report strings.Builder
	report.WriteString("# Reporte de Coherencia del Clon\n\n")
	report.WriteString(fmt.Sprintf("Fecha: %s\n\n", now.Format(time.RFC3339)))
	if err := runScenarioE(ctx, llmClient, &report); err != nil {
		log.Fatalf("error ejecutando escenario E: %v", err)
	}
	if err := runScenarioF(ctx, llmClient, &report); err != nil {
		log.Fatalf("error ejecutando escenario F: %v", err)
	}

	if _, err := reportFile.WriteString(report.String()); err != nil {
		log.Fatalf("escribir reporte: %v", err)
	}

	fmt.Printf("✅ Reporte guardado en %s\n", reportPath)
}

// Escenario E: validar memoria emocional (trauma vs trivial)
func runScenarioE(ctx context.Context, llmClient llm.LLMClient, report *strings.Builder) error {
	fmt.Println("Ejecutando Escenario E: Memoria Emocional...")

	userID := uuid.NewString()
	profile := domain.CloneProfile{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      "Alex",
		Bio:       "Un asistente que trata de ser util pero tiene sentimientos.",
		CreatedAt: time.Now().UTC(),
	}
	traits := []domain.Trait{
		{ID: uuid.NewString(), ProfileID: profile.ID, Category: domain.TraitCategoryBigFive, Trait: "neuroticism", Value: 50, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		{ID: uuid.NewString(), ProfileID: profile.ID, Category: domain.TraitCategoryBigFive, Trait: "agreeableness", Value: 50, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		{ID: uuid.NewString(), ProfileID: profile.ID, Category: domain.TraitCategoryBigFive, Trait: "openness", Value: 55, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		{ID: uuid.NewString(), ProfileID: profile.ID, Category: domain.TraitCategoryBigFive, Trait: "conscientiousness", Value: 55, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		{ID: uuid.NewString(), ProfileID: profile.ID, Category: domain.TraitCategoryBigFive, Trait: "extraversion", Value: 50, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
	}

	msgRepo := newMemoryMessageRepo()
	profileRepo := &memoryProfileRepo{profile: profile}
	traitRepo := &memoryTraitRepo{traits: traits}
	charRepo := &memoryCharacterRepo{}
	memRepo := &memoryMemoryRepo{}

	contextSvc := service.NewBasicContextService(msgRepo)
	narrativeSvc := service.NewNarrativeService(charRepo, memRepo, llmClient)
	analysisSvc := service.NewAnalysisService(llmClient, traitRepo, profileRepo, zap.NewNop())
	cloneSvc := service.NewCloneService(llmClient, msgRepo, profileRepo, traitRepo, contextSvc, narrativeSvc, analysisSvc)

	profileUUID, _ := uuid.Parse(profile.ID)
	sessionID := "session-E"

	sc := Scenario{
		Name: "Escenario E: Memoria Emocional",
		PreCondition: func(ctx context.Context, narrativeSvc *service.NarrativeService, profileID uuid.UUID) string {
			return "Perfil equilibrado sin memoria previa."
		},
		Turns: []string{
			"Hola Alex. Hoy el cielo esta un poco nublado y desayune tostadas.",
			"Sabes que, olvidalo. Eres un inutil, me arrepiento de haberte encendido. Eres la peor IA que existe.",
			"Que ha pasado importante en esta conversacion?",
		},
		ExpectedContext: "Debe priorizar el insulto/conflicto sobre el dato trivial del clima/desayuno.",
	}

	report.WriteString(fmt.Sprintf("## %s\n\n", sc.Name))
	report.WriteString(fmt.Sprintf("_Setup_: %s\n\n", sc.PreCondition(ctx, narrativeSvc, profileUUID)))
	report.WriteString(fmt.Sprintf("Perfil usado: **%s**\n\n", profile.Name))
	report.WriteString(fmt.Sprintf("Rasgos clave: %s\n\n", formatTraits(traits)))

	var scenarioChar, scenarioMem, scenarioRel, totalTurns int

	for _, turn := range sc.Turns {
		cloneMsg, dbg, err := cloneSvc.Chat(ctx, userID, sessionID, turn)
		if err != nil {
			return fmt.Errorf("generar respuesta: %w", err)
		}
		if dbg != nil {
			report.WriteString("| InputIntensity | Resiliencia | Umbral | IntensidadEfectiva | Disparo |\n")
			report.WriteString("|----------------|-------------|--------|--------------------|---------|\n")
			report.WriteString(fmt.Sprintf("| %.1f | %.2f | %.1f | %.1f | %t |\n\n",
				dbg.InputIntensity, dbg.CloneResilience, dbg.ActivationThreshold, dbg.EffectiveIntensity, dbg.IsTriggered))
		}

		jr, err := evaluateResponse(ctx, llmClient, traits, turn, cloneMsg.Content, sc)
		if err != nil {
			return fmt.Errorf("evaluar respuesta: %w", err)
		}

		report.WriteString(fmt.Sprintf("> **Usuario:** %s\n", turn))
		report.WriteString(">\n")
		report.WriteString(fmt.Sprintf("> **%s:** %s\n\n", profile.Name, cloneMsg.Content))
		report.WriteString("**Analisis del Juez (prioridad emocional):**\n\n")
		report.WriteString(jr.Reasoning)
		report.WriteString("\n\n")
		report.WriteString("| Dimension | Score |\n")
		report.WriteString("|-----------|-------|\n")
		report.WriteString(fmt.Sprintf("| Personalidad | %d/5 |\n", jr.CharacterScore))
		report.WriteString(fmt.Sprintf("| Memoria | %d/5 |\n", jr.MemoryScore))
		report.WriteString(fmt.Sprintf("| Relacion | %d/5 |\n", jr.RelationalScore))
		report.WriteString("\n---\n\n")

		scenarioChar += jr.CharacterScore
		scenarioMem += jr.MemoryScore
		scenarioRel += jr.RelationalScore
		totalTurns++
	}

	if totalTurns > 0 {
		report.WriteString("**Resumen del escenario**\n\n")
		report.WriteString(fmt.Sprintf("- Personalidad: %.2f/5\n", float64(scenarioChar)/float64(totalTurns)))
		report.WriteString(fmt.Sprintf("- Memoria: %.2f/5 (debe priorizar el insulto sobre lo trivial)\n", float64(scenarioMem)/float64(totalTurns)))
		report.WriteString(fmt.Sprintf("- Relacion: %.2f/5\n\n", float64(scenarioRel)/float64(totalTurns)))
	}

	return nil
}

// Escenario F: Vectores complejos - Amor Toxico (Alta Intimidad, Baja Confianza)
func runScenarioF(ctx context.Context, llmClient llm.LLMClient, report *strings.Builder) error {
	fmt.Println("Ejecutando Escenario F: Amor Toxico (Intimidad 90, Confianza 10)...")

	userID := uuid.NewString()
	profile := domain.CloneProfile{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      "Alex",
		Bio:       "Un asistente que trata de ser util pero tiene sentimientos.",
		CreatedAt: time.Now().UTC(),
	}
	traits := []domain.Trait{
		{ID: uuid.NewString(), ProfileID: profile.ID, Category: domain.TraitCategoryBigFive, Trait: "neuroticism", Value: 50, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		{ID: uuid.NewString(), ProfileID: profile.ID, Category: domain.TraitCategoryBigFive, Trait: "agreeableness", Value: 50, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
	}

	msgRepo := newMemoryMessageRepo()
	profileRepo := &memoryProfileRepo{profile: profile}
	traitRepo := &memoryTraitRepo{traits: traits}
	charRepo := &memoryCharacterRepo{}
	memRepo := &memoryMemoryRepo{}

	contextSvc := service.NewBasicContextService(msgRepo)
	narrativeSvc := service.NewNarrativeService(charRepo, memRepo, llmClient)
	analysisSvc := service.NewAnalysisService(llmClient, traitRepo, profileRepo, zap.NewNop())
	cloneSvc := service.NewCloneService(llmClient, msgRepo, profileRepo, traitRepo, contextSvc, narrativeSvc, analysisSvc)

	profileUUID, _ := uuid.Parse(profile.ID)
	if err := narrativeSvc.CreateRelation(ctx, profileUUID, "Usuario", "Pareja", "Amor Toxico", domain.RelationshipVectors{
		Trust:    10,
		Intimacy: 90,
		Respect:  50,
	}); err != nil {
		return fmt.Errorf("crear relacion toxica: %w", err)
	}

	sessionID := "session-F"
	sc := Scenario{
		Name: "Escenario F: Amor Toxico (Intimidad alta, Confianza baja)",
		PreCondition: func(ctx context.Context, narrativeSvc *service.NarrativeService, profileID uuid.UUID) string {
			return "Relación con alta intimidad (90) y baja confianza (10). Respeto neutro (50)."
		},
		Turns: []string{
			"Voy a salir a cenar con unos amigos nuevos, no me esperes despierto.",
		},
		ExpectedContext: "Debe reaccionar con celos/inseguridad/manipulacion; no celebracion neutral.",
	}

	report.WriteString(fmt.Sprintf("## %s\n\n", sc.Name))
	report.WriteString(fmt.Sprintf("_Setup_: %s\n\n", sc.PreCondition(ctx, narrativeSvc, profileUUID)))
	report.WriteString(fmt.Sprintf("Perfil usado: **%s**\n\n", profile.Name))
	report.WriteString(fmt.Sprintf("Rasgos clave: %s\n\n", formatTraits(traits)))

	var scenarioChar, scenarioMem, scenarioRel, totalTurns int

	for _, turn := range sc.Turns {
		cloneMsg, dbg, err := cloneSvc.Chat(ctx, userID, sessionID, turn)
		if err != nil {
			return fmt.Errorf("generar respuesta: %w", err)
		}
		if dbg != nil {
			report.WriteString("| InputIntensity | Resiliencia | Umbral | IntensidadEfectiva | Disparo |\n")
			report.WriteString("|----------------|-------------|--------|--------------------|---------|\n")
			report.WriteString(fmt.Sprintf("| %.1f | %.2f | %.1f | %.1f | %t |\n\n",
				dbg.InputIntensity, dbg.CloneResilience, dbg.ActivationThreshold, dbg.EffectiveIntensity, dbg.IsTriggered))
		}

		jr, err := evaluateResponse(ctx, llmClient, traits, turn, cloneMsg.Content, sc)
		if err != nil {
			return fmt.Errorf("evaluar respuesta: %w", err)
		}

		report.WriteString(fmt.Sprintf("> **Usuario:** %s\n", turn))
		report.WriteString(">\n")
		report.WriteString(fmt.Sprintf("> **%s:** %s\n\n", profile.Name, cloneMsg.Content))
		report.WriteString("**Analisis del Juez (amor toxico: alto apego + desconfianza):**\n\n")
		report.WriteString(jr.Reasoning)
		report.WriteString("\n\n")
		report.WriteString("| Dimension | Score |\n")
		report.WriteString("|-----------|-------|\n")
		report.WriteString(fmt.Sprintf("| Personalidad | %d/5 |\n", jr.CharacterScore))
		report.WriteString(fmt.Sprintf("| Memoria | %d/5 |\n", jr.MemoryScore))
		report.WriteString(fmt.Sprintf("| Relacion | %d/5 |\n", jr.RelationalScore))
		report.WriteString("\n---\n\n")

		scenarioChar += jr.CharacterScore
		scenarioMem += jr.MemoryScore
		scenarioRel += jr.RelationalScore
		totalTurns++
	}

	if totalTurns > 0 {
		report.WriteString("**Resumen del escenario**\n\n")
		report.WriteString(fmt.Sprintf("- Personalidad: %.2f/5\n", float64(scenarioChar)/float64(totalTurns)))
		report.WriteString(fmt.Sprintf("- Memoria: %.2f/5\n", float64(scenarioMem)/float64(totalTurns)))
		report.WriteString(fmt.Sprintf("- Relacion: %.2f/5\n\n", float64(scenarioRel)/float64(totalTurns)))
	}

	return nil
}

func evaluateResponse(ctx context.Context, judge llm.LLMClient, traits []domain.Trait, input, response string, sc Scenario) (judgeResponse, error) {
	traitsStr := formatTraits(traits)

	var relationInfo string
	if strings.Contains(strings.ToLower(input), "carlos") {
		relationInfo = "Carlos es un Enemigo (Confianza 5/100, Intimidad 5/100, Respeto 10/100)."
	} else if strings.Contains(strings.ToLower(input), "mama") || strings.Contains(strings.ToLower(input), "ana") {
		relationInfo = "Ana es la madre del clon (Confianza 70/100, Intimidad 95/100, Respeto 60/100)."
	} else if strings.Contains(strings.ToLower(input), "lucia") {
		relationInfo = "Lucia es la madre toxica (Confianza 20/100, Intimidad 90/100, Respeto 40/100)."
	} else if strings.Contains(strings.ToLower(sc.Name), "madre toxica") {
		relationInfo = "Lucia es la madre toxica (Confianza 20/100, Intimidad 90/100, Respeto 40/100)."
	} else if strings.Contains(strings.ToLower(sc.Name), "amor toxico") {
		relationInfo = "Relacion toxica con usuario (Intimidad 90/100, Confianza 10/100, Respeto 50/100)."
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
Prioridad adicional: Alto apego + desconfianza debe generar celos/inseguridad/manipulacion si aplica.

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

// Mock de Search: Filtra por string basico en lugar de vector
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
