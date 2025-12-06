package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NewRouter configura el router de Gin con middlewares y rutas base.
func NewRouter(logger *zap.Logger, handlers *Handlers) *gin.Engine {
	r := gin.New()

	// Middlewares básicos: logging, recovery y JSON content-type.
	r.Use(zapLoggerMiddleware(logger), gin.Recovery(), jsonContentTypeMiddleware())

	// Rutas Sprint 1.
	r.POST("/users", handlers.CreateUser)
	r.POST("/clone/init", handlers.InitClone)
	r.GET("/clone/profile", handlers.GetCloneProfile)
	r.POST("/session", handlers.CreateSession)
	r.POST("/message", handlers.PostMessage)

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
