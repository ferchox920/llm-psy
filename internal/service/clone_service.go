package service

import (
	"context"

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

func (s *CloneService) ClonePrompt(ctx context.Context, prompt string) (string, error) {
	response, err := s.llmClient.Generate(ctx, prompt)
	if err != nil {
		return "", err
	}
	// TODO: persistir el intercambio en messageRepo.
	_ = response
	return response, nil
}
