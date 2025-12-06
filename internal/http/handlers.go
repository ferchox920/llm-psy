package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
	"clone-llm/internal/repository"
	"clone-llm/internal/service"
)

// Handlers mantiene dependencias para los endpoints HTTP.
type Handlers struct {
	logger       *zap.Logger
	users        repository.UserRepository
	profiles     repository.ProfileRepository
	sessions     repository.SessionRepository
	messages     repository.MessageRepository
	traits       repository.TraitRepository
	llmClient    llm.LLMClient
	analysisServ *service.AnalysisService
}

// NewHandlers crea una instancia de Handlers con las dependencias necesarias.
func NewHandlers(
	logger *zap.Logger,
	users repository.UserRepository,
	profiles repository.ProfileRepository,
	sessions repository.SessionRepository,
	messages repository.MessageRepository,
	traits repository.TraitRepository,
	llmClient llm.LLMClient,
	analysisServ *service.AnalysisService,
) *Handlers {
	return &Handlers{
		logger:       logger,
		users:        users,
		profiles:     profiles,
		sessions:     sessions,
		messages:     messages,
		traits:       traits,
		llmClient:    llmClient,
		analysisServ: analysisServ,
	}
}

// CreateUser maneja POST /users.
func (h *Handlers) CreateUser(c *gin.Context) {
	var req struct {
		Email       string `json:"email" binding:"required,email"`
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid create user request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user := domain.User{
		ID:          uuid.NewString(),
		Email:       req.Email,
		DisplayName: req.DisplayName,
		CreatedAt:   time.Now().UTC(),
	}

	if err := h.users.Create(c.Request.Context(), user); err != nil {
		h.logger.Error("create user failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"user": user})
}

// InitClone maneja POST /clone/init.
func (h *Handlers) InitClone(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id" binding:"required"`
		Name   string `json:"name" binding:"required"`
		Bio    string `json:"bio"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid init clone request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	profile := domain.CloneProfile{
		ID:        uuid.NewString(),
		UserID:    req.UserID,
		Name:      req.Name,
		Bio:       req.Bio,
		CreatedAt: time.Now().UTC(),
	}

	if err := h.profiles.Create(c.Request.Context(), profile); err != nil {
		h.logger.Error("create clone profile failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not init clone"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"profile": profile})
}

// CreateSession maneja POST /session.
func (h *Handlers) CreateSession(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid create session request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	session := domain.Session{
		ID:        uuid.NewString(),
		UserID:    req.UserID,
		Token:     uuid.NewString(),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		CreatedAt: time.Now().UTC(),
	}

	if err := h.sessions.Create(c.Request.Context(), session); err != nil {
		h.logger.Error("create session failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create session"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"session": session})
}

// PostMessage maneja POST /message.
func (h *Handlers) PostMessage(c *gin.Context) {
	var req struct {
		UserID    string `json:"user_id" binding:"required"`
		SessionID string `json:"session_id"`
		Content   string `json:"content" binding:"required"`
		Role      string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid post message request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	msg := domain.Message{
		ID:        uuid.NewString(),
		UserID:    req.UserID,
		SessionID: req.SessionID,
		Content:   req.Content,
		Role:      req.Role,
		CreatedAt: time.Now().UTC(),
	}

	if err := h.messages.Create(c.Request.Context(), msg); err != nil {
		h.logger.Error("create message failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not post message"})
		return
	}

	// Lanzamos el análisis de manera asíncrona para no bloquear al usuario.
	go func(userID, content string) {
		h.logger.Info("analysis started", zap.String("user_id", userID))
		if err := h.analysisServ.AnalyzeAndPersist(context.Background(), userID, content); err != nil {
			h.logger.Warn("analysis failed", zap.Error(err), zap.String("user_id", userID))
			return
		}
		h.logger.Info("analysis finished", zap.String("user_id", userID))
	}(req.UserID, req.Content)

	c.JSON(http.StatusCreated, gin.H{"message": msg})
}

// GetCloneProfile maneja GET /clone/profile y devuelve el perfil psicológico del clon.
func (h *Handlers) GetCloneProfile(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	profile, err := h.profiles.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}
		h.logger.Error("get profile failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch profile"})
		return
	}

	traits, err := h.traits.FindByProfileID(c.Request.Context(), profile.ID)
	if err != nil {
		h.logger.Error("get traits failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch traits"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"profile": profile,
		"traits":  traits,
	})
}
