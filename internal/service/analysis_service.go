package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
	"clone-llm/internal/repository"
)

// AnalysisService usa el LLM para inferir rasgos y evaluar carga emocional.
type AnalysisService struct {
	llmClient   llm.LLMClient
	traitRepo   repository.TraitRepository
	profileRepo repository.ProfileRepository
	logger      *zap.Logger
}

func NewAnalysisService(
	llmClient llm.LLMClient,
	traitRepo repository.TraitRepository,
	profileRepo repository.ProfileRepository,
	logger *zap.Logger,
) *AnalysisService {
	return &AnalysisService{
		llmClient:   llmClient,
		traitRepo:   traitRepo,
		profileRepo: profileRepo,
		logger:      logger,
	}
}

// AnalyzeAndPersist guarda los rasgos inferidos y devuelve error si falla.
func (s *AnalysisService) AnalyzeAndPersist(ctx context.Context, userID, text string) error {
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get profile for user %s: %w", userID, err)
	}

	parsed, err := s.runAnalysis(ctx, text)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, t := range parsed.Traits {
		trait := domain.Trait{
			ID:         uuid.NewString(),
			ProfileID:  profile.ID,
			Category:   domain.TraitCategoryBigFive,
			Trait:      strings.ToLower(t.Trait),
			Value:      t.Value,
			Confidence: t.Confidence,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := s.traitRepo.Upsert(ctx, trait); err != nil {
			s.logger.Warn("trait upsert failed", zap.Error(err), zap.String("profile_id", profile.ID), zap.String("trait", trait.Trait))
			return fmt.Errorf("trait upsert: %w", err)
		}
	}

	return nil
}

// AnalyzeEmotion devuelve la intensidad y categoria emocional sin persistir rasgos.
// Aplica un umbral de ruido segun la resiliencia del perfil para evitar sobrerreaccionar a inputs triviales.
func (s *AnalysisService) AnalyzeEmotion(ctx context.Context, profile *domain.CloneProfile, text string) (EmotionAnalysis, error) {
	parsed, err := s.runAnalysis(ctx, text)
	if err != nil {
		return EmotionAnalysis{}, err
	}
	intensity := parsed.EmotionalIntensity
	if intensity <= 0 {
		intensity = 10
	}
	category := strings.TrimSpace(parsed.EmotionCategory)
	if category == "" {
		category = "NEUTRAL"
	}

	resilience := 0.5
	if profile != nil {
		resilience = profile.GetResilience()
	}
	// Amortiguacion y puerta de ruido
	effective := float64(intensity) * (1.0 - (resilience * 0.5))
	noiseThreshold := 20.0 + (resilience * 30.0)
	if effective < noiseThreshold && !strings.EqualFold(parsed.EmotionCategory, "Extreme") {
		effective = 0
		category = "NEUTRAL"
	}

	return EmotionAnalysis{
		EmotionalIntensity: int(effective),
		EmotionCategory:    category,
	}, nil
}

type EmotionAnalysis struct {
	EmotionalIntensity int
	EmotionCategory    string
}

func (s *AnalysisService) runAnalysis(ctx context.Context, text string) (AnalysisResponse, error) {
	systemPrompt := `Eres un psicologo experto observando una conversacion. Analiza el siguiente texto del usuario y:
- Estima valores numericos (0-100) para los rasgos del modelo Big Five (Openness, Conscientiousness, Extraversion, Agreeableness, Neuroticism).
- Extrae la carga emocional del mensaje.
- Devuelve SOLO un JSON con este formato:
{
  "traits": [{"trait": "openness", "value": 85, "confidence": 0.9}, ...],
  "emotional_intensity": 75,
  "emotion_category": "IRA"
}

Guia de emotional_intensity (1-100):
- 0-20: hechos triviales (clima, comida, saludos)
- 21-50: opiniones o charla normal
- 51-80: discusiones, confesiones personales
- 81-100: insultos graves, declaraciones de amor/odio, traumas, crisis`
	fullPrompt := systemPrompt + "\n\nTexto del usuario:\n" + strings.TrimSpace(text)

	rawResp, err := s.llmClient.Generate(ctx, fullPrompt)
	if err != nil {
		return AnalysisResponse{}, fmt.Errorf("llm generate: %w", err)
	}

	cleanedResp := cleanLLMJSONResponse(rawResp)

	var parsed AnalysisResponse
	if err := json.Unmarshal([]byte(cleanedResp), &parsed); err != nil {
		return AnalysisResponse{}, fmt.Errorf("parse llm response: %w", err)
	}
	return parsed, nil
}

// AnalysisResponse captura rasgos y carga emocional devueltos por el LLM analista.
type AnalysisResponse struct {
	Traits             []llmTraitItem `json:"traits"`
	EmotionalIntensity int            `json:"emotional_intensity"`
	EmotionCategory    string         `json:"emotion_category"`
}

type llmTraitItem struct {
	Trait      string   `json:"trait"`
	Value      int      `json:"value"`
	Confidence *float64 `json:"confidence,omitempty"`
}
