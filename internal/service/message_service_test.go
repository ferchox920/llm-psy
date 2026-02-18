package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"clone-llm/internal/domain"
)

type mockMessageServiceRepo struct {
	lastCreated domain.Message
	createErr   error
	listData    []domain.Message
	listErr     error
	lastSession string
}

func (m *mockMessageServiceRepo) Create(_ context.Context, message domain.Message) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.lastCreated = message
	return nil
}

func (m *mockMessageServiceRepo) ListBySessionID(_ context.Context, sessionID string) ([]domain.Message, error) {
	m.lastSession = sessionID
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listData, nil
}

func TestMessageServiceSave_NormalizesAndDefaults(t *testing.T) {
	repo := &mockMessageServiceRepo{}
	svc := NewMessageService(repo)

	err := svc.Save(context.Background(), domain.Message{
		UserID:    " u1 ",
		SessionID: " s1 ",
		Role:      " clone ",
		Content:   " hola ",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.lastCreated.ID == "" {
		t.Fatalf("expected generated id")
	}
	if repo.lastCreated.CreatedAt.IsZero() {
		t.Fatalf("expected created_at default")
	}
	if repo.lastCreated.UserID != "u1" || repo.lastCreated.SessionID != "s1" {
		t.Fatalf("expected trimmed ids, got user=%q session=%q", repo.lastCreated.UserID, repo.lastCreated.SessionID)
	}
	if repo.lastCreated.Role != "clone" || repo.lastCreated.Content != "hola" {
		t.Fatalf("expected trimmed role/content, got role=%q content=%q", repo.lastCreated.Role, repo.lastCreated.Content)
	}
}

func TestMessageServiceSave_Validation(t *testing.T) {
	repo := &mockMessageServiceRepo{}
	svc := NewMessageService(repo)

	cases := []domain.Message{
		{Role: "user", Content: "hola"},
		{UserID: "u1", Content: "hola"},
		{UserID: "u1", Role: "user"},
	}
	for i, c := range cases {
		if err := svc.Save(context.Background(), c); !errors.Is(err, ErrMessageInvalidInput) {
			t.Fatalf("case %d expected ErrMessageInvalidInput, got %v", i, err)
		}
	}
}

func TestMessageServiceSave_PreservesExplicitFields(t *testing.T) {
	repo := &mockMessageServiceRepo{}
	svc := NewMessageService(repo)
	now := time.Now().UTC().Add(-time.Minute)

	msg := domain.Message{
		ID:        "m1",
		UserID:    "u1",
		SessionID: "s1",
		Role:      "user",
		Content:   "hola",
		CreatedAt: now,
	}
	if err := svc.Save(context.Background(), msg); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.lastCreated.ID != "m1" || !repo.lastCreated.CreatedAt.Equal(now) {
		t.Fatalf("expected explicit id/created_at preserved")
	}
}

func TestMessageServiceListBySession(t *testing.T) {
	repo := &mockMessageServiceRepo{
		listData: []domain.Message{{ID: "m1"}, {ID: "m2"}},
	}
	svc := NewMessageService(repo)

	out, err := svc.ListBySession(context.Background(), " s1 ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.lastSession != "s1" {
		t.Fatalf("expected trimmed session, got %q", repo.lastSession)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out))
	}
}

func TestMessageServiceListBySession_EmptySession(t *testing.T) {
	repo := &mockMessageServiceRepo{}
	svc := NewMessageService(repo)
	out, err := svc.ListBySession(context.Background(), "  ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("expected empty list, got %+v", out)
	}
}

func TestMessageService_NotConfigured(t *testing.T) {
	var svc *MessageService
	if err := svc.Save(context.Background(), domain.Message{}); !errors.Is(err, ErrMessageServiceNotConfigured) {
		t.Fatalf("expected ErrMessageServiceNotConfigured, got %v", err)
	}

	svc = NewMessageService(nil)
	if _, err := svc.ListBySession(context.Background(), "s1"); !errors.Is(err, ErrMessageServiceNotConfigured) {
		t.Fatalf("expected ErrMessageServiceNotConfigured, got %v", err)
	}
}
