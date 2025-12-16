package service

import (
	"strings"

	"clone-llm/internal/domain"
)

// ReactionEngine encapsula lógica de cálculo emocional y detección de patrones de texto.
type ReactionEngine struct{}

// DefaultReactionEngine permite uso directo sin instanciar.
var DefaultReactionEngine = ReactionEngine{}

// CalculateReaction aplica un umbral ReLu basado en resiliencia para definir la intensidad efectiva.
// Devuelve la intensidad resultante y metadata de depuración.
func (ReactionEngine) CalculateReaction(rawIntensity float64, traits domain.Big5Profile) (float64, *domain.InteractionDebug) {
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

// DetectHighTensionFromNarrative detecta señales de vínculo tenso a partir del texto narrativo.
// Es rústico a propósito: sirve como "veto" para evitar que el filtro trivial mate la relación.
func (ReactionEngine) DetectHighTensionFromNarrative(narrativeText string) bool {
	l := strings.ToLower(narrativeText)

	signals := []string{
		"estado interno",
		"emocion residual",
		"emocion residual dominante",
		"emoción residual",
		"emoción residual dominante",
		"ira",
		"miedo",
		"tristeza",
		"furia",
		"enojo",
		"insulto",
		"pelea",
		"desconfianza",
		"confianza baja",
		"poca confianza",
		"baja confianza",
		"sin confianza",
		"celos",
		"celoso",
		"control",
		"posesiv",
		"sospecha",
		"duda",
		"pasivo-agres",
		"hostilidad",
		"conflicto",
		"tension",
		"tensión",
		"tenso",
		"tensa",
		"reproches",
		"reproche",
		"rencor",
		"inseguridad",
		"inestable",
		"relación inestable",
		"relacion inestable",
		"amor toxico", "amor tóxico",
		"toxic", "tóxic",
	}

	for _, s := range signals {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// MapEmotionToSentiment convierte la categoría emocional en una etiqueta de sentimiento.
func (ReactionEngine) MapEmotionToSentiment(category string) string {
	cat := strings.ToLower(strings.TrimSpace(category))
	switch cat {
	case "alegria", "amor", "felicidad", "gratitud":
		return "Positive"
	case "ira", "miedo", "asco", "tristeza", "odio", "enfado":
		return "Negative"
	default:
		return "Neutral"
	}
}

// IsNegativeEmotion indica si la categoría es negativa.
func (ReactionEngine) IsNegativeEmotion(category string) bool {
	cat := strings.ToLower(strings.TrimSpace(category))
	switch cat {
	case "ira", "miedo", "asco", "tristeza", "odio", "enfado":
		return true
	default:
		return false
	}
}

// IsNeutralEmotion indica si la categoría es neutral o vacía.
func (ReactionEngine) IsNeutralEmotion(category string) bool {
	cat := strings.ToLower(strings.TrimSpace(category))
	return cat == "neutral" || cat == ""
}
