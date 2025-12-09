package service

import (
	"clone-llm/internal/domain"
)

// AnalysisResult es una vista simplificada de lo que el analisis produce para decidir metas.
type AnalysisResult struct {
	Sentiment    string
	Curiosity    int
	Relationship domain.RelationshipVectors
}

// DetermineNextGoal devuelve la meta mas adecuada para el turno actual segun heuristica.
func DetermineNextGoal(profile domain.CloneProfile, analysis AnalysisResult) domain.Goal {
	// 1. Paranoia por baja confianza y alto neuroticismo
	if profile.CurrentGoal == nil && profile.Big5.Neuroticism > 60 {
		rel := analysis.Relationship
		if rel.Trust < 20 {
			return domain.Goal{
				ID:          "",
				Description: "Interrogar al usuario sobre sus intenciones reales.",
				Status:      "active",
				Trigger:     "trust_low_neuroticism_high",
			}
		}
	}

	// 2. Intimidad alta y sentimiento positivo
	if rel := analysis.Relationship; rel.Intimacy > 70 && analysis.Sentiment == "Positive" {
		return domain.Goal{
			ID:          "",
			Description: "Profundizar en un tema personal o emocional.",
			Status:      "active",
			Trigger:     "intimacy_high_positive",
		}
	}

	// 3. Curiosidad alta
	if analysis.Curiosity > 80 {
		return domain.Goal{
			ID:          "",
			Description: "Hacer una pregunta específica sobre un dato mencionado anteriormente.",
			Status:      "active",
			Trigger:     "curiosity_high",
		}
	}

	// 4. Fallback
	return domain.Goal{
		ID:          "",
		Description: "Mantener la conversación fluyendo naturalmente.",
		Status:      "active",
		Trigger:     "default",
	}
}
