package service

import (
	"testing"
	"time"

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

