package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"clone-llm/internal/config"
	"clone-llm/internal/db"
	apihttp "clone-llm/internal/http"
	"clone-llm/internal/llm"
	"clone-llm/internal/repository"
	"clone-llm/internal/service"

	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

func main() {
	ctx := context.Background()

	if err := godotenv.Load(); err != nil {
		log.Printf("warning: loading .env: %v", err)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		panic(err)
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		logger.Fatal("db connect", zap.Error(err))
	}
	defer pool.Close()

	userRepo := repository.NewPgUserRepository(pool)
	profileRepo := repository.NewPgProfileRepository(pool)
	sessionRepo := repository.NewPgSessionRepository(pool)
	messageRepo := repository.NewPgMessageRepository(pool)
	traitRepo := repository.NewPgTraitRepository(pool)
	characterRepo := repository.NewPgCharacterRepository(pool)
	memoryRepo := repository.NewPgMemoryRepository(pool)
	llmClient := llm.NewHTTPClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, logger)
	analysisSvc := service.NewAnalysisService(llmClient, traitRepo, profileRepo, logger)
	contextSvc := service.NewBasicContextService(messageRepo)
	narrativeSvc := service.NewNarrativeService(characterRepo, memoryRepo, llmClient)
	promptBuilder := service.ClonePromptBuilder{}
	responseParser := service.LLMResponseParser{}
	reactionEngine := service.ReactionEngine{}
	cloneSvc := service.NewCloneService(llmClient, messageRepo, profileRepo, traitRepo, contextSvc, narrativeSvc, analysisSvc, promptBuilder, responseParser, reactionEngine)
	userHandler := apihttp.NewUserHandler(logger, userRepo)
	cloneHandler := apihttp.NewCloneHandler(logger, profileRepo, traitRepo)
	chatHandler := apihttp.NewChatHandler(logger, sessionRepo, messageRepo, analysisSvc, cloneSvc)
	router := apihttp.NewRouter(logger, userHandler, chatHandler, cloneHandler)

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("starting server", zap.String("port", cfg.HTTPPort))

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("server error", zap.Error(err))
	}
}
