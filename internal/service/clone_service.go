package service

import (
	"context"
	"errors"
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
	promptBuilder    ClonePromptBuilder
	responseParser   LLMResponseParser
	reactionEngine   ReactionEngine
}

var (
	ErrCloneServiceNotConfigured = errors.New("clone service not configured")
	ErrCloneInvalidInput         = errors.New("clone invalid input")
)

func NewCloneService(
	llmClient llm.LLMClient,
	messageRepo repository.MessageRepository,
	profileRepo repository.ProfileRepository,
	traitRepo repository.TraitRepository,
	contextService ContextService,
	narrativeService *NarrativeService,
	analysisService *AnalysisService,
	promptBuilder ClonePromptBuilder,
	responseParser LLMResponseParser,
	reactionEngine ReactionEngine,
) *CloneService {
	return &CloneService{
		llmClient:        llmClient,
		messageRepo:      messageRepo,
		profileRepo:      profileRepo,
		traitRepo:        traitRepo,
		contextService:   contextService,
		narrativeService: narrativeService,
		analysisService:  analysisService,
		promptBuilder:    promptBuilder,
		responseParser:   responseParser,
		reactionEngine:   reactionEngine,
	}
}

// Chat genera una respuesta del clon basada en perfil, rasgos y contexto, la persiste y devuelve el mensaje completo.
func (s *CloneService) Chat(ctx context.Context, userID, sessionID, userMessage string) (domain.Message, *domain.InteractionDebug, error) {
	if s == nil || s.llmClient == nil || s.messageRepo == nil || s.profileRepo == nil || s.traitRepo == nil || s.contextService == nil {
		return domain.Message{}, nil, ErrCloneServiceNotConfigured
	}

	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	userMessage = strings.TrimSpace(userMessage)
	if userID == "" || userMessage == "" {
		return domain.Message{}, nil, ErrCloneInvalidInput
	}

	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("get profile: %w", err)
	}

	analysisSummary := AnalysisResult{Input: userMessage}
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

	isHighTension := false
	if strings.TrimSpace(narrativeText) != "" {
		isHighTension = s.reactionEngine.DetectHighTensionFromNarrative(narrativeText)
	}
	if narrativeText != "" {
		log.Printf("debug: narrative tension=%t text=%q", isHighTension, narrativeText)
	}

	// Snapshot del estado del vínculo (si existe) para metas/contexto
	if s.narrativeService != nil && parseErr == nil {
		if rel, ok := s.snapshotRelationship(ctx, profileUUID, userMessage); ok {
			analysisSummary.Relationship = rel
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
	analysisSummary.Sentiment = s.reactionEngine.MapEmotionToSentiment(emotionCategory)

	// Filtro: si el input es bajo y neutro/negativo leve, no lo elevamos a memoria
	const tensionIntensityFloor = 35
	effectiveIntensity := emotionalIntensity
	if emotionalIntensity < 30 && (s.reactionEngine.IsNegativeEmotion(emotionCategory) || s.reactionEngine.IsNeutralEmotion(emotionCategory)) {
		if !isHighTension {
			effectiveIntensity = 0
			trivialInput = true
		} else {
			// Relación tensa: forzamos atención mínima aunque el analyzer lo pinte neutro.
			trivialInput = false
			if effectiveIntensity < tensionIntensityFloor {
				effectiveIntensity = tensionIntensityFloor
			}
		}
	}
	if isHighTension {
		if effectiveIntensity < tensionIntensityFloor {
			effectiveIntensity = tensionIntensityFloor
		}
		trivialInput = false
	}

	// Modelo ReLu de intensidad efectiva basado en resiliencia/Big5
	effective, dbg := s.reactionEngine.CalculateReaction(float64(effectiveIntensity), profile.Big5)
	interactionDebug := dbg
	effectiveIntensity = int(math.Round(effective))

	// Si la resiliencia es alta y no hubo activación, tratamos como trivial
	if effectiveIntensity == 0 && resilience >= 0.5 && !isHighTension {
		trivialInput = true
	}

	// Persistir memoria solo si hay señal emocional real
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

	analysisSummary.IsTrivial = trivialInput

	goal := DetermineGoal(profile, analysisSummary)
	profile.CurrentGoal = &goal
	if strings.TrimSpace(goal.Trigger) != "" && !strings.EqualFold(goal.Trigger, "default") && strings.TrimSpace(goal.Description) != "" {
		obj := "[OBJETIVO]\n- " + strings.TrimSpace(goal.Description)
		if strings.TrimSpace(narrativeText) != "" {
			narrativeText = strings.TrimSpace(narrativeText) + "\n\n" + obj
		} else {
			narrativeText = obj
		}
	}

	prompt := s.promptBuilder.BuildClonePrompt(&profile, traits, contextText, narrativeText, userMessage, trivialInput)

	responseRaw, err := s.llmClient.Generate(ctx, prompt)
	if err != nil {
		return domain.Message{}, nil, fmt.Errorf("llm generate: %w", err)
	}

	log.Printf("clone raw response received (len=%d)", len(responseRaw))

	llmResp, ok := s.responseParser.ParseLLMResponseSafe(responseRaw)
	if !ok {
		llmResp.PublicResponse = SanitizeFallbackPublicText(responseRaw)
	}

	response := strings.TrimSpace(llmResp.PublicResponse)
	if response == "" {
		response = "No tengo una respuesta en este momento."
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
		return domain.Message{}, nil, fmt.Errorf("persist clone message: %w", err)
	}

	return cloneMessage, interactionDebug, nil
}

// snapshotRelationship intenta recuperar el vinculo activo (o el primero disponible) para usarlo en metas.
func (s *CloneService) snapshotRelationship(ctx context.Context, profileID uuid.UUID, userMessage string) (domain.RelationshipVectors, bool) {
	if s.narrativeService == nil || profileID == uuid.Nil {
		return domain.RelationshipVectors{}, false
	}

	chars, err := s.narrativeService.characterRepo.ListByProfileID(ctx, profileID)
	if err != nil || len(chars) == 0 {
		return domain.RelationshipVectors{}, false
	}

	active := detectActiveCharacters(chars, userMessage)
	if len(active) == 0 {
		active = chars
	}
	return active[0].Relationship, true
}
