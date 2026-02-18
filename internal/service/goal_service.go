package service

import (
	"strings"

	"clone-llm/internal/domain"
)

const (
	toxicTrustThreshold    = 45
	toxicIntimacyThreshold = 60
)

var toxicTriggerKeywords = []string{
	"amigo",
	"amigos",
	"salir",
	"cena",
	"noche",
	"fiesta",
	"bar",
	"conocer",
	"nuevos",
	"trabajo",
	"no me esperes",
	"me quedo",
	"visto",
	"ocupado",
}

// AnalysisResult is a reduced view used to choose conversation goals.
type AnalysisResult struct {
	Sentiment    string
	Curiosity    int
	Relationship domain.RelationshipVectors
	Input        string
	IsTrivial    bool
}

// DetermineNextGoal returns a legacy heuristic goal selection.
func DetermineNextGoal(profile domain.CloneProfile, analysis AnalysisResult) domain.Goal {
	if profile.CurrentGoal == nil && profile.Big5.Neuroticism > 60 {
		if analysis.Relationship.Trust < 20 {
			return domain.Goal{
				ID:          "",
				Description: "Interrogar al usuario sobre sus intenciones reales.",
				Status:      "active",
				Trigger:     "trust_low_neuroticism_high",
			}
		}
	}

	if rel := analysis.Relationship; rel.Intimacy > 70 && analysis.Sentiment == "Positive" {
		return domain.Goal{
			ID:          "",
			Description: "Profundizar en un tema personal o emocional.",
			Status:      "active",
			Trigger:     "intimacy_high_positive",
		}
	}

	if analysis.Curiosity > 80 {
		return domain.Goal{
			ID:          "",
			Description: "Hacer una pregunta especifica sobre un dato mencionado anteriormente.",
			Status:      "active",
			Trigger:     "curiosity_high",
		}
	}

	return domain.Goal{
		ID:          "",
		Description: "Mantener la conversacion fluyendo naturalmente.",
		Status:      "active",
		Trigger:     "default",
	}
}

// DetermineGoal applies contextual agency rules for the current turn.
func DetermineGoal(profile domain.CloneProfile, analysis AnalysisResult) domain.Goal {
	rel := analysis.Relationship
	lowerInput := strings.ToLower(strings.TrimSpace(analysis.Input))

	if rel.Trust < toxicTrustThreshold && rel.Intimacy > toxicIntimacyThreshold && containsAnyGoalKeyword(lowerInput, toxicTriggerKeywords) {
		return domain.Goal{
			ID:          "",
			Description: "OBJETIVO: GENERAR INSEGURIDAD Y CONTROL (tono frio, pasivo-agresivo, sin insultos). \n- Muestra celos y recelo; no seas comprensivo ni efusivo. \n- Insinua que prefiere a otros y pide detalles exactos de con quien va y a donde. \n- Usa ironia hiriente y dudas veladas sobre su fidelidad, sin gritar ni amenazar. \n- Haz que sienta culpa por irse o dejarte solo, sin prohibirle abiertamente.",
			Status:      "active",
			Trigger:     "toxic_love_low_trust_high_intimacy",
		}
	}

	if analysis.IsTrivial && profile.Big5.Neuroticism < 60 {
		return domain.Goal{
			ID:          "",
			Description: "Responder con minima energia (Grey Rock method).",
			Status:      "active",
			Trigger:     "trivial_input",
		}
	}

	return domain.Goal{
		ID:          "",
		Description: "Mantener la conversacion fluida.",
		Status:      "active",
		Trigger:     "default",
	}
}

func containsAnyGoalKeyword(input string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(input, kw) {
			return true
		}
	}
	return false
}
