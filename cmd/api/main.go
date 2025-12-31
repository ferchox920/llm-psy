package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"clone-llm/internal/config"
	"clone-llm/internal/db"
	"clone-llm/internal/email"
	apihttp "clone-llm/internal/http"
	"clone-llm/internal/llm"
	"clone-llm/internal/repository"
	"clone-llm/internal/service"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
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
	emailSender := email.NewDisabledSender("email sender not configured")
	if cfg.SMTPHost != "" {
		sender, err := email.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom, cfg.SMTPFromName, cfg.SMTPUseTLS)
		if err != nil {
			logger.Warn("smtp sender init failed", zap.Error(err))
		} else {
			emailSender = sender
		}
	}
	var (
		otpLimiter   service.OTPRateLimiter
		tokenStore   service.RefreshTokenStore
		redisClient *redis.Client
	)
	if cfg.RedisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})
		ctxPing, cancel := context.WithTimeout(ctx, 2*time.Second)
		if err := redisClient.Ping(ctxPing).Err(); err != nil {
			logger.Warn("redis ping failed", zap.Error(err))
		} else {
			otpLimiter = service.NewRedisOTPRateLimiter(redisClient, 10*time.Minute, 3)
			tokenStore = service.NewRedisRefreshTokenStore(redisClient)
		}
		cancel()
	}
	jwtSvc := service.NewJWTServiceWithStore(
		cfg.JWTSecret,
		time.Duration(cfg.JWTAccessTTLMinutes)*time.Minute,
		time.Duration(cfg.JWTRefreshTTLMinutes)*time.Minute,
		tokenStore,
	)
	if cfg.JWTSecret == "" {
		logger.Warn("jwt secret not configured")
	}

	userSvc := service.NewUserService(logger, userRepo, emailSender, otpLimiter)
	userHandler := apihttp.NewUserHandler(logger, userSvc, jwtSvc)
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
