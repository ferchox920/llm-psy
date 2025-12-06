package service

import (
	"context"
	"fmt"
	"strings"

	"clone-llm/internal/repository"
)

// ContextService define contrato para recuperar contexto conversacional.
type ContextService interface {
	GetContext(ctx context.Context, sessionID string) (string, error)
}

// BasicContextService obtiene los últimos mensajes y los formatea como texto plano.
type BasicContextService struct {
	messageRepo repository.MessageRepository
}

func NewBasicContextService(messageRepo repository.MessageRepository) *BasicContextService {
	return &BasicContextService{messageRepo: messageRepo}
}

func (s *BasicContextService) GetContext(ctx context.Context, sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", nil
	}

	messages, err := s.messageRepo.ListBySessionID(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("list messages: %w", err)
	}

	if len(messages) > 10 {
		messages = messages[len(messages)-10:]
	}

	// Asegura orden cronológico.
	if len(messages) >= 2 && messages[0].CreatedAt.After(messages[len(messages)-1].CreatedAt) {
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}
	}

	lines := make([]string, 0, len(messages))
	for _, m := range messages {
		role := strings.Title(m.Role)
		if strings.EqualFold(m.Role, "user") {
			role = "User"
		} else if strings.EqualFold(m.Role, "clone") {
			role = "Clone"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", role, m.Content))
	}

	return strings.Join(lines, "\n"), nil
}
