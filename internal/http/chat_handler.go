package http

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
	"clone-llm/internal/service"
)

// ChatHandler mantiene dependencias para endpoints de sesiones y mensajes.
type ChatHandler struct {
	logger       *zap.Logger
	sessions     repository.SessionRepository
	messages     repository.MessageRepository
	analysisServ *service.AnalysisService
	cloneServ    *service.CloneService
}

// NewChatHandler crea una instancia de ChatHandler con dependencias necesarias.
func NewChatHandler(
	logger *zap.Logger,
	sessions repository.SessionRepository,
	messages repository.MessageRepository,
	analysisServ *service.AnalysisService,
	cloneServ *service.CloneService,
) *ChatHandler {
	return &ChatHandler{
		logger:       logger,
		sessions:     sessions,
		messages:     messages,
		analysisServ: analysisServ,
		cloneServ:    cloneServ,
	}
}

// CreateSession maneja POST /session.
func (h *ChatHandler) CreateSession(c *gin.Context) {
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
func (h *ChatHandler) PostMessage(c *gin.Context) {
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

	// Lanzamos el analisis de manera asincrona para no bloquear al usuario.
	go func(userID, content string) {
		h.logger.Info("analysis started", zap.String("user_id", userID))
		if err := h.analysisServ.AnalyzeAndPersist(context.Background(), userID, content); err != nil {
			h.logger.Warn("analysis failed", zap.Error(err), zap.String("user_id", userID))
			return
		}
		h.logger.Info("analysis finished", zap.String("user_id", userID))
	}(req.UserID, req.Content)

	cloneMsg, _, err := h.cloneServ.Chat(c.Request.Context(), req.UserID, req.SessionID, req.Content)
	if err != nil {
		h.logger.Error("clone response failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":        "could not generate clone response",
			"user_message": msg,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"user_message":  msg,
		"clone_message": cloneMsg,
	})
}
