package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
	"clone-llm/internal/repository"
)

// CloneService orquesta la generación de respuestas usando el LLM y persiste los mensajes.
type CloneService struct {
	llmClient      llm.LLMClient
	messageRepo    repository.MessageRepository
	profileRepo    repository.ProfileRepository
	traitRepo      repository.TraitRepository
	contextService ContextService
}

func NewCloneService(
	llmClient llm.LLMClient,
	messageRepo repository.MessageRepository,
	profileRepo repository.ProfileRepository,
	traitRepo repository.TraitRepository,
	contextService ContextService,
) *CloneService {
	return &CloneService{
		llmClient:      llmClient,
		messageRepo:    messageRepo,
		profileRepo:    profileRepo,
		traitRepo:      traitRepo,
		contextService: contextService,
	}
}

// Chat genera una respuesta del clon basada en perfil, rasgos y contexto, la persiste y devuelve el mensaje completo.
func (s *CloneService) Chat(ctx context.Context, userID, sessionID, userMessage string) (domain.Message, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return domain.Message{}, fmt.Errorf("get profile: %w", err)
	}

	traits, err := s.traitRepo.FindByProfileID(ctx, profile.ID)
	if err != nil {
		return domain.Message{}, fmt.Errorf("get traits: %w", err)
	}

	contextText, err := s.contextService.GetContext(ctx, sessionID)
	if err != nil {
		return domain.Message{}, fmt.Errorf("get context: %w", err)
	}

	prompt := buildClonePrompt(profile, traits, contextText, userMessage)

	response, err := s.llmClient.Generate(ctx, prompt)
	if err != nil {
		return domain.Message{}, fmt.Errorf("llm generate: %w", err)
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
		return domain.Message{}, fmt.Errorf("persist clone message: %w", err)
	}

	return cloneMessage, nil
}

func buildClonePrompt(profile domain.CloneProfile, traits []domain.Trait, contextText, userMessage string) string {
	var traitsParts []string
	for _, t := range traits {
		traitsParts = append(traitsParts, fmt.Sprintf("%s: %d/100", titleCase(t.Trait), t.Value))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Eres %s.\n", profile.Name))
	if strings.TrimSpace(profile.Bio) != "" {
		sb.WriteString(fmt.Sprintf("Bio: %s\n", profile.Bio))
	}
	if len(traitsParts) > 0 {
		sb.WriteString("Rasgos (modelo Big Five): ")
		sb.WriteString(strings.Join(traitsParts, ", "))
		sb.WriteString("\n")
	}
	sb.WriteString("Estás en una conversación.\n")
	if strings.TrimSpace(contextText) != "" {
		sb.WriteString("Historial reciente:\n")
		sb.WriteString(contextText)
		sb.WriteString("\n")
	}
	sb.WriteString("Responde al último mensaje del usuario manteniendo el tono y personalidad descritos.\n")
	sb.WriteString("Mensaje del usuario:\n")
	sb.WriteString(userMessage)
	return sb.String()
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}
