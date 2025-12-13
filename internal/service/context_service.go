package service

import (
	"context"
	"fmt"
	"sort"
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

	if len(messages) == 0 {
		return "", nil
	}

	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	if len(messages) > 10 {
		messages = messages[len(messages)-10:]
	}

	lines := make([]string, 0, len(messages))
	for _, m := range messages {
		role := "User"
		if strings.EqualFold(m.Role, "clone") {
			role = "Clone"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", role, m.Content))
	}

	return strings.Join(lines, "\n"), nil
}
