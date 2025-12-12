package main

import (
	"context"
	"fmt"
	"log"
	"os"
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

	scenarios := []Scenario{
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
	}

	passed := 0

	for _, sc := range scenarios {
		start := time.Now()
		env, err := createTestEnvironment(ctx, userRepo, profileRepo, sc.Name)
		if err != nil {
			fmt.Printf("❌ FAIL [%s] setup env: %v\n\n", sc.Name, err)
			continue
		}

		if err := narrativeSvc.InjectMemory(ctx, env.profileID, sc.MemoryText, 5, 8, 90, sc.MemoryEmotion); err != nil {
			fmt.Printf("❌ FAIL [%s] inject memory: %v\n\n", sc.Name, err)
			continue
		}

		runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		contextOut, err := narrativeSvc.BuildNarrativeContext(runCtx, env.profileID, sc.UserInput)
		cancel()
		if err != nil {
			fmt.Printf("❌ FAIL [%s] build narrative: %v\n\n", sc.Name, err)
			continue
		}

		matched := strings.Contains(strings.ToLower(contextOut), strings.ToLower(sc.MemoryText))
		latency := time.Since(start)

		fmt.Println("--- Contexto generado ---")
		fmt.Println(contextOut)
		fmt.Println("------------------------")

		if matched == sc.ShouldMatch {
			fmt.Printf("✅ PASS [%s] esperado=%t matched=%t latency=%s\n\n", sc.Name, sc.ShouldMatch, matched, latency)
			passed++
		} else {
			fmt.Printf("❌ FAIL [%s] esperado=%t matched=%t latency=%s\n\n", sc.Name, sc.ShouldMatch, matched, latency)
		}
	}

	fmt.Printf("Resultados: %d/%d tests pasaron\n", passed, len(scenarios))
	if passed != len(scenarios) {
		os.Exit(1)
	}
	os.Exit(0)
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
