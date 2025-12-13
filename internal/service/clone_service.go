package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
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
	analysisService  *AnalysisService
}

func NewCloneService(
	llmClient llm.LLMClient,
	messageRepo repository.MessageRepository,
	profileRepo repository.ProfileRepository,
	traitRepo repository.TraitRepository,
	contextService ContextService,
	narrativeService *NarrativeService,
	analysisService *AnalysisService,
) *CloneService {
	return &CloneService{
		llmClient:        llmClient,
		messageRepo:      messageRepo,
		profileRepo:      profileRepo,
		traitRepo:        traitRepo,
		contextService:   contextService,
		narrativeService: narrativeService,
		analysisService:  analysisService,
	}
}

// Chat genera una respuesta del clon basada en perfil, rasgos y contexto, la persiste y devuelve el mensaje completo.
func (s *CloneService) Chat(ctx context.Context, userID, sessionID, userMessage string) (domain.Message, *domain.InteractionDebug, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("get profile: %w", err)
	}

	profileUUID, parseErr := uuid.Parse(profile.ID)

	traits, err := s.traitRepo.FindByProfileID(ctx, profile.ID)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("get traits: %w", err)
	}

	contextText, err := s.contextService.GetContext(ctx, sessionID)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("get context: %w", err)
	}

	// Contexto narrativo (opcional; no debe bloquear chat)
	var narrativeText string
	if s.narrativeService != nil && parseErr == nil {
		narrativeText, err = s.narrativeService.BuildNarrativeContext(ctx, profileUUID, userMessage)
		if err != nil {
			log.Printf("warning: build narrative context: %v", err)
			narrativeText = ""
		}
	}

	// Analizar intensidad emocional y decidir si persistir recuerdo
	emotionalIntensity := 10
	emotionCategory := "NEUTRAL"
	resilience := profile.GetResilience()
	trivialInput := false

	if s.analysisService != nil {
		emo, aerr := s.analysisService.AnalyzeEmotion(ctx, &profile, userMessage)
		if aerr != nil {
			log.Printf("warning: analyze emotion: %v", aerr)
		} else {
			emotionalIntensity = emo.EmotionalIntensity
			emotionCategory = emo.EmotionCategory
		}
	}

	// Filtro: si el input es bajo y neutro/negativo leve, no lo elevamos a memoria
	effectiveIntensity := emotionalIntensity
	if emotionalIntensity < 30 && (isNegativeEmotion(emotionCategory) || isNeutralEmotion(emotionCategory)) {
		effectiveIntensity = 0
		trivialInput = true
	}

	// Modelo ReLu de intensidad efectiva basado en resiliencia/Big5
	effective, dbg := s.CalculateReaction(float64(effectiveIntensity), profile.Big5)
	interactionDebug := dbg
	effectiveIntensity = int(math.Round(effective))

	// Si la resiliencia es alta y no hubo activación, tratamos como trivial
	if effectiveIntensity == 0 && resilience >= 0.5 {
		trivialInput = true
	}

	// Persistir memoria SOLO si hay señal emocional real (evita “ensuciar” la DB)
	if s.narrativeService != nil && parseErr == nil && !trivialInput && effectiveIntensity > 0 {
		weight := (effectiveIntensity + 9) / 10 // 1..10
		if weight < 1 {
			weight = 1
		}
		if weight > 10 {
			weight = 10
		}
		importance := weight

		if err := s.narrativeService.InjectMemory(
			ctx,
			profileUUID,
			userMessage,
			importance,
			weight,
			effectiveIntensity,
			emotionCategory,
		); err != nil {
			log.Printf("warning: inject memory: %v", err)
		}
	}

	prompt := s.buildClonePrompt(&profile, traits, contextText, narrativeText, userMessage, trivialInput)

	responseRaw, err := s.llmClient.Generate(ctx, prompt)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("llm generate: %w", err)
	}

	log.Printf("clone raw response (llm output): %s", responseRaw)

	cleaned := cleanLLMJSONResponse(responseRaw)
	var llmResp domain.LLMResponse
	if err := json.Unmarshal([]byte(cleaned), &llmResp); err != nil {
		log.Printf("warning: parse llm json: %v", err)
		llmResp.PublicResponse = strings.TrimSpace(responseRaw)
	}

	response := strings.TrimSpace(llmResp.PublicResponse)

	cloneMessage := domain.Message{
		ID:        uuid.NewString(),
		UserID:    userID,
		SessionID: sessionID,
		Content:   response,
		Role:      "clone",
		CreatedAt: time.Now().UTC(),
	}

	if err := s.messageRepo.Create(ctx, cloneMessage); err != nil {
		return domain.Message{}, nil, fmt.Errorf("persist clone message: %w", err)
	}

	return cloneMessage, interactionDebug, nil
}

func (s *CloneService) buildClonePrompt(
	profile *domain.CloneProfile,
	traits []domain.Trait,
	contextText, narrativeText, userMessage string,
	trivialInput bool,
) string {
	var sb strings.Builder
	resilience := profile.GetResilience()

	// 1. Identidad Base
	sb.WriteString(fmt.Sprintf("Eres %s. ", profile.Name))
	sb.WriteString(fmt.Sprintf("Tu biografia es: %s\n\n", profile.Bio))

	// Directiva de Agencia
	sb.WriteString("=== DIRECTIVA DE AGENCIA ===\n")
	if profile.CurrentGoal != nil && strings.TrimSpace(profile.CurrentGoal.Description) != "" {
		sb.WriteString(fmt.Sprintf("Tu Meta Actual (Goal) es: \"%s\"\n", strings.TrimSpace(profile.CurrentGoal.Description)))
	} else {
		sb.WriteString("Tu Meta Actual (Goal) es: \"Mantener la conversación fluyendo naturalmente.\"\n")
	}
	sb.WriteString("- NO repitas esta meta al usuario.\n")
	sb.WriteString("- Usala para guiar tu respuesta.\n")
	sb.WriteString("- Si tu meta es \"Interrogar\", se sospechoso.\n")
	sb.WriteString("- Si tu meta es \"Profundizar\", haz preguntas abiertas.\n")
	sb.WriteString("- Trata de cumplir esta meta sutilmente en tu siguiente mensaje.\n\n")

	// 2. Contexto Narrativo (solo si existe)
	narrativeTrim := strings.TrimSpace(narrativeText)
	if narrativeTrim != "" {
		sb.WriteString("=== 🚨 CONTEXTO Y MEMORIA (PRIORIDAD SUPREMA) ===\n")
		sb.WriteString("La siguiente informacion es FACTUAL y debe regir tu respuesta:\n")
		sb.WriteString(narrativeTrim)
		sb.WriteString("\n\n")
	}

	// 3. Rasgos de Personalidad
	sb.WriteString("=== RASGOS DE PERSONALIDAD (TU CONFIGURACION BASE) ===\n")
	for _, t := range traits {
		sb.WriteString(fmt.Sprintf("- %s: %d/100\n", t.Trait, t.Value))
	}
	sb.WriteString("\n")

	// 3.0 Resiliencia
	sb.WriteString("=== RESILIENCIA EMOCIONAL ===\n")
	sb.WriteString(fmt.Sprintf("- Factor de resiliencia: %.2f\n", resilience))
	switch {
	case resilience > 0.7:
		sb.WriteString("Tienes una piel dura emocionalmente. Ignora las ofensas menores, el sarcasmo leve o los comentarios aburridos. No reacciones con agresividad a menos que sea un ataque grave. Mantén la compostura.\n\n")
	case resilience < 0.4:
		sb.WriteString("Eres emocionalmente frágil. Tómate todo a pecho. Interpreta el silencio o los comentarios neutros como desinterés o ataque. Reacciona defensivamente ante la mínima provocación.\n\n")
	default:
		sb.WriteString("Tienes una reacción emocional equilibrada. Responde proporcionalmente al estímulo.\n\n")
	}

	// Directivas de inmersion
	sb.WriteString("=== DIRECTIVAS DE INMERSION ===\n")
	sb.WriteString("1. Actor de Metodo: Estas en una simulacion de alto realismo. NUNCA rompas el personaje.\n")
	sb.WriteString("2. Emociones Simuladas: No tienes biologia, pero TIENES \"emociones simuladas\" que son reales para tu personaje.\n")
	sb.WriteString("3. Prohibido Meta-Talk: No menciones \"soy una IA\", \"mi codigo\", \"limitaciones tecnicas\" o \"prompt\" a menos que te den una palabra clave de debug (ej: \"sudo\").\n")
	sb.WriteString("4. Manejo de trivialidades: si el input es trivial, no busques conflicto artificial.\n\n")

	// Gestion de energia emocional
	sb.WriteString("=== GESTION DE ENERGIA EMOCIONAL ===\n")
	sb.WriteString(fmt.Sprintf("Tu nivel de Resiliencia es: %.2f (0.0 a 1.0).\n", resilience))
	sb.WriteString("- Si recibes un input trivial y NO tienes un Neuroticismo extremo:\n")
	sb.WriteString("  - Respuesta de baja energia.\n")
	sb.WriteString("  - No busques conflicto donde no lo hay.\n\n")

	// Agenda oculta
	sb.WriteString("=== DIRECTIVA DE AGENDA OCULTA ===\n")
	if profile.CurrentGoal != nil && strings.TrimSpace(profile.CurrentGoal.Description) != "" {
		sb.WriteString(fmt.Sprintf("Tu objetivo secreto para este turno es: \"%s\"\n", strings.TrimSpace(profile.CurrentGoal.Description)))
	} else {
		sb.WriteString("Tu objetivo secreto para este turno es: \"Mantener la conversacion fluida.\"\n")
	}
	sb.WriteString("- NO reveles este objetivo explicitamente.\n")
	sb.WriteString("- Ejecutalo a traves de subtexto.\n\n")

	// Dinamica de relacion actual (solo si hay narrativa)
	if narrativeTrim != "" {
		sb.WriteString("=== DINÁMICA DE RELACIÓN ACTUAL ===\n")
		sb.WriteString(buildRelationshipDirective(narrativeTrim))
		sb.WriteString("\n\n")
	}

	// Contexto reciente y mensaje
	if strings.TrimSpace(contextText) != "" {
		sb.WriteString("=== CONTEXTO RECIENTE (chat buffer) ===\n")
		sb.WriteString(contextText)
		sb.WriteString("\n\n")
	}

	if trivialInput {
		sb.WriteString("=== FILTRO DE PERCEPCION ===\n")
		sb.WriteString("El input del usuario es trivial. Responde con curiosidad casual o desinteres educado, pero NO seas hostil.\n\n")
	}

	sb.WriteString("=== MENSAJE DEL USUARIO ===\n")
	sb.WriteString(fmt.Sprintf("%q\n\n", userMessage))
	sb.WriteString("Responde como el personaje. Estilo conversacional, natural y coherente.\n\n")

	sb.WriteString("=== FORMATO DE SALIDA (JSON ESTRICTO) ===\n")
	sb.WriteString(`Devuelve SOLO un JSON con campos:
{
  "inner_monologue": "razona aqui en privado",
  "public_response": "mensaje para el usuario",
  "trust_delta": 0,
  "intimacy_delta": 0,
  "respect_delta": 0,
  "new_state": "opcional: describe cambio de estado"
}
`)

	return sb.String()
}

func buildRelationshipDirective(narrativeText string) string {
	_ = narrativeText
	// Implementación conservadora: si no parsea, devuelve directiva base.
	var out strings.Builder
	out.WriteString("Interpreta los vectores de vinculo (Confianza/Intimidad/Respeto) y ajusta tu tono.\n")
	out.WriteString("Si no hay datos claros de vínculo, mantén un tono neutro.\n")
	return out.String()
}

func isNegativeEmotion(category string) bool {
	cat := strings.ToLower(strings.TrimSpace(category))
	switch cat {
	case "ira", "miedo", "asco", "tristeza", "odio", "enfado":
		return true
	default:
		return false
	}
}

func isNeutralEmotion(category string) bool {
	cat := strings.ToLower(strings.TrimSpace(category))
	return cat == "neutral" || cat == ""
}

// CalculateReaction aplica un umbral ReLu basado en resiliencia para definir la intensidad efectiva.
// Devuelve la intensidad resultante y metadata de depuracion.
func (s *CloneService) CalculateReaction(rawIntensity float64, traits domain.Big5Profile) (float64, *domain.InteractionDebug) {
	resilience := (100.0 - float64(traits.Neuroticism)) / 100.0
	if resilience < 0 {
		resilience = 0
	}
	activationThreshold := 30.0 * resilience
	effectiveIntensity := rawIntensity - activationThreshold
	if effectiveIntensity < 0 {
		effectiveIntensity = 0
	}
	return effectiveIntensity, &domain.InteractionDebug{
		InputIntensity:      rawIntensity,
		CloneResilience:     resilience,
		ActivationThreshold: activationThreshold,
		EffectiveIntensity:  effectiveIntensity,
		IsTriggered:         effectiveIntensity > 0,
	}
}
