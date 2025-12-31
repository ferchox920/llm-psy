package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"clone-llm/internal/domain"
	"clone-llm/internal/email"
	"clone-llm/internal/repository"
)

// UserService coordina reglas de negocio para usuarios.
type UserService struct {
	logger      *zap.Logger
	users       repository.UserRepository
	emailSender email.Sender
	otpLimiter  OTPRateLimiter
}

func NewUserService(logger *zap.Logger, users repository.UserRepository, emailSender email.Sender, otpLimiter OTPRateLimiter) *UserService {
	if otpLimiter == nil {
		otpLimiter = NewOTPRateLimiter(otpTTL, 3)
	}
	return &UserService{
		logger:      logger,
		users:       users,
		emailSender: emailSender,
		otpLimiter:  otpLimiter,
	}
}

type CreateUserInput struct {
	Email           string
	DisplayName     string
	AuthProvider    string
	AuthSubject     string
	PasswordHash    string
	EmailVerifiedAt *time.Time
	OtpCodeHash     string
	OtpExpiresAt    *time.Time
}

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrOTPNotRequested  = errors.New("otp not requested")
	ErrOTPExpired       = errors.New("otp expired")
	ErrOTPInvalid       = errors.New("otp invalid")
	ErrOAuthInvalid     = errors.New("oauth data invalid")
	ErrEmailSendFailure = errors.New("email send failed")
	ErrRateLimited      = errors.New("rate limited")
)

const otpTTL = 10 * time.Minute

func (s *UserService) CreateUser(ctx context.Context, input CreateUserInput) (domain.User, error) {
	email := strings.TrimSpace(input.Email)
	displayName := strings.TrimSpace(input.DisplayName)
	authProvider := strings.TrimSpace(input.AuthProvider)
	authSubject := strings.TrimSpace(input.AuthSubject)
	passwordHash := strings.TrimSpace(input.PasswordHash)

	// TODO: integrar verificacion de seguridad y OAuth en este flujo.
	user := domain.User{
		ID:              uuid.NewString(),
		Email:           email,
		DisplayName:     displayName,
		AuthProvider:    authProvider,
		AuthSubject:     authSubject,
		PasswordHash:    passwordHash,
		EmailVerifiedAt: input.EmailVerifiedAt,
		OtpCodeHash:     input.OtpCodeHash,
		OtpExpiresAt:    input.OtpExpiresAt,
		CreatedAt:       time.Now().UTC(),
	}

	if err := s.users.Create(ctx, user); err != nil {
		return domain.User{}, err
	}

	return user, nil
}

type OAuthInput struct {
	Provider    string
	Subject     string
	Email       string
	DisplayName string
}

func (s *UserService) UpsertOAuthUser(ctx context.Context, input OAuthInput) (domain.User, error) {
	provider := strings.TrimSpace(input.Provider)
	subject := strings.TrimSpace(input.Subject)
	emailAddr := strings.TrimSpace(input.Email)
	displayName := strings.TrimSpace(input.DisplayName)

	if provider == "" || subject == "" {
		return domain.User{}, ErrOAuthInvalid
	}

	user, err := s.users.GetByAuth(ctx, provider, subject)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, err
	}

	if emailAddr != "" {
		existing, err := s.users.GetByEmail(ctx, emailAddr)
		if err == nil {
			if err := s.users.LinkOAuth(ctx, existing.ID, provider, subject); err != nil {
				return domain.User{}, err
			}
			verifiedAt := time.Now().UTC()
			if err := s.users.VerifyEmail(ctx, existing.ID, verifiedAt); err != nil {
				return domain.User{}, err
			}
			existing.AuthProvider = provider
			existing.AuthSubject = subject
			existing.EmailVerifiedAt = &verifiedAt
			if displayName != "" && existing.DisplayName == "" {
				existing.DisplayName = displayName
			}
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, err
		}
	}

	verifiedAt := time.Now().UTC()
	user = domain.User{
		ID:              uuid.NewString(),
		Email:           emailAddr,
		DisplayName:     displayName,
		AuthProvider:    provider,
		AuthSubject:     subject,
		EmailVerifiedAt: &verifiedAt,
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.users.Create(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *UserService) RequestOTP(ctx context.Context, emailAddr, displayName string) (domain.User, error) {
	emailAddr = strings.TrimSpace(emailAddr)
	displayName = strings.TrimSpace(displayName)
	if emailAddr == "" {
		return domain.User{}, fmt.Errorf("email is required")
	}

	if s.otpLimiter != nil && !s.otpLimiter.Allow(emailAddr) {
		return domain.User{}, ErrRateLimited
	}

	user, err := s.users.GetByEmail(ctx, emailAddr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			user = domain.User{
				ID:          uuid.NewString(),
				Email:       emailAddr,
				DisplayName: displayName,
				CreatedAt:   time.Now().UTC(),
			}
			if err := s.users.Create(ctx, user); err != nil {
				return domain.User{}, err
			}
		} else {
			return domain.User{}, err
		}
	}

	code, hash, expiresAt, err := generateOTP()
	if err != nil {
		return domain.User{}, err
	}

	if err := s.users.UpdateOTP(ctx, user.ID, hash, expiresAt); err != nil {
		return domain.User{}, err
	}

	if s.emailSender == nil {
		return domain.User{}, ErrEmailSendFailure
	}
	if err := s.emailSender.SendVerificationOTP(ctx, emailAddr, code, expiresAt); err != nil {
		s.logger.Warn("send verification otp failed", zap.Error(err), zap.String("email", emailAddr))
		return domain.User{}, ErrEmailSendFailure
	}

	user.OtpExpiresAt = &expiresAt
	return user, nil
}

func (s *UserService) VerifyOTP(ctx context.Context, emailAddr, code string) (domain.User, error) {
	emailAddr = strings.TrimSpace(emailAddr)
	code = strings.TrimSpace(code)
	if emailAddr == "" || code == "" {
		return domain.User{}, ErrOTPInvalid
	}

	user, err := s.users.GetByEmail(ctx, emailAddr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, ErrUserNotFound
		}
		return domain.User{}, err
	}

	if user.OtpCodeHash == "" || user.OtpExpiresAt == nil {
		return domain.User{}, ErrOTPNotRequested
	}
	if time.Now().UTC().After(*user.OtpExpiresAt) {
		return domain.User{}, ErrOTPExpired
	}
	if !verifyOTP(code, user.OtpCodeHash) {
		return domain.User{}, ErrOTPInvalid
	}

	verifiedAt := time.Now().UTC()
	if err := s.users.VerifyEmail(ctx, user.ID, verifiedAt); err != nil {
		return domain.User{}, err
	}

	user.EmailVerifiedAt = &verifiedAt
	user.OtpCodeHash = ""
	user.OtpExpiresAt = nil
	return user, nil
}

func generateOTP() (string, string, time.Time, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", "", time.Time{}, err
	}
	code := fmt.Sprintf("%06d", n.Int64())

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", "", time.Time{}, err
	}
	saltStr := base64.StdEncoding.EncodeToString(salt)
	hashBytes := sha256.Sum256([]byte(saltStr + ":" + code))
	hash := base64.StdEncoding.EncodeToString(hashBytes[:])

	expiresAt := time.Now().UTC().Add(otpTTL)
	return code, saltStr + ":" + hash, expiresAt, nil
}

func verifyOTP(code, stored string) bool {
	parts := strings.Split(stored, ":")
	if len(parts) != 2 {
		return false
	}
	saltStr := parts[0]
	expectedHash := parts[1]
	hashBytes := sha256.Sum256([]byte(saltStr + ":" + code))
	hash := base64.StdEncoding.EncodeToString(hashBytes[:])
	return subtle.ConstantTimeCompare([]byte(hash), []byte(expectedHash)) == 1
}

// OTPRateLimiter limita la frecuencia de solicitudes de OTP por clave.
type OTPRateLimiter interface {
	Allow(key string) bool
}

type otpRateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	max    int
	hits   map[string][]time.Time
}

// NewOTPRateLimiter crea un rate limiter en memoria.
func NewOTPRateLimiter(window time.Duration, max int) OTPRateLimiter {
	if max <= 0 {
		max = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &otpRateLimiter{
		window: window,
		max:    max,
		hits:   make(map[string][]time.Time),
	}
}

func (l *otpRateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now().UTC()
	cutoff := now.Add(-l.window)
	entries := l.hits[key]
	kept := entries[:0]
	for _, ts := range entries {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= l.max {
		l.hits[key] = kept
		return false
	}
	kept = append(kept, now)
	l.hits[key] = kept
	return true
}
