package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
)

// UserHandler mantiene dependencias para endpoints de usuarios.
type UserHandler struct {
	logger *zap.Logger
	users  repository.UserRepository
}

// NewUserHandler crea una instancia de UserHandler con dependencias necesarias.
func NewUserHandler(logger *zap.Logger, users repository.UserRepository) *UserHandler {
	return &UserHandler{
		logger: logger,
		users:  users,
	}
}

// CreateUser maneja POST /users.
func (h *UserHandler) CreateUser(c *gin.Context) {
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
