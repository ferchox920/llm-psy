package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"clone-llm/internal/llm"
	"go.uber.org/zap"
)

// TestService orchestrates personality test questions and trait analysis.
type TestService struct {
	llmClient   llm.LLMClient // kept for compatibility with existing constructors
	analysisSvc *AnalysisService
	logger      *zap.Logger
	analyzeFn   func(ctx context.Context, userID, text string) error
}

var (
	ErrTestServiceNotConfigured = errors.New("test service not configured")
	ErrTestServiceInvalidInput  = errors.New("test service invalid input")
)

func NewTestService(llmClient llm.LLMClient, analysisSvc *AnalysisService, logger *zap.Logger) *TestService {
	svc := &TestService{
		llmClient:   llmClient,
		analysisSvc: analysisSvc,
		logger:      logger,
	}
	if analysisSvc != nil {
		svc.analyzeFn = analysisSvc.AnalyzeAndPersist
	}
	return svc
}

// GenerateInitialQuestions returns the static OCEAN questionnaire.
func (s *TestService) GenerateInitialQuestions() []string {
	return []string{
		"Que tan de acuerdo estas con explorar ideas no convencionales o abstractas?",
		"Cuando te enfrentas a algo totalmente nuevo, sentis curiosidad o rechazo inicial?",
		"Disfrutas de actividades creativas como escribir, dibujar, programar cosas experimentales o pensar teorias nuevas?",
		"Con que frecuencia planificas tu dia con antelacion y sigues tu horario?",
		"Cuando tenes un objetivo importante, te mantienes constante o dependes del impulso del momento?",
		"Que tan ordenado sos con tus responsabilidades, finanzas o compromisos?",
		"Disfrutas de ser el centro de atencion en reuniones sociales, o prefieres grupos pequenos?",
		"Despues de pasar tiempo con mucha gente, te sentis con mas energia o agotado?",
		"Te resulta facil iniciar conversaciones con desconocidos?",
		"Tiendes a ser comprensivo y perdonar facilmente los errores de otros?",
		"Cuando hay un conflicto, preferis ceder, negociar o imponer tu punto de vista?",
		"Que tan importante es para vos mantener la armonia en tus relaciones?",
		"Te preocupas con frecuencia por el futuro o sientes ansiedad en situaciones de incertidumbre?",
		"Cuando algo sale mal, te afecta emocionalmente por mucho tiempo o lo superas rapido?",
		"Con que frecuencia experimentas cambios intensos de animo?",
	}
}

// AnalyzeTestResponses concatenates answers and sends them to AnalysisService.
func (s *TestService) AnalyzeTestResponses(ctx context.Context, userID string, responses map[string]string) error {
	if s == nil || s.analyzeFn == nil {
		return ErrTestServiceNotConfigured
	}
	userID = strings.TrimSpace(userID)
	if userID == "" || len(responses) == 0 {
		return ErrTestServiceInvalidInput
	}

	analysisText := s.buildAnalysisText(responses)

	if s.logger != nil {
		s.logger.Info("Starting trait analysis from test responses", zap.String("user_id", userID))
	}

	if err := s.analyzeFn(ctx, userID, analysisText); err != nil {
		return fmt.Errorf("failed to analyze and persist test traits: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Successfully analyzed and persisted initial traits", zap.String("user_id", userID))
	}
	return nil
}

func (s *TestService) buildAnalysisText(responses map[string]string) string {
	var fullText strings.Builder
	fullText.WriteString("Respuestas del Test de Personalidad del usuario. Analiza estas respuestas para inferir los rasgos Big Five (OCEAN):\n")

	preferredOrder := s.GenerateInitialQuestions()
	used := make(map[string]struct{}, len(preferredOrder))

	for _, question := range preferredOrder {
		answer, ok := responses[question]
		if !ok {
			continue
		}
		fullText.WriteString(fmt.Sprintf("P: %s\nR: %s\n---\n", strings.TrimSpace(question), strings.TrimSpace(answer)))
		used[question] = struct{}{}
	}

	var extras []string
	for question := range responses {
		if _, ok := used[question]; !ok {
			extras = append(extras, question)
		}
	}
	sort.Strings(extras)
	for _, question := range extras {
		fullText.WriteString(fmt.Sprintf("P: %s\nR: %s\n---\n", strings.TrimSpace(question), strings.TrimSpace(responses[question])))
	}

	return fullText.String()
}
