package service

import (
	"context"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
)

// MessageService encapsula la lógica para manejar mensajes de usuarios.
type MessageService struct {
	repo repository.MessageRepository
}

func NewMessageService(repo repository.MessageRepository) *MessageService {
	return &MessageService{repo: repo}
}

func (s *MessageService) Save(ctx context.Context, msg domain.Message) error {
	return s.repo.Create(ctx, msg)
}

func (s *MessageService) ListBySession(ctx context.Context, sessionID string) ([]domain.Message, error) {
	return s.repo.ListBySessionID(ctx, sessionID)
}
