package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"clone-llm/internal/service"
)

// UserHandler mantiene dependencias para endpoints de usuarios.
type UserHandler struct {
	logger   *zap.Logger
	userServ *service.UserService
}

// NewUserHandler crea una instancia de UserHandler con dependencias necesarias.
func NewUserHandler(logger *zap.Logger, userServ *service.UserService) *UserHandler {
	return &UserHandler{
		logger:   logger,
		userServ: userServ,
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

	user, err := h.userServ.CreateUser(c.Request.Context(), service.CreateUserInput{
		Email:       req.Email,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		h.logger.Error("create user failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"user": user})
}

// RequestOTP maneja POST /auth/otp/request.
func (h *UserHandler) RequestOTP(c *gin.Context) {
	var req struct {
		Email       string `json:"email" binding:"required,email"`
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid otp request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	_, err := h.userServ.RequestOTP(c.Request.Context(), req.Email, req.DisplayName)
	if err != nil {
		if errors.Is(err, service.ErrEmailSendFailure) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "email delivery unavailable"})
			return
		}
		if errors.Is(err, service.ErrRateLimited) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}
		h.logger.Error("request otp failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not request otp"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "otp_sent"})
}

// VerifyOTP maneja POST /auth/otp/verify.
func (h *UserHandler) VerifyOTP(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Code  string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid otp verify request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user, err := h.userServ.VerifyOTP(c.Request.Context(), req.Email, req.Code)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		case errors.Is(err, service.ErrOTPNotRequested),
			errors.Is(err, service.ErrOTPExpired),
			errors.Is(err, service.ErrOTPInvalid):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		default:
			h.logger.Error("verify otp failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify otp"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// OAuthLogin maneja POST /auth/oauth.
func (h *UserHandler) OAuthLogin(c *gin.Context) {
	var req struct {
		Provider    string `json:"provider" binding:"required"`
		Subject     string `json:"subject" binding:"required"`
		Email       string `json:"email" binding:"email"`
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid oauth request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user, err := h.userServ.UpsertOAuthUser(c.Request.Context(), service.OAuthInput{
		Provider:    req.Provider,
		Subject:     req.Subject,
		Email:       req.Email,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		if errors.Is(err, service.ErrOAuthInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid oauth data"})
			return
		}
		h.logger.Error("oauth login failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not complete oauth"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}
