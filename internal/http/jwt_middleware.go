package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"clone-llm/internal/service"
)

const authClaimsKey = "auth_claims"

// JWTAuthMiddleware valida JWT access tokens y guarda claims en el contexto.
func JWTAuthMiddleware(jwtSvc *service.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if jwtSvc == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "jwt not configured"})
			c.Abort()
			return
		}

		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if header == "" || !strings.HasPrefix(strings.ToLower(header), "bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			c.Abort()
			return
		}

		token := strings.TrimSpace(header[len("Bearer "):])
		claims, err := jwtSvc.ParseAccessToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		c.Set(authClaimsKey, claims)
		c.Next()
	}
}

// GetAuthClaims obtiene claims de JWT desde el contexto.
func GetAuthClaims(c *gin.Context) (service.Claims, bool) {
	val, ok := c.Get(authClaimsKey)
	if !ok {
		return service.Claims{}, false
	}
	claims, ok := val.(service.Claims)
	return claims, ok
}
