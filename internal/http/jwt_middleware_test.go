package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"clone-llm/internal/domain"
	"clone-llm/internal/service"
)

func TestJWTAuthMiddleware_AllowsValidAccessToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtSvc := service.NewJWTServiceWithStore("secret", 15*time.Minute, 30*time.Minute, service.NewMemoryRefreshTokenStore())
	user := domain.User{ID: "u1", Email: "user@example.com", CreatedAt: time.Now().UTC()}
	pair, err := jwtSvc.GeneratePair(user)
	if err != nil {
		t.Fatalf("generate pair: %v", err)
	}

	r := gin.New()
	r.GET("/protected", JWTAuthMiddleware(jwtSvc), func(c *gin.Context) {
		claims, ok := GetAuthClaims(c)
		if !ok || claims.UserID != "u1" {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestJWTAuthMiddleware_RejectsMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtSvc := service.NewJWTServiceWithStore("secret", 15*time.Minute, 30*time.Minute, service.NewMemoryRefreshTokenStore())

	r := gin.New()
	r.GET("/protected", JWTAuthMiddleware(jwtSvc), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

