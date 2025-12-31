package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NewRouter configura el router de Gin con middlewares y rutas base.
func NewRouter(
	logger *zap.Logger,
	userH *UserHandler,
	chatH *ChatHandler,
	cloneH *CloneHandler,
) *gin.Engine {
	r := gin.New()

	// Middlewares basicos: logging, recovery y JSON content-type.
	r.Use(zapLoggerMiddleware(logger), gin.Recovery(), jsonContentTypeMiddleware())

	// Rutas Sprint 1.
	users := r.Group("/users")
	users.POST("", userH.CreateUser)

	auth := r.Group("/auth")
	auth.POST("/otp/request", userH.RequestOTP)
	auth.POST("/otp/verify", userH.VerifyOTP)
	auth.POST("/oauth", userH.OAuthLogin)

	clone := r.Group("/clone")
	clone.POST("/init", cloneH.InitClone)
	clone.GET("/profile", cloneH.GetCloneProfile)

	r.POST("/session", chatH.CreateSession)
	r.POST("/message", chatH.PostMessage)

	return r
}

// zapLoggerMiddleware crea un middleware simple de logging con zap.
func zapLoggerMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
		)
	}
}

// jsonContentTypeMiddleware fuerza Content-Type: application/json en responses.
func jsonContentTypeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "application/json")
		c.Next()
	}
}
