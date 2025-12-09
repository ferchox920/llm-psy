package service

import (
	"clone-llm/internal/domain"
)

// AnalysisResult es una vista simplificada de lo que el analisis produce para decidir metas.
type AnalysisResult struct {
	Sentiment string
	Curiosity int
}

// DetermineNextGoal devuelve la meta mas adecuada para el turno actual segun heuristica.
func DetermineNextGoal(profile domain.CloneProfile, analysis AnalysisResult) domain.Goal {
	// 1. Paranoia por baja confianza y alto neuroticismo
	if profile.CurrentGoal == nil && profile.Big5.Neuroticism > 60 {
		rel := profile.Relationship
		if rel.Trust < 20 {
			return domain.Goal{
				ID:          "",
				Description: "Interrogar al usuario sobre sus intenciones reales.",
				Status:      "pending",
				Priority:    9,
				Source:      "system_generated",
			}
		}
	}

	// 2. Intimidad alta y sentimiento positivo
	if rel := profile.Relationship; rel.Intimacy > 70 && analysis.Sentiment == "Positive" {
		return domain.Goal{
			ID:          "",
			Description: "Profundizar en un tema personal o emocional.",
			Status:      "pending",
			Priority:    8,
			Source:      "system_generated",
		}
	}

	// 3. Curiosidad alta
	if analysis.Curiosity > 80 {
		return domain.Goal{
			ID:          "",
			Description: "Hacer una pregunta específica sobre un dato mencionado anteriormente.",
			Status:      "pending",
			Priority:    7,
			Source:      "system_generated",
		}
	}

	// 4. Fallback
	return domain.Goal{
		ID:          "",
		Description: "Mantener la conversación fluyendo naturalmente.",
		Status:      "pending",
		Priority:    5,
		Source:      "system_generated",
	}
}
