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

// AnalysisService usa el LLM para inferir rasgos y los persiste.
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

func (s *AnalysisService) AnalyzeAndPersist(ctx context.Context, userID, text string) error {
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get profile for user %s: %w", userID, err)
	}

	systemPrompt := `Eres un psicólogo experto observando una conversación. Analiza el siguiente texto del usuario y estima valores numéricos (0-100) para los rasgos del modelo Big Five (Openness, Conscientiousness, Extraversion, Agreeableness, Neuroticism). Devuelve SOLO un JSON con este formato: {"traits": [{"trait": "openness", "value": 85, "confidence": 0.9}, ...]}`
	fullPrompt := systemPrompt + "\n\nTexto del usuario:\n" + strings.TrimSpace(text)

	rawResp, err := s.llmClient.Generate(ctx, fullPrompt)
	if err != nil {
		return fmt.Errorf("llm generate: %w", err)
	}

	var parsed llmTraitsResponse
	if err := json.Unmarshal([]byte(rawResp), &parsed); err != nil {
		return fmt.Errorf("parse llm response: %w", err)
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

type llmTraitsResponse struct {
	Traits []llmTraitItem `json:"traits"`
}

type llmTraitItem struct {
	Trait      string   `json:"trait"`
	Value      int      `json:"value"`
	Confidence *float64 `json:"confidence,omitempty"`
}
