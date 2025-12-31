package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"clone-llm/internal/domain"
)

type mockUserRepo struct {
	usersByID    map[string]domain.User
	usersByEmail map[string]string
	usersByAuth  map[string]string
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		usersByID:    make(map[string]domain.User),
		usersByEmail: make(map[string]string),
		usersByAuth:  make(map[string]string),
	}
}

func (m *mockUserRepo) Create(_ context.Context, user domain.User) error {
	m.usersByID[user.ID] = user
	if user.Email != "" {
		m.usersByEmail[user.Email] = user.ID
	}
	if user.AuthProvider != "" && user.AuthSubject != "" {
		key := user.AuthProvider + "|" + user.AuthSubject
		m.usersByAuth[key] = user.ID
	}
	return nil
}

func (m *mockUserRepo) GetByID(_ context.Context, id string) (domain.User, error) {
	user, ok := m.usersByID[id]
	if !ok {
		return domain.User{}, pgx.ErrNoRows
	}
	return user, nil
}

func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (domain.User, error) {
	id, ok := m.usersByEmail[email]
	if !ok {
		return domain.User{}, pgx.ErrNoRows
	}
	return m.GetByID(context.Background(), id)
}

func (m *mockUserRepo) GetByAuth(_ context.Context, provider, subject string) (domain.User, error) {
	key := provider + "|" + subject
	id, ok := m.usersByAuth[key]
	if !ok {
		return domain.User{}, pgx.ErrNoRows
	}
	return m.GetByID(context.Background(), id)
}

func (m *mockUserRepo) UpdateOTP(_ context.Context, id, otpHash string, otpExpiresAt time.Time) error {
	user, ok := m.usersByID[id]
	if !ok {
		return pgx.ErrNoRows
	}
	user.OtpCodeHash = otpHash
	user.OtpExpiresAt = &otpExpiresAt
	m.usersByID[id] = user
	return nil
}

func (m *mockUserRepo) VerifyEmail(_ context.Context, id string, verifiedAt time.Time) error {
	user, ok := m.usersByID[id]
	if !ok {
		return pgx.ErrNoRows
	}
	user.EmailVerifiedAt = &verifiedAt
	user.OtpCodeHash = ""
	user.OtpExpiresAt = nil
	m.usersByID[id] = user
	return nil
}

func (m *mockUserRepo) LinkOAuth(_ context.Context, id, provider, subject string) error {
	user, ok := m.usersByID[id]
	if !ok {
		return pgx.ErrNoRows
	}
	user.AuthProvider = provider
	user.AuthSubject = subject
	m.usersByID[id] = user
	if provider != "" && subject != "" {
		key := provider + "|" + subject
		m.usersByAuth[key] = id
	}
	return nil
}

type mockEmailSender struct {
	lastTo      string
	lastCode    string
	lastExpires time.Time
	err         error
}

func (m *mockEmailSender) SendVerificationOTP(_ context.Context, toEmail string, code string, expiresAt time.Time) error {
	m.lastTo = toEmail
	m.lastCode = code
	m.lastExpires = expiresAt
	return m.err
}

func TestUserServiceRequestOTP_NewUser(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	svc := NewUserService(zap.NewNop(), repo, sender, nil)

	start := time.Now().UTC()
	user, err := svc.RequestOTP(context.Background(), "user@example.com", "Test")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if user.Email != "user@example.com" {
		t.Fatalf("expected email user@example.com, got %s", user.Email)
	}
	if sender.lastTo != "user@example.com" {
		t.Fatalf("expected email to be sent to user@example.com, got %s", sender.lastTo)
	}
	if sender.lastCode == "" {
		t.Fatalf("expected otp code to be sent")
	}
	if sender.lastExpires.Before(start.Add(9*time.Minute)) {
		t.Fatalf("expected otp expiry at least 9 minutes ahead, got %v", sender.lastExpires)
	}
	if sender.lastExpires.After(start.Add(11 * time.Minute)) {
		t.Fatalf("expected otp expiry around 10 minutes, got %v", sender.lastExpires)
	}

	stored, err := repo.GetByEmail(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("expected user stored, got %v", err)
	}
	if stored.OtpCodeHash == "" || stored.OtpExpiresAt == nil {
		t.Fatalf("expected otp to be stored")
	}
}

func TestUserServiceVerifyOTP_Success(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	svc := NewUserService(zap.NewNop(), repo, sender, nil)

	_, err := svc.RequestOTP(context.Background(), "user@example.com", "")
	if err != nil {
		t.Fatalf("expected request otp success, got %v", err)
	}
	if sender.lastCode == "" {
		t.Fatalf("expected code to be captured")
	}

	user, err := svc.VerifyOTP(context.Background(), "user@example.com", sender.lastCode)
	if err != nil {
		t.Fatalf("expected verify success, got %v", err)
	}
	if user.EmailVerifiedAt == nil {
		t.Fatalf("expected email verified")
	}

	stored, err := repo.GetByEmail(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("expected user stored, got %v", err)
	}
	if stored.OtpCodeHash != "" || stored.OtpExpiresAt != nil {
		t.Fatalf("expected otp cleared after verification")
	}
}

func TestUserServiceVerifyOTP_Expired(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	svc := NewUserService(zap.NewNop(), repo, sender, nil)

	code, hash, _, err := generateOTP()
	if err != nil {
		t.Fatalf("generate otp failed: %v", err)
	}
	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	user := domain.User{
		ID:          "u1",
		Email:       "user@example.com",
		OtpCodeHash: hash,
		OtpExpiresAt: &expiredAt,
		CreatedAt:   time.Now().UTC(),
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	_, err = svc.VerifyOTP(context.Background(), "user@example.com", code)
	if !errors.Is(err, ErrOTPExpired) {
		t.Fatalf("expected ErrOTPExpired, got %v", err)
	}
}

func TestUserServiceUpsertOAuthUser_LinksExistingByEmail(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	svc := NewUserService(zap.NewNop(), repo, sender, nil)

	user := domain.User{
		ID:        "u1",
		Email:     "user@example.com",
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	res, err := svc.UpsertOAuthUser(context.Background(), OAuthInput{
		Provider:    "google",
		Subject:     "sub-1",
		Email:       "user@example.com",
		DisplayName: "Test",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if res.AuthProvider != "google" || res.AuthSubject != "sub-1" {
		t.Fatalf("expected oauth linked")
	}
	if res.EmailVerifiedAt == nil {
		t.Fatalf("expected email verified")
	}

	stored, err := repo.GetByID(context.Background(), "u1")
	if err != nil {
		t.Fatalf("expected stored user, got %v", err)
	}
	if stored.AuthProvider != "google" || stored.AuthSubject != "sub-1" {
		t.Fatalf("expected stored oauth link")
	}
	if stored.EmailVerifiedAt == nil {
		t.Fatalf("expected stored email verified")
	}
}

func TestUserServiceUpsertOAuthUser_CreatesNew(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	svc := NewUserService(zap.NewNop(), repo, sender, nil)

	res, err := svc.UpsertOAuthUser(context.Background(), OAuthInput{
		Provider:    "github",
		Subject:     "sub-2",
		Email:       "new@example.com",
		DisplayName: "New",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if res.ID == "" || res.AuthProvider != "github" || res.AuthSubject != "sub-2" {
		t.Fatalf("expected new oauth user")
	}
	if res.EmailVerifiedAt == nil {
		t.Fatalf("expected email verified for oauth user")
	}
}

func TestUserServiceRequestOTP_EmailSendFailure(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{err: errors.New("smtp down")}
	svc := NewUserService(zap.NewNop(), repo, sender, nil)

	_, err := svc.RequestOTP(context.Background(), "user@example.com", "")
	if !errors.Is(err, ErrEmailSendFailure) {
		t.Fatalf("expected ErrEmailSendFailure, got %v", err)
	}
}

type mockLimiter struct {
	allow bool
}

func (m *mockLimiter) Allow(_ string) bool {
	return m.allow
}

func TestUserServiceRequestOTP_RateLimited(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	limiter := &mockLimiter{allow: false}
	svc := NewUserService(zap.NewNop(), repo, sender, limiter)

	_, err := svc.RequestOTP(context.Background(), "user@example.com", "")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}
