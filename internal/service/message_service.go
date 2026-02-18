package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
)

// MessageService encapsula la lógica para manejar mensajes de usuarios.
type MessageService struct {
	repo repository.MessageRepository
}

var (
	ErrMessageServiceNotConfigured = errors.New("message service not configured")
	ErrMessageInvalidInput         = errors.New("message invalid input")
)

func NewMessageService(repo repository.MessageRepository) *MessageService {
	return &MessageService{repo: repo}
}

func (s *MessageService) Save(ctx context.Context, msg domain.Message) error {
	if s == nil || s.repo == nil {
		return ErrMessageServiceNotConfigured
	}

	msg.UserID = strings.TrimSpace(msg.UserID)
	msg.SessionID = strings.TrimSpace(msg.SessionID)
	msg.Role = strings.TrimSpace(msg.Role)
	msg.Content = strings.TrimSpace(msg.Content)

	if msg.UserID == "" || msg.Role == "" || msg.Content == "" {
		return ErrMessageInvalidInput
	}
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}

	return s.repo.Create(ctx, msg)
}

func (s *MessageService) ListBySession(ctx context.Context, sessionID string) ([]domain.Message, error) {
	if s == nil || s.repo == nil {
		return nil, ErrMessageServiceNotConfigured
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return []domain.Message{}, nil
	}
	return s.repo.ListBySessionID(ctx, sessionID)
}
