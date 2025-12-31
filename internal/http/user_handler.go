package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"clone-llm/internal/domain"
	"clone-llm/internal/service"
)

// UserHandler mantiene dependencias para endpoints de usuarios.
type UserHandler struct {
	logger   *zap.Logger
	userServ *service.UserService
	jwtServ  *service.JWTService
}

// NewUserHandler crea una instancia de UserHandler con dependencias necesarias.
func NewUserHandler(logger *zap.Logger, userServ *service.UserService, jwtServ *service.JWTService) *UserHandler {
	return &UserHandler{
		logger:   logger,
		userServ: userServ,
		jwtServ:  jwtServ,
	}
}

// CreateUser maneja POST /users.
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req struct {
		Email       string `json:"email" binding:"required,email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid create user request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user, err := h.userServ.CreateUser(c.Request.Context(), service.CreateUserInput{
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Password:    req.Password,
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

	tokens, err := h.issueTokens(user)
	if err != nil {
		h.logger.Error("jwt issue failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue tokens"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user, "tokens": tokens})
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

	tokens, err := h.issueTokens(user)
	if err != nil {
		h.logger.Error("jwt issue failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue tokens"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user, "tokens": tokens})
}

// Login maneja POST /auth/login.
func (h *UserHandler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid login request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user, err := h.userServ.Authenticate(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		h.logger.Error("login failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not login"})
		return
	}

	tokens, err := h.issueTokens(user)
	if err != nil {
		h.logger.Error("jwt issue failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue tokens"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user, "tokens": tokens})
}

// RefreshToken maneja POST /auth/refresh.
func (h *UserHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid refresh request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if h.jwtServ == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "jwt not configured"})
		return
	}
	tokens, err := h.jwtServ.RefreshPair(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tokens": tokens})
}

// Logout maneja POST /auth/logout.
func (h *UserHandler) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid logout request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if h.jwtServ == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "jwt not configured"})
		return
	}
	_ = h.jwtServ.RevokeRefresh(req.RefreshToken)
	c.Status(http.StatusNoContent)
}

func (h *UserHandler) issueTokens(user domain.User) (service.TokenPair, error) {
	if h.jwtServ == nil {
		return service.TokenPair{}, errors.New("jwt not configured")
	}
	return h.jwtServ.GeneratePair(user)
}
