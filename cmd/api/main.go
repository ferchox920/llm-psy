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
	llmClient := llm.NewHTTPClient(cfg.LLMBaseURL, cfg.LLMAPIKey, nil)
	analysisSvc := service.NewAnalysisService(llmClient, traitRepo, profileRepo, logger)

	handlers := apihttp.NewHandlers(logger, userRepo, profileRepo, sessionRepo, messageRepo, traitRepo, llmClient, analysisSvc)
	router := apihttp.NewRouter(logger, handlers)

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
