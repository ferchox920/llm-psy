package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
	"clone-llm/internal/repository"
)

// CloneService orquesta la generación de respuestas usando el LLM y persiste los mensajes.
type CloneService struct {
	llmClient   llm.LLMClient
	messageRepo repository.MessageRepository
}

func NewCloneService(llmClient llm.LLMClient, messageRepo repository.MessageRepository) *CloneService {
	return &CloneService{
		llmClient:   llmClient,
		messageRepo: messageRepo,
	}
}

func (s *CloneService) ClonePrompt(ctx context.Context, userID, sessionID, prompt string) (string, error) {
	response, err := s.llmClient.Generate(ctx, prompt)
	if err != nil {
		return "", err
	}

	cloneMessage := domain.Message{
		ID:        uuid.NewString(),
		UserID:    userID,
		SessionID: sessionID,
		Content:   response,
		Role:      "clone",
		CreatedAt: time.Now().UTC(),
	}

	if err := s.messageRepo.Create(ctx, cloneMessage); err != nil {
		return "", err
	}

	return response, nil
}
