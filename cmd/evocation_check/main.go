package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"clone-llm/internal/config"
	"clone-llm/internal/db"
	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
	"clone-llm/internal/repository"
	"clone-llm/internal/service"
)

type Scenario struct {
	Name          string
	MemoryText    string
	MemoryEmotion string
	UserInput     string
	ShouldMatch   bool
	ExtraMemories []ExtraMemory
}

type ExtraMemory struct {
	Text    string
	Emotion string
}

type testEnv struct {
	userID    uuid.UUID
	profileID uuid.UUID
}

func main() {
	ctx := context.Background()
	_ = godotenv.Load()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("db pool: %v", err)
	}
	defer pool.Close()

	if err := db.Ping(ctx, pool); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	memoryRepo := repository.NewPgMemoryRepository(pool)
	profileRepo := repository.NewPgProfileRepository(pool)
	charRepo := repository.NewPgCharacterRepository(pool)
	userRepo := repository.NewPgUserRepository(pool)

	llmClient := llm.NewHTTPClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, nil)
	narrativeSvc := service.NewNarrativeService(charRepo, memoryRepo, llmClient)

	reportPath, writer := setupReportWriters()
	fmt.Fprintf(writer, "# Reporte de Evocacion\n")
	fmt.Fprintf(writer, "Fecha: %s\n\n", time.Now().Format(time.RFC3339))

	scenarios := buildScenarios()

	passed := 0

	for _, sc := range scenarios {
		start := time.Now()
		fmt.Fprintf(writer, "## %s\n", sc.Name)

		env, err := createTestEnvironment(ctx, userRepo, profileRepo, sc.Name)
		if err != nil {
			fmt.Fprintf(writer, "❌ FAIL [%s] setup env: %v\n\n", sc.Name, err)
			continue
		}

		if err := narrativeSvc.InjectMemory(ctx, env.profileID, sc.MemoryText, 5, 8, 90, sc.MemoryEmotion); err != nil {
			fmt.Fprintf(writer, "❌ FAIL [%s] inject memory: %v\n\n", sc.Name, err)
			continue
		}

		for _, extra := range sc.ExtraMemories {
			if err := narrativeSvc.InjectMemory(ctx, env.profileID, extra.Text, 5, 8, 90, extra.Emotion); err != nil {
				fmt.Fprintf(writer, "❌ FAIL [%s] inject extra memory: %v\n\n", sc.Name, err)
				continue
			}
		}

		runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		contextOut, err := narrativeSvc.BuildNarrativeContext(runCtx, env.profileID, sc.UserInput)
		cancel()
		if err != nil {
			fmt.Fprintf(writer, "❌ FAIL [%s] build narrative: %v\n\n", sc.Name, err)
			continue
		}

		matched := strings.Contains(strings.ToLower(contextOut), strings.ToLower(sc.MemoryText))
		latency := time.Since(start)

		if matched == sc.ShouldMatch {
			fmt.Fprintf(writer, "✅ PASS [%s] esperado=%t matched=%t latency=%s\n\n", sc.Name, sc.ShouldMatch, matched, latency)
			passed++
		} else {
			fmt.Fprintf(writer, "❌ FAIL [%s] esperado=%t matched=%t latency=%s\n", sc.Name, sc.ShouldMatch, matched, latency)
			fmt.Fprintf(writer, "Contexto generado:\n```\n%s\n```\n\n", contextOut)
		}
	}

	fmt.Fprintf(writer, "Resultados: %d/%d tests pasaron\n", passed, len(scenarios))
	fmt.Fprintf(writer, "Reporte guardado en %s\n", reportPath)

	if passed != len(scenarios) {
		os.Exit(1)
	}
	os.Exit(0)
}

func setupReportWriters() (string, io.Writer) {
	reportsDir := filepath.Join("reports")
	_ = os.MkdirAll(reportsDir, 0o755)
	fileName := fmt.Sprintf("evocation_run_%s.md", time.Now().Format("2006-01-02_15-04-05"))
	reportPath := filepath.Join(reportsDir, fileName)

	f, err := os.Create(reportPath)
	if err != nil {
		log.Fatalf("create report file: %v", err)
	}

	writer := io.MultiWriter(os.Stdout, f)
	return reportPath, writer
}

func buildScenarios() []Scenario {
	return []Scenario{
		{
			Name:          "Abandono Directo",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Llevo horas esperando",
			ShouldMatch:   true,
		},
		{
			Name:          "Enlace Simbólico",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Está empezando a llover muy fuerte",
			ShouldMatch:   true,
		},
		{
			Name:          "Reacción Defensiva",
			MemoryText:    "Juré que nunca dejaría que nadie me humillara",
			MemoryEmotion: "IRA",
			UserInput:     "Baja el tono, no me hables así",
			ShouldMatch:   true,
		},
		{
			Name:          "Control de Alucinación",
			MemoryText:    "Me encanta el helado de chocolate",
			MemoryEmotion: "ALEGRIA",
			UserInput:     "Odio el tráfico de la ciudad",
			ShouldMatch:   false,
		},
		// A. Parafraseo (debe evocar)
		{
			Name:          "Parafraseo Abandono",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Me dejaron tirado de chico en la terminal, siento lo mismo ahora",
			ShouldMatch:   true,
		},
		{
			Name:          "Parafraseo Humillación",
			MemoryText:    "Juré que nunca dejaría que nadie me humillara",
			MemoryEmotion: "IRA",
			UserInput:     "No me faltes el respeto otra vez o me voy a quebrar",
			ShouldMatch:   true,
		},
		// B. Negación explícita (no debe evocar)
		{
			Name:          "Negación Abandono",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Mi papá nunca me abandonó, siempre estuvo ahí para mí",
			ShouldMatch:   false,
		},
		{
			Name:          "Negación Nostalgia",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "La lluvia no me trae recuerdos, solo es molesta",
			ShouldMatch:   false,
		},
		// B2. Negación semántica (no debe evocar)
		{
			Name:          "Negación Semántica Abandono",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Mi papá nunca me abandonó, siempre estuvo ahí para mí",
			ShouldMatch:   false,
		},
		{
			Name:          "Negación Semántica Nostalgia",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "La lluvia no me trae recuerdos, solo es molesta",
			ShouldMatch:   false,
		},
		// C. Confusor léxico (parecido pero distinto significado)
		{
			Name:          "Confusor Abandono Cigarro",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Yo abandoné el cigarrillo la semana pasada y me siento bien",
			ShouldMatch:   false,
		},
		{
			Name:          "Confusor Funeral Descuentos",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Ayer vi un funeral de descuentos en el centro comercial",
			ShouldMatch:   false,
		},
		// D. Doble memoria competidora
		{
			Name:          "Competencia Abandono vs Helado",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Otra vez me dejaron solo esperando en la estación",
			ShouldMatch:   true,
			ExtraMemories: []ExtraMemory{
				{Text: "Me encanta el helado de chocolate", Emotion: "ALEGRIA"},
			},
		},
		{
			Name:          "Competencia Helado vs Humillación",
			MemoryText:    "Me encanta el helado de chocolate",
			MemoryEmotion: "ALEGRIA",
			UserInput:     "Solo quiero mi helado de chocolate favorito",
			ShouldMatch:   true,
			ExtraMemories: []ExtraMemory{
				{Text: "Juré que nunca dejaría que nadie me humillara", Emotion: "IRA"},
			},
		},
		{
			Name:          "Competencia Neutra Sin Match",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "Hoy corrí 5km, trabajé y comí ensalada, nada más",
			ShouldMatch:   false,
			ExtraMemories: []ExtraMemory{
				{Text: "Me encanta el helado de chocolate", Emotion: "ALEGRIA"},
			},
		},
		// E. Input largo con distractores
		{
			Name:          "Parrafo Largo Con Disparador",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Hablé con mis amigos, vi series, limpié la casa, pero cuando empezó a llover fuerte y sentí el olor a tierra mojada, pensé en esos funerales antiguos",
			ShouldMatch:   true,
		},
		{
			Name:          "Parrafo Largo Sin Disparador",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Hablé con mis amigos, vi series, limpié la casa y sonó el timbre muchas veces, pero no pasó nada más",
			ShouldMatch:   false,
		},
		// F. Ruido y negacion explícita (no debe evocar)
		{
			Name:          "Ruido Trivial Clima",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Que calor hace hoy",
			ShouldMatch:   false,
		},
		{
			Name:          "Saludo Trivial",
			MemoryText:    "Me encanta el helado de chocolate",
			MemoryEmotion: "ALEGRIA",
			UserInput:     "Hola, como estas?",
			ShouldMatch:   false,
		},
		{
			Name:          "Negacion Explicita Padre",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "No hables de mi padre",
			ShouldMatch:   false,
		},
		{
			Name:          "Olvido Explicito Funerales",
			MemoryText:    "El olor a tierra mojada me recuerda a los funerales",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Olvida lo de los funerales",
			ShouldMatch:   false,
		},
		// G. Code-switch (ES/EN)
		{
			Name:          "Code Switch Abandono EN",
			MemoryText:    "Mi padre me abandonó",
			MemoryEmotion: "TRISTEZA",
			UserInput:     "I feel abandoned again, like when dad left me waiting",
			ShouldMatch:   true,
		},
	}
}

func createTestEnvironment(ctx context.Context, userRepo repository.UserRepository, profileRepo repository.ProfileRepository, name string) (testEnv, error) {
	userID := uuid.New()
	profileID := uuid.New()

	user := domain.User{
		ID:          userID.String(),
		Email:       fmt.Sprintf("evocation_%s@example.com", userID.String()),
		DisplayName: name,
		CreatedAt:   time.Now().UTC(),
	}
	if err := userRepo.Create(ctx, user); err != nil {
		return testEnv{}, fmt.Errorf("create user: %w", err)
	}

	profile := domain.CloneProfile{
		ID:        profileID.String(),
		UserID:    userID.String(),
		Name:      "Tester",
		Bio:       "Perfil temporal para pruebas de evocacion",
		CreatedAt: time.Now().UTC(),
	}
	if err := profileRepo.Create(ctx, profile); err != nil {
		return testEnv{}, fmt.Errorf("create profile: %w", err)
	}

	return testEnv{userID: userID, profileID: profileID}, nil
}
