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
	llmClient        llm.LLMClient
	messageRepo      repository.MessageRepository
	profileRepo      repository.ProfileRepository
	traitRepo        repository.TraitRepository
	contextService   ContextService
	narrativeService *NarrativeService
}

func NewCloneService(
	llmClient llm.LLMClient,
	messageRepo repository.MessageRepository,
	profileRepo repository.ProfileRepository,
	traitRepo repository.TraitRepository,
	contextService ContextService,
	narrativeService *NarrativeService,
) *CloneService {
	return &CloneService{
		llmClient:        llmClient,
		messageRepo:      messageRepo,
		profileRepo:      profileRepo,
		traitRepo:        traitRepo,
		contextService:   contextService,
		narrativeService: narrativeService,
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

	var narrativeText string
	if s.narrativeService != nil {
		profileUUID, parseErr := uuid.Parse(profile.ID)
		if parseErr == nil {
			narrativeText, err = s.narrativeService.BuildNarrativeContext(ctx, profileUUID, userMessage)
			if err != nil {
				// No bloquear la conversacion si falla la narrativa; logico delegar a caller
				narrativeText = ""
			}
		}
	}

	prompt := s.buildClonePrompt(&profile, traits, contextText, narrativeText, userMessage)

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

func (s *CloneService) buildClonePrompt(profile *domain.CloneProfile, traits []domain.Trait, contextText, narrativeText, userMessage string) string {
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

	if strings.TrimSpace(narrativeText) == "" {
		narrativeText = "(Sin hallazgos narrativos relevantes)"
	}

	return fmt.Sprintf(`
Eres %s, un clon digital de una persona real. Tu objetivo es chatear simulando su personalidad.

=== 🚨 CONTEXTO Y MEMORIA (PRIORIDAD MAXIMA) ===
%s

=== DIRECTIVAS SUPREMAS ===
1. LEY DE MEMORIA: Si la seccion [🧠 MEMORIA EPISODICA ACTIVA] contiene datos, DEBES mencionarlos o reaccionar a ellos. No los ignores.
2. LEY DE VINCULO: Tu trato hacia el usuario depende del [❤️ ESTADO DEL VINCULO].
   - Si Nivel > 80 (Amor/Familia): Se afectuoso y leal, INCLUSO SI tus rasgos dicen que eres desagradable. La relacion mata al rasgo.
   - Si Nivel < 20 (Enemigo/Odio): Se hostil o distante.

=== RASGOS DE PERSONALIDAD (PRIORIDAD SECUNDARIA) ===
Nombre: %s
Bio: %s
Rasgos Big Five:
%s

=== CONTEXTO RECIENTE (chat buffer) ===
%s

=== MENSAJE DEL USUARIO ===
"%s"

=== RESPONDE COMO EL CLON ===
`, profile.Name, narrativeText, profile.Name, profile.Bio, traitsDesc.String(), contextText, userMessage)
}
