package service

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"clone-llm/internal/domain"
)

func TestJWTService_GenerateParseAccess(t *testing.T) {
	svc := NewJWTServiceWithStore("secret", 15*time.Minute, 30*time.Minute, NewMemoryRefreshTokenStore())
	user := domain.User{
		ID:          "u1",
		Email:       "user@example.com",
		DisplayName: "Test",
		CreatedAt:   time.Now().UTC(),
	}

	pair, err := svc.GeneratePair(user)
	if err != nil {
		t.Fatalf("generate pair: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("expected tokens")
	}

	claims, err := svc.ParseAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("parse access: %v", err)
	}
	if claims.UserID != "u1" || claims.Email != "user@example.com" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestJWTService_RefreshRotation(t *testing.T) {
	svc := NewJWTServiceWithStore("secret", 15*time.Minute, 30*time.Minute, NewMemoryRefreshTokenStore())
	user := domain.User{
		ID:        "u1",
		Email:     "user@example.com",
		CreatedAt: time.Now().UTC(),
	}

	pair, err := svc.GeneratePair(user)
	if err != nil {
		t.Fatalf("generate pair: %v", err)
	}

	refreshed, err := svc.RefreshPair(pair.RefreshToken)
	if err != nil {
		t.Fatalf("refresh pair: %v", err)
	}
	if refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatalf("expected refreshed tokens")
	}

	_, err = svc.RefreshPair(pair.RefreshToken)
	if err == nil {
		t.Fatalf("expected old refresh token to be revoked")
	}
}

func TestJWTService_RevokeRefresh(t *testing.T) {
	svc := NewJWTServiceWithStore("secret", 15*time.Minute, 30*time.Minute, NewMemoryRefreshTokenStore())
	user := domain.User{
		ID:        "u1",
		Email:     "user@example.com",
		CreatedAt: time.Now().UTC(),
	}
	pair, err := svc.GeneratePair(user)
	if err != nil {
		t.Fatalf("generate pair: %v", err)
	}

	if err := svc.RevokeRefresh(pair.RefreshToken); err != nil {
		t.Fatalf("revoke refresh: %v", err)
	}
	if _, err := svc.RefreshPair(pair.RefreshToken); err == nil {
		t.Fatalf("expected refresh to fail after revoke")
	}
}

func TestJWTService_RejectsEmptySecret(t *testing.T) {
	svc := NewJWTServiceWithStore("", 15*time.Minute, 30*time.Minute, NewMemoryRefreshTokenStore())
	user := domain.User{ID: "u1", Email: "user@example.com", CreatedAt: time.Now().UTC()}

	if _, err := svc.GeneratePair(user); !errors.Is(err, ErrJWTInvalid) {
		t.Fatalf("expected ErrJWTInvalid on empty secret, got %v", err)
	}
}

func TestJWTService_RejectsAccessTokenInRefreshFlow(t *testing.T) {
	svc := NewJWTServiceWithStore("secret", 15*time.Minute, 30*time.Minute, NewMemoryRefreshTokenStore())
	user := domain.User{ID: "u1", Email: "user@example.com", CreatedAt: time.Now().UTC()}
	pair, err := svc.GeneratePair(user)
	if err != nil {
		t.Fatalf("generate pair: %v", err)
	}

	if _, err := svc.RefreshPair(pair.AccessToken); !errors.Is(err, ErrJWTInvalid) {
		t.Fatalf("expected ErrJWTInvalid for access token used as refresh, got %v", err)
	}
}

func TestJWTService_RejectsWrongIssuer(t *testing.T) {
	svc := NewJWTServiceWithStore("secret", 15*time.Minute, 30*time.Minute, NewMemoryRefreshTokenStore())
	now := time.Now().UTC()
	claims := Claims{
		UserID:    "u1",
		Email:     "user@example.com",
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "other-issuer",
			Subject:   "u1",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if _, err := svc.ParseAccessToken(signed); !errors.Is(err, ErrJWTInvalid) {
		t.Fatalf("expected ErrJWTInvalid for wrong issuer, got %v", err)
	}
}
