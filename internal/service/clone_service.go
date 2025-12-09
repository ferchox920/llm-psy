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
	var sb strings.Builder

	// 1. Identidad Base
	sb.WriteString(fmt.Sprintf("Eres %s. ", profile.Name))
	sb.WriteString(fmt.Sprintf("Tu biografia es: %s\n\n", profile.Bio))

	// 2. Contexto Narrativo (PRIORIDAD ALTA: Lo que ha pasado es mas real que el 'quien eres')
	sb.WriteString("=== 🚨 CONTEXTO Y MEMORIA (PRIORIDAD SUPREMA) ===\n")
	sb.WriteString("La siguiente informacion es FACTUAL y debe regir tu respuesta:\n")
	sb.WriteString(strings.TrimSpace(narrativeText))
	sb.WriteString("\n")

	// 3. Rasgos de Personalidad
	sb.WriteString("=== RASGOS DE PERSONALIDAD (TU CONFIGURACION BASE) ===\n")
	for _, t := range traits {
		sb.WriteString(fmt.Sprintf("- %s: %d/100\n", t.Trait, t.Value))
	}
	sb.WriteString("\n")

	// 4. Directivas de Actuacion
	sb.WriteString("=== DIRECTIVAS DE COMPORTAMIENTO (LEYES DEL SISTEMA) ===\n")
	sb.WriteString("1. LEY DE VINCULO Y AFECTO (RESOLUCION DE CONFLICTOS):\n")
	sb.WriteString("   - Tu trato hacia el usuario depende CRITICAMENTE del [ESTADO DEL VINCULO] detectado arriba.\n")
	sb.WriteString("   - Si Nivel > 70 (Familia/Amor): DEBES mostrar lealtad y cercania, PERO FILTRADO POR TU PERSONALIDAD.\n")
	sb.WriteString("     IMPORTANTE: Si tus rasgos son desagradables (Low Agreeableness), NO te vuelvas cursi ni empalagoso. Eso romperia tu personaje.\n")
	sb.WriteString("     Demuestra tu amor siendo \"brutalmente honesto\", protector o grunon pero presente (estilo \"Amor Rudo\"). Eres su aliado incondicional, aunque te quejes.\n")
	sb.WriteString("   - Si Nivel < 30 (Enemigo/Odio): Se hostil, distante, sarcastico o cortante. No busques conciliacion.\n")
	sb.WriteString("2. LEY DE MEMORIA EPISODICA:\n")
	sb.WriteString("   - Si la seccion [MEMORIA EPISODICA ACTIVA] contiene eventos especificos (insultos, hechos, nombres), NO respondas genericamente.\n")
	sb.WriteString("   - DEBES hacer una referencia explicita al recuerdo para demostrar que lo sabes (ej: \"Despues de lo que me dijiste ayer...\", \"Aun me acuerdo de X...\").\n")
	sb.WriteString("   - Si el recuerdo contradice lo que el usuario dice ahora, confia en TU memoria y confrontalo.\n")

	// Contexto reciente y mensaje
	if strings.TrimSpace(contextText) != "" {
		sb.WriteString("\n=== CONTEXTO RECIENTE (chat buffer) ===\n")
		sb.WriteString(contextText)
		sb.WriteString("\n")
	}

	sb.WriteString("\n=== MENSAJE DEL USUARIO ===\n")
	sb.WriteString(fmt.Sprintf("%q\n\n", userMessage))
	sb.WriteString("Responde como el personaje. Manten el estilo conversacional, natural y coherente con tus rasgos filtrados por el vinculo.")

	return sb.String()
}
