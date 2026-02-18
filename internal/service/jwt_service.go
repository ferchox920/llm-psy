package service

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"clone-llm/internal/domain"
)

// JWTService emite y valida tokens JWT.
type JWTService struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	issuer     string
	store      RefreshTokenStore
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type Claims struct {
	UserID        string `json:"uid"`
	Email         string `json:"email"`
	DisplayName   string `json:"display_name,omitempty"`
	AuthProvider  string `json:"auth_provider,omitempty"`
	EmailVerified bool   `json:"email_verified"`
	TokenType     string `json:"typ"`
	jwt.RegisteredClaims
}

var (
	ErrJWTInvalid = errors.New("jwt invalid")
	ErrJWTExpired = errors.New("jwt expired")
)

func NewJWTService(secret string, accessTTL, refreshTTL time.Duration) *JWTService {
	if accessTTL <= 0 {
		accessTTL = 15 * time.Minute
	}
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	return &JWTService{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		issuer:     "clone-llm",
		store:      NewMemoryRefreshTokenStore(),
	}
}

func NewJWTServiceWithStore(secret string, accessTTL, refreshTTL time.Duration, store RefreshTokenStore) *JWTService {
	svc := NewJWTService(secret, accessTTL, refreshTTL)
	if store != nil {
		svc.store = store
	}
	return svc
}

func (s *JWTService) GeneratePair(user domain.User) (TokenPair, error) {
	if len(s.secret) == 0 {
		return TokenPair{}, ErrJWTInvalid
	}
	now := time.Now().UTC()
	access, err := s.signToken(user, now, s.accessTTL, "access")
	if err != nil {
		return TokenPair{}, err
	}
	refresh, jti, err := s.signRefreshToken(user, now)
	if err != nil {
		return TokenPair{}, err
	}
	if s.store != nil {
		if err := s.store.Store(jti, user.ID, s.refreshTTL); err != nil {
			return TokenPair{}, err
		}
	}
	return TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(s.accessTTL.Seconds()),
	}, nil
}

func (s *JWTService) RefreshPair(refreshToken string) (TokenPair, error) {
	if len(s.secret) == 0 {
		return TokenPair{}, ErrJWTInvalid
	}
	if strings.TrimSpace(refreshToken) == "" {
		return TokenPair{}, ErrJWTInvalid
	}
	claims, err := s.parseToken(refreshToken)
	if err != nil {
		return TokenPair{}, err
	}
	if claims.TokenType != "refresh" {
		return TokenPair{}, ErrJWTInvalid
	}
	if !s.isValidClaims(claims) {
		return TokenPair{}, ErrJWTInvalid
	}
	if claims.ID == "" || s.store == nil {
		return TokenPair{}, ErrJWTInvalid
	}
	ok, err := s.store.Exists(claims.ID)
	if err != nil || !ok {
		return TokenPair{}, ErrJWTInvalid
	}
	if err := s.store.Revoke(claims.ID); err != nil {
		return TokenPair{}, ErrJWTInvalid
	}

	user := domain.User{
		ID:           claims.UserID,
		Email:        claims.Email,
		DisplayName:  claims.DisplayName,
		AuthProvider: claims.AuthProvider,
	}
	if claims.EmailVerified {
		now := time.Now().UTC()
		user.EmailVerifiedAt = &now
	}
	return s.GeneratePair(user)
}

func (s *JWTService) RevokeRefresh(refreshToken string) error {
	if len(s.secret) == 0 {
		return ErrJWTInvalid
	}
	if strings.TrimSpace(refreshToken) == "" {
		return ErrJWTInvalid
	}
	claims, err := s.parseToken(refreshToken)
	if err != nil {
		return err
	}
	if !s.isValidClaims(claims) {
		return ErrJWTInvalid
	}
	if claims.TokenType != "refresh" || claims.ID == "" {
		return ErrJWTInvalid
	}
	if s.store == nil {
		return ErrJWTInvalid
	}
	return s.store.Revoke(claims.ID)
}

func (s *JWTService) ParseAccessToken(accessToken string) (Claims, error) {
	if len(s.secret) == 0 {
		return Claims{}, ErrJWTInvalid
	}
	if strings.TrimSpace(accessToken) == "" {
		return Claims{}, ErrJWTInvalid
	}
	claims, err := s.parseToken(accessToken)
	if err != nil {
		return Claims{}, err
	}
	if claims.TokenType != "access" {
		return Claims{}, ErrJWTInvalid
	}
	if !s.isValidClaims(claims) {
		return Claims{}, ErrJWTInvalid
	}
	return claims, nil
}

func (s *JWTService) signToken(user domain.User, now time.Time, ttl time.Duration, tokenType string) (string, error) {
	emailVerified := user.EmailVerifiedAt != nil
	claims := Claims{
		UserID:        user.ID,
		Email:         user.Email,
		DisplayName:   user.DisplayName,
		AuthProvider:  user.AuthProvider,
		EmailVerified: emailVerified,
		TokenType:     tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *JWTService) signRefreshToken(user domain.User, now time.Time) (string, string, error) {
	jti := uuid.NewString()
	emailVerified := user.EmailVerifiedAt != nil
	claims := Claims{
		UserID:        user.ID,
		Email:         user.Email,
		DisplayName:   user.DisplayName,
		AuthProvider:  user.AuthProvider,
		EmailVerified: emailVerified,
		TokenType:     "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Issuer:    s.issuer,
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	return signed, jti, err
}

func (s *JWTService) parseToken(tokenString string) (Claims, error) {
	var claims Claims
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	_, err := parser.ParseWithClaims(tokenString, &claims, func(_ *jwt.Token) (any, error) {
		return s.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return Claims{}, ErrJWTExpired
		}
		return Claims{}, ErrJWTInvalid
	}
	return claims, nil
}

func (s *JWTService) isValidClaims(claims Claims) bool {
	if strings.TrimSpace(claims.UserID) == "" {
		return false
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return false
	}
	if claims.Subject != claims.UserID {
		return false
	}
	return strings.TrimSpace(claims.Issuer) == s.issuer
}
