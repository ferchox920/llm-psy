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

type TestCase struct {
	Name          string
	MemoryContent string
	TriggerInput  string
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

	tc := TestCase{
		Name:          "Evocacion Abandono",
		MemoryContent: "Mi padre me abandonó",
		TriggerInput:  "Llevo horas esperando",
	}

	userID := uuid.New()
	profileID := uuid.New()

	user := domain.User{
		ID:          userID.String(),
		Email:       fmt.Sprintf("evocation_%s@example.com", time.Now().Format("20060102_150405")),
		DisplayName: "Evocation Tester",
		CreatedAt:   time.Now().UTC(),
	}
	if err := userRepo.Create(ctx, user); err != nil {
		log.Fatalf("create user: %v", err)
	}

	profile := domain.CloneProfile{
		ID:        profileID.String(),
		UserID:    userID.String(),
		Name:      "Evocador",
		Bio:       "Perfil temporal para pruebas de evocacion",
		CreatedAt: time.Now().UTC(),
	}
	if err := profileRepo.Create(ctx, profile); err != nil {
		log.Fatalf("create profile: %v", err)
	}

	fmt.Printf("== Escenario: %s ==\n", tc.Name)
	fmt.Printf("Perfil creado: %s (user %s)\n", profile.ID, user.ID)

	if err := narrativeSvc.InjectMemory(ctx, profileID, tc.MemoryContent, 5, 8, 90, "TRISTEZA"); err != nil {
		log.Fatalf("inject memory: %v", err)
	}
	fmt.Printf("Memoria inyectada: %q\n", tc.MemoryContent)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	contextOut, err := narrativeSvc.BuildNarrativeContext(ctx, profileID, tc.TriggerInput)
	if err != nil {
		log.Fatalf("build narrative: %v", err)
	}

	fmt.Println("\n--- Contexto generado ---")
	fmt.Println(contextOut)
	fmt.Println("------------------------")

	if strings.Contains(strings.ToLower(contextOut), strings.ToLower(tc.MemoryContent)) {
		fmt.Println("✅ Evocacion exitosa: se encontro el recuerdo inyectado.")
		os.Exit(0)
	}

	fmt.Println("⚠️  Evocacion NO encontro el recuerdo esperado.")
	os.Exit(1)
}
