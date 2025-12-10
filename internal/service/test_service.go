package service

import (
	"context"
	"fmt"
	"strings"

	"clone-llm/internal/llm"
	"go.uber.org/zap"
)

// TestService orquesta la generación de preguntas y el análisis de las respuestas para inicializar la personalidad.
type TestService struct {
	llmClient   llm.LLMClient
	analysisSvc *AnalysisService // Reutiliza el servicio de análisis existente
	logger      *zap.Logger
}

func NewTestService(llmClient llm.LLMClient, analysisSvc *AnalysisService, logger *zap.Logger) *TestService {
	return &TestService{
		llmClient:   llmClient,
		analysisSvc: analysisSvc,
		logger:      logger,
	}
}

// GenerateInitialQuestions devuelve un conjunto estático de preguntas para el test OCEAN.
func (s *TestService) GenerateInitialQuestions() []string {
	return []string{
		"¿Qué tan de acuerdo estás con explorar ideas no convencionales o abstractas?",
		"¿Con qué frecuencia planificas tu día con antelación y sigues tu horario?",
		"¿Disfrutas de ser el centro de atención en reuniones sociales, o prefieres grupos pequeños?",
		"¿Tiendes a ser comprensivo y perdonar fácilmente los errores de otros?",
		"¿Te preocupas con frecuencia por el futuro o sientes ansiedad en situaciones de incertidumbre?",
	}
}

// AnalyzeTestResponses concatena las respuestas y usa el AnalysisService para inferir rasgos.
func (s *TestService) AnalyzeTestResponses(ctx context.Context, userID string, responses map[string]string) error {
	var fullText strings.Builder
	fullText.WriteString("Respuestas del Test de Personalidad del usuario. Analiza estas respuestas para inferir los rasgos Big Five (OCEAN):\n")

	for question, answer := range responses {
		fullText.WriteString(fmt.Sprintf("P: %s\nR: %s\n---\n", question, answer))
	}

	analysisText := fullText.String()

	s.logger.Info("Starting trait analysis from test responses", zap.String("user_id", userID))

	// Reutiliza la lógica de inferencia del LLM ya existente
	err := s.analysisSvc.AnalyzeAndPersist(ctx, userID, analysisText)
	if err != nil {
		return fmt.Errorf("failed to analyze and persist test traits: %w", err)
	}

	s.logger.Info("Successfully analyzed and persisted initial traits", zap.String("user_id", userID))
	return nil
}
