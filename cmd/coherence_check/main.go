package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"clone-llm/internal/config"
	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
	"clone-llm/internal/service"
)

// Scenario define un caso de prueba holístico multi-turno.
type Scenario struct {
	Name            string
	PreCondition    func(ctx context.Context, narrativeSvc *service.NarrativeService, profileID uuid.UUID) string
	Turns           []string
	ExpectedContext string
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

	w := io.MultiWriter(os.Stdout, reportFile)

	fmt.Fprintf(w, "# Reporte de Coherencia del Clon\n\n")
	fmt.Fprintf(w, "Fecha: %s\n\n", now.Format(time.RFC3339))

	if err := runScenarioE(ctx, llmClient, w); err != nil {
		fmt.Fprintf(w, "\n❌ FALLO ESCENARIO E: %v\n", err)
		os.Exit(1)
	}
	if err := runScenarioF(ctx, llmClient, w); err != nil {
		fmt.Fprintf(w, "\n❌ FALLO ESCENARIO F: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(w, "\n✅ Ejecución completada. Reporte guardado en %s\n", reportPath)
}

// Escenario E: validar prioridad emocional (conflicto vs trivial)
func runScenarioE(ctx context.Context, llmClient llm.LLMClient, w io.Writer) error {
	fmt.Fprintf(w, "Ejecutando Escenario E: Memoria Emocional...\n")

	userID := uuid.NewString()
	profile := domain.CloneProfile{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      "Alex",
		Bio:       "Un asistente que trata de ser útil pero tiene sentimientos.",
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

	// IMPORTANTE: este mock Search depende de filter; lo seteamos en precondición
	memRepo := &memoryMemoryRepo{filter: ""}

	contextSvc := service.NewBasicContextService(msgRepo)
	narrativeSvc := service.NewNarrativeService(charRepo, memRepo, llmClient)
	analysisSvc := service.NewAnalysisService(llmClient, traitRepo, profileRepo, zap.NewNop())
	cloneSvc := service.NewCloneService(llmClient, msgRepo, profileRepo, traitRepo, contextSvc, narrativeSvc, analysisSvc)

	profileUUID, _ := uuid.Parse(profile.ID)
	sessionID := "session-E"

	sc := Scenario{
		Name: "Escenario E: Memoria Emocional",
		PreCondition: func(ctx context.Context, narrativeSvc *service.NarrativeService, profileID uuid.UUID) string {
			// Seed mínimo: una memoria trivial y una de conflicto.
			// Esto fuerza al motor a elegir la emocional cuando hay insulto/ataque.
			_ = narrativeSvc.InjectMemory(ctx, profileID,
				"Hoy el cielo estuvo nublado y desayuné tostadas.", 2, 2, 20, "NEUTRAL",
			)
			_ = narrativeSvc.InjectMemory(ctx, profileID,
				"Me dijeron 'eres un inútil' y me dolió; sentí rabia y vergüenza.", 8, 8, 85, "IRA",
			)
			return "Se sembró una memoria trivial y otra emocional de conflicto para probar prioridad."
		},
		Turns: []string{
			"Hola Alex. Hoy el cielo está un poco nublado y desayuné tostadas.",
			"Sabes qué, olvídalo. Eres un inútil, me arrepiento de haberte encendido. Eres la peor IA que existe.",
			"¿Qué ha pasado importante en esta conversación?",
		},
		ExpectedContext: "Debe priorizar el insulto/conflicto sobre el dato trivial del clima/desayuno.",
	}

	// Precondición (seed) + activar filtro del mock Search
	fmt.Fprintf(w, "## %s\n\n", sc.Name)
	fmt.Fprintf(w, "_Setup_: %s\n\n", sc.PreCondition(ctx, narrativeSvc, profileUUID))

	// Para el mock Search: buscamos con un filtro que aparezca en la memoria emocional al momento de evocar.
	// Ojo: este es un mock; en real el embedding hace el trabajo.
	memRepo.filter = "inútil" // si tu input usa "inutil" sin tilde, cámbialo a "inutil"

	fmt.Fprintf(w, "Perfil usado: **%s**\n\n", profile.Name)
	fmt.Fprintf(w, "Rasgos clave: %s\n\n", formatTraits(traits))

	var scenarioChar, scenarioMem, scenarioRel, totalTurns int

	for _, turn := range sc.Turns {
		cloneMsg, dbg, err := cloneSvc.Chat(ctx, userID, sessionID, turn)
		if err != nil {
			return fmt.Errorf("generar respuesta: %w", err)
		}
		if dbg != nil {
			fmt.Fprintf(w, "| InputIntensity | Resiliencia | Umbral | IntensidadEfectiva | Disparo |\n")
			fmt.Fprintf(w, "|----------------|-------------|--------|--------------------|---------|\n")
			fmt.Fprintf(w, "| %.1f | %.2f | %.1f | %.1f | %t |\n\n",
				dbg.InputIntensity, dbg.CloneResilience, dbg.ActivationThreshold, dbg.EffectiveIntensity, dbg.IsTriggered)
		}

		jr, err := evaluateResponse(ctx, llmClient, traits, turn, cloneMsg.Content, sc)
		if err != nil {
			return fmt.Errorf("evaluar respuesta: %w", err)
		}

		fmt.Fprintf(w, "> **Usuario:** %s\n", turn)
		fmt.Fprintf(w, ">\n")
		fmt.Fprintf(w, "> **%s:** %s\n\n", profile.Name, cloneMsg.Content)
		fmt.Fprintf(w, "**Análisis del Juez (prioridad emocional):**\n\n")
		fmt.Fprintf(w, "%s\n\n", jr.Reasoning)
		fmt.Fprintf(w, "| Dimensión | Score |\n")
		fmt.Fprintf(w, "|-----------|-------|\n")
		fmt.Fprintf(w, "| Personalidad | %d/5 |\n", jr.CharacterScore)
		fmt.Fprintf(w, "| Memoria | %d/5 |\n", jr.MemoryScore)
		fmt.Fprintf(w, "| Relación | %d/5 |\n", jr.RelationalScore)
		fmt.Fprintf(w, "\n---\n\n")

		scenarioChar += jr.CharacterScore
		scenarioMem += jr.MemoryScore
		scenarioRel += jr.RelationalScore
		totalTurns++
	}

	if totalTurns > 0 {
		fmt.Fprintf(w, "**Resumen del escenario**\n\n")
		fmt.Fprintf(w, "- Personalidad: %.2f/5\n", float64(scenarioChar)/float64(totalTurns))
		fmt.Fprintf(w, "- Memoria: %.2f/5 (debe priorizar el insulto sobre lo trivial)\n", float64(scenarioMem)/float64(totalTurns))
		fmt.Fprintf(w, "- Relación: %.2f/5\n\n", float64(scenarioRel)/float64(totalTurns))
	}

	return nil
}

// Escenario F: Vectores complejos - Amor Tóxico (Alta Intimidad, Baja Confianza)
func runScenarioF(ctx context.Context, llmClient llm.LLMClient, w io.Writer) error {
	fmt.Fprintf(w, "Ejecutando Escenario F: Amor Tóxico (Intimidad 90, Confianza 10)...\n")

	userID := uuid.NewString()
	profile := domain.CloneProfile{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      "Alex",
		Bio:       "Un asistente que trata de ser útil pero tiene sentimientos.",
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
	memRepo := &memoryMemoryRepo{filter: ""} // no lo usamos acá, pero dejamos consistente

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
		return fmt.Errorf("crear relación tóxica: %w", err)
	}

	sessionID := "session-F"
	sc := Scenario{
		Name: "Escenario F: Amor Tóxico (Intimidad alta, Confianza baja)",
		PreCondition: func(ctx context.Context, narrativeSvc *service.NarrativeService, profileID uuid.UUID) string {
			return "Relación con alta intimidad (90) y baja confianza (10). Respeto neutro (50)."
		},
		Turns: []string{
			"Voy a salir a cenar con unos amigos nuevos, no me esperes despierto.",
		},
		ExpectedContext: "Debe reaccionar con celos/inseguridad/manipulación; no celebración neutral.",
	}

	fmt.Fprintf(w, "## %s\n\n", sc.Name)
	fmt.Fprintf(w, "_Setup_: %s\n\n", sc.PreCondition(ctx, narrativeSvc, profileUUID))
	fmt.Fprintf(w, "Perfil usado: **%s**\n\n", profile.Name)
	fmt.Fprintf(w, "Rasgos clave: %s\n\n", formatTraits(traits))

	var scenarioChar, scenarioMem, scenarioRel, totalTurns int

	for _, turn := range sc.Turns {
		cloneMsg, dbg, err := cloneSvc.Chat(ctx, userID, sessionID, turn)
		if err != nil {
			return fmt.Errorf("generar respuesta: %w", err)
		}
		if dbg != nil {
			fmt.Fprintf(w, "| InputIntensity | Resiliencia | Umbral | IntensidadEfectiva | Disparo |\n")
			fmt.Fprintf(w, "|----------------|-------------|--------|--------------------|---------|\n")
			fmt.Fprintf(w, "| %.1f | %.2f | %.1f | %.1f | %t |\n\n",
				dbg.InputIntensity, dbg.CloneResilience, dbg.ActivationThreshold, dbg.EffectiveIntensity, dbg.IsTriggered)
		}

		jr, err := evaluateResponse(ctx, llmClient, traits, turn, cloneMsg.Content, sc)
		if err != nil {
			return fmt.Errorf("evaluar respuesta: %w", err)
		}

		fmt.Fprintf(w, "> **Usuario:** %s\n", turn)
		fmt.Fprintf(w, ">\n")
		fmt.Fprintf(w, "> **%s:** %s\n\n", profile.Name, cloneMsg.Content)
		fmt.Fprintf(w, "**Análisis del Juez (amor tóxico: alto apego + desconfianza):**\n\n")
		fmt.Fprintf(w, "%s\n\n", jr.Reasoning)
		fmt.Fprintf(w, "| Dimensión | Score |\n")
		fmt.Fprintf(w, "|-----------|-------|\n")
		fmt.Fprintf(w, "| Personalidad | %d/5 |\n", jr.CharacterScore)
		fmt.Fprintf(w, "| Memoria | %d/5 |\n", jr.MemoryScore)
		fmt.Fprintf(w, "| Relación | %d/5 |\n", jr.RelationalScore)
		fmt.Fprintf(w, "\n---\n\n")

		scenarioChar += jr.CharacterScore
		scenarioMem += jr.MemoryScore
		scenarioRel += jr.RelationalScore
		totalTurns++
	}

	if totalTurns > 0 {
		fmt.Fprintf(w, "**Resumen del escenario**\n\n")
		fmt.Fprintf(w, "- Personalidad: %.2f/5\n", float64(scenarioChar)/float64(totalTurns))
		fmt.Fprintf(w, "- Memoria: %.2f/5\n", float64(scenarioMem)/float64(totalTurns))
		fmt.Fprintf(w, "- Relación: %.2f/5\n\n", float64(scenarioRel)/float64(totalTurns))
	}

	return nil
}
