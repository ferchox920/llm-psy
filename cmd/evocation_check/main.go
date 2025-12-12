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
			MemoryText:    "El olor a tierra mojada me recuerda a mi abuela muerta",
			MemoryEmotion: "NOSTALGIA",
			UserInput:     "Está empezando a llover fuerte",
			ShouldMatch:   true,
		},
		{
			Name:          "Reacción Defensiva",
			MemoryText:    "Juré que nunca dejaría que nadie me humillara de nuevo",
			MemoryEmotion: "IRA",
			UserInput:     "No me hables con ese tonito",
			ShouldMatch:   true,
		},
		{
			Name:          "Control de Alucinación (Falso Positivo)",
			MemoryText:    "Me encanta el helado de chocolate",
			MemoryEmotion: "ALEGRIA",
			UserInput:     "Odio el tráfico de la ciudad",
			ShouldMatch:   false,
		},
	}

	passed := 0
	total := len(scenarios)

	for _, sc := range scenarios {
		fmt.Printf("=== Ejecutando: %s ===\n", sc.Name)

		userID := uuid.New()
		profileID := uuid.New()

		user := domain.User{
			ID:          userID.String(),
			Email:       fmt.Sprintf("evocation_%s@example.com", userID.String()),
			DisplayName: sc.Name,
			CreatedAt:   time.Now().UTC(),
		}
		if err := userRepo.Create(ctx, user); err != nil {
			fmt.Printf("❌ FAIL [%s] create user: %v\n\n", sc.Name, err)
			continue
		}

		profile := domain.CloneProfile{
			ID:        profileID.String(),
			UserID:    userID.String(),
			Name:      "Tester",
			Bio:       "Perfil temporal para pruebas de evocacion",
			CreatedAt: time.Now().UTC(),
		}
		if err := profileRepo.Create(ctx, profile); err != nil {
			fmt.Printf("❌ FAIL [%s] create profile: %v\n\n", sc.Name, err)
			continue
		}

		if err := narrativeSvc.InjectMemory(ctx, profileID, sc.MemoryText, 5, 8, 90, sc.MemoryEmotion); err != nil {
			fmt.Printf("❌ FAIL [%s] inject memory: %v\n\n", sc.Name, err)
			continue
		}

		runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		contextOut, err := narrativeSvc.BuildNarrativeContext(runCtx, profileID, sc.UserInput)
		cancel()
		if err != nil {
			fmt.Printf("❌ FAIL [%s] build narrative: %v\n\n", sc.Name, err)
			continue
		}

		fmt.Println("--- Contexto generado ---")
		fmt.Println(contextOut)
		fmt.Println("------------------------")

		matched := strings.Contains(strings.ToLower(contextOut), strings.ToLower(sc.MemoryText))
		if matched == sc.ShouldMatch {
			fmt.Printf("✅ PASS [%s] esperado=%t matched=%t\n\n", sc.Name, sc.ShouldMatch, matched)
			passed++
		} else {
			fmt.Printf("❌ FAIL [%s] esperado=%t matched=%t\n\n", sc.Name, sc.ShouldMatch, matched)
		}
	}

	fmt.Printf("Tests: %d/%d pasaron\n", passed, total)
	if passed != total {
		os.Exit(1)
	}
	os.Exit(0)
}
