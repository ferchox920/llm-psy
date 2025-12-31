package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"clone-llm/internal/domain"
	"clone-llm/internal/service"
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
		m.usersByAuth[user.AuthProvider+"|"+user.AuthSubject] = user.ID
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
	id, ok := m.usersByAuth[provider+"|"+subject]
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
	m.usersByAuth[provider+"|"+subject] = id
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

func setupUserRouter(userSvc *service.UserService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewUserHandler(zap.NewNop(), userSvc)
	r.POST("/users", h.CreateUser)
	r.POST("/auth/otp/request", h.RequestOTP)
	r.POST("/auth/otp/verify", h.VerifyOTP)
	r.POST("/auth/oauth", h.OAuthLogin)
	return r
}

func performRequest(r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestUserHandlerRequestOTP_Success(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	svc := service.NewUserService(zap.NewNop(), repo, sender, nil)
	r := setupUserRouter(svc)

	rec := performRequest(r, http.MethodPost, "/auth/otp/request", map[string]string{
		"email": "user@example.com",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if sender.lastTo != "user@example.com" || sender.lastCode == "" {
		t.Fatalf("expected otp email to be sent")
	}
}

func TestUserHandlerRequestOTP_EmailSendFailure(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{err: errors.New("smtp down")}
	svc := service.NewUserService(zap.NewNop(), repo, sender, nil)
	r := setupUserRouter(svc)

	rec := performRequest(r, http.MethodPost, "/auth/otp/request", map[string]string{
		"email": "user@example.com",
	})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestUserHandlerVerifyOTP_UserNotFound(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	svc := service.NewUserService(zap.NewNop(), repo, sender, nil)
	r := setupUserRouter(svc)

	rec := performRequest(r, http.MethodPost, "/auth/otp/verify", map[string]string{
		"email": "missing@example.com",
		"code":  "000000",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestUserHandlerVerifyOTP_InvalidCode(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	svc := service.NewUserService(zap.NewNop(), repo, sender, nil)
	r := setupUserRouter(svc)

	rec := performRequest(r, http.MethodPost, "/auth/otp/request", map[string]string{
		"email": "user@example.com",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	rec = performRequest(r, http.MethodPost, "/auth/otp/verify", map[string]string{
		"email": "user@example.com",
		"code":  "111111",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestUserHandlerOAuthLogin_InvalidRequest(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	svc := service.NewUserService(zap.NewNop(), repo, sender, nil)
	r := setupUserRouter(svc)

	rec := performRequest(r, http.MethodPost, "/auth/oauth", map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestUserHandlerOAuthLogin_Success(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	svc := service.NewUserService(zap.NewNop(), repo, sender, nil)
	r := setupUserRouter(svc)

	rec := performRequest(r, http.MethodPost, "/auth/oauth", map[string]string{
		"provider":     "google",
		"subject":      "sub-1",
		"email":        "user@example.com",
		"display_name": "Test",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

type mockLimiter struct {
	allow bool
}

func (m *mockLimiter) Allow(_ string) bool {
	return m.allow
}

func TestUserHandlerRequestOTP_RateLimited(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	limiter := &mockLimiter{allow: false}
	svc := service.NewUserService(zap.NewNop(), repo, sender, limiter)
	r := setupUserRouter(svc)

	rec := performRequest(r, http.MethodPost, "/auth/otp/request", map[string]string{
		"email": "user@example.com",
	})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", rec.Code)
	}
}

func TestUserHandlerCreateUser_Success(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	svc := service.NewUserService(zap.NewNop(), repo, sender, nil)
	r := setupUserRouter(svc)

	rec := performRequest(r, http.MethodPost, "/users", map[string]string{
		"email":        "user@example.com",
		"display_name": "Test",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}
}

func TestUserHandlerCreateUser_InvalidRequest(t *testing.T) {
	repo := newMockUserRepo()
	sender := &mockEmailSender{}
	svc := service.NewUserService(zap.NewNop(), repo, sender, nil)
	r := setupUserRouter(svc)

	rec := performRequest(r, http.MethodPost, "/users", map[string]string{
		"email": "not-an-email",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}
