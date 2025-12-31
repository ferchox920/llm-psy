package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
)

// CloneHandler mantiene dependencias para endpoints del clon.
type CloneHandler struct {
	logger   *zap.Logger
	profiles repository.ProfileRepository
	traits   repository.TraitRepository
}

// NewCloneHandler crea una instancia de CloneHandler con dependencias necesarias.
func NewCloneHandler(
	logger *zap.Logger,
	profiles repository.ProfileRepository,
	traits repository.TraitRepository,
) *CloneHandler {
	return &CloneHandler{
		logger:   logger,
		profiles: profiles,
		traits:   traits,
	}
}

// InitClone maneja POST /clone/init.
func (h *CloneHandler) InitClone(c *gin.Context) {
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

// GetCloneProfile maneja GET /clone/profile y devuelve el perfil psicologico del clon.
func (h *CloneHandler) GetCloneProfile(c *gin.Context) {
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
