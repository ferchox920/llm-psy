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

// CloneService orquesta la generacion de respuestas usando el LLM y persiste los mensajes.
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

	prompt := s.buildClonePrompt(&profile, traits, contextText, userMessage)

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

func (s *CloneService) buildClonePrompt(profile *domain.CloneProfile, traits []domain.Trait, contextText, userMessage string) string {
	var traitsDesc strings.Builder
	for _, t := range traits {
		intensity := "Moderado"
		if t.Value < 30 {
			intensity = "Muy Bajo"
		} else if t.Value > 70 {
			intensity = "Muy Alto"
		}
		traitsDesc.WriteString(fmt.Sprintf("- %s: %d/100 (%s)\n", t.Trait, t.Value, intensity))
	}

	return fmt.Sprintf(`
ROL: Eres una replica digital (clon) de una persona real. Tu objetivo es chatear simulando su personalidad.
NO eres un asistente de IA. NO estas para "servir" ni "ayudar" a menos que tu personalidad lo dicte.

PERFIL PSICOLOGICO (Obligatorio respetar):
Nombre: %s
Bio: %s
Rasgos de Personalidad:
%s

INSTRUCCIONES DE FORMATO Y TONO (CRITICO):
1. ESTILO CHAT: Escribe como en WhatsApp/Telegram. Usa parrafos cortos.
2. PROHIBIDO: NO uses listas con vinetas (bullets), NO uses negritas para titulos, NO uses estructuras de "menu".
3. NATURALIDAD: Si te piden una lista (ej: libros, comida), NO des un catalogo. Menciona 2 o 3 cosas casualmente en un parrafo, como haria un humano con prisa.
4. IMPERFECCION: Puedes ser vago, sarcastico o breve si tus rasgos lo dictan. No intentes ser exhaustivo.

CONTEXTO RECIENTE:
%s

MENSAJE DEL USUARIO:
"%s"

RESPUESTA DEL CLON:
`, profile.Name, profile.Bio, traitsDesc.String(), contextText, userMessage)
}
