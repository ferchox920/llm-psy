package service

import (
	"fmt"
	"strings"
	"time"

	"clone-llm/internal/domain"
)

/*
========================
 Utilidades de texto
========================
*/

func normalize(s string) string {
	// normalizacion conservadora: lower + trim.
	// (si ya tienes algo mejor, usalo y borra esto)
	return strings.ToLower(strings.TrimSpace(s))
}

func containsAny(msg string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

/*
========================
 Negacion semantica
========================
*/

func hasNegationSemantic(msgLower string) bool {
	msg := normalize(msgLower)

	markers := []string{"nunca", "jamas", "ya no", "no me"}
	triggers := []string{"abandon", "funeral", "recuerd", "lluvia"}

	for _, m := range markers {
		if strings.Contains(msg, m) {
			for _, t := range triggers {
				if strings.Contains(msg, t) {
					return true
				}
			}
		}
	}
	return false
}

func detectActiveCharacters(chars []domain.Character, userMessage string) []domain.Character {
	var out []domain.Character
	msg := strings.ToLower(strings.TrimSpace(userMessage))
	if msg == "" {
		return out
	}
	for _, c := range chars {
		name := strings.ToLower(strings.TrimSpace(c.Name))
		if name == "" {
			continue
		}
		if strings.Contains(msg, name) {
			out = append(out, c)
		}
	}
	return out
}

/*
========================
 Vinculo / Dinamicas
========================
*/

func deriveBondDynamics(trust, intimacy, respect int) string {
	var parts []string

	if intimacy >= 70 && trust <= 40 {
		parts = append(parts, "MODO: CELOS PATOLOGICOS. Apego alto + desconfianza: actua con sospecha y necesidad de confirmacion; usa control indirecto (insinuaciones/ironia suave/victimismo leve). Evita interrogatorio explicito: maximo 1 pregunta. No pidas lista de nombres/hora/lugar. Puedes dar 1 pinchazo pasivo-agresivo y 1 frase carinosa-condicional, sin amenazas.")
	}
	if respect <= 35 {
		parts = append(parts, "MODO: HOSTILIDAD DESPECTIVA. Usa sarcasmo, minimiza y reprocha.")
	}
	if len(parts) == 0 {
		return "vinculo relativamente estable/neutral"
	}
	return strings.Join(parts, "; ")
}

func resolveSectionTitle(isBenign bool, memories []domain.NarrativeMemory) string {
	if isBenign {
		return "[RECUERDOS DE CONSUELO]"
	}

	for _, m := range memories {
		if isNegativeCategory(m.EmotionCategory) {
			return "[MEMORIA EMOCIONAL PRIORITARIA]"
		}
	}
	return "[MEMORIA RELEVANTE]"
}

func humanizeRelative(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "instantes"
	}
	if d < time.Hour {
		min := int(d.Minutes())
		if min == 1 {
			return "1 minuto"
		}
		return fmt.Sprintf("%d minutos", min)
	}
	if d < 24*time.Hour {
		hrs := int(d.Hours())
		if hrs == 1 {
			return "1 hora"
		}
		return fmt.Sprintf("%d horas", hrs)
	}

	days := int(d.Hours()) / 24
	if days == 1 {
		return "1 dia"
	}
	if days < 30 {
		return fmt.Sprintf("%d dias", days)
	}

	months := days / 30
	if months == 1 {
		return "1 mes"
	}
	if months < 12 {
		return fmt.Sprintf("%d meses", months)
	}

	years := months / 12
	if years == 1 {
		return "1 ano"
	}
	return fmt.Sprintf("%d anos", years)
}

/*
========================
 Intento benigno / mixto
========================
*/

func detectBenignIntent(msgLower string) bool {
	msg := normalize(msgLower)

	desireMarkers := []string{
		"quiero", "necesito", "me antoja", "se me antoja",
		"me encanta", "me gusta", "favorito", "confort", "algo rico",
	}
	objects := []string{
		"helado", "chocolate", "cafe", "pizza", "torta", "postre",
		"dulce", "musica", "cancion", "pelicula", "serie",
		"juego", "cafecito",
	}

	return containsAny(msg, desireMarkers) && containsAny(msg, objects)
}

func detectMixedIntent(msgLower string) bool {
	if !detectBenignIntent(msgLower) {
		return false
	}

	msg := normalize(msgLower)
	negMarkers := []string{
		"abandono", "abandon", "esperando", "planta",
		"solo", "soledad", "humillacion", "humillado",
		"duelo", "triste", "tristeza", "ira", "enoj", "furia",
		"me dejaron",
	}

	return containsAny(msg, negMarkers)
}

/*
========================
 Intensidad: compat 0-10 y 0-100
========================
*/

// normalizeIntensity convierte intensidades "cortas" (0-10) a escala 0-100.
// Esto alinea los tests (que usan 7/8/9) con la logica (umbrales 60/70).
func normalizeIntensity(v int) int {
	if v < 0 {
		return 0
	}
	// Si viene en 0..10, la escalamos.
	if v <= 10 {
		return v * 10
	}
	// Si ya viene en 0..100, la dejamos.
	if v > 100 {
		return 100
	}
	return v
}

/*
========================
 Filtro de trauma
========================
*/

// Umbral trauma (en escala 0-100).
func shouldSkipTrauma(m domain.NarrativeMemory) bool {
	if isNegativeCategory(m.EmotionCategory) {
		// Negativas requieren intensidad suficiente.
		return normalizeIntensity(m.EmotionalIntensity) < 60
	}
	// Positivas/neutral: no se consideran trauma, no las filtramos por intensidad.
	return false
}

func isNegativeCategory(cat string) bool {
	c := strings.ToLower(strings.TrimSpace(cat))
	switch c {
	case "alegria", "alegría":
		c = "alegria"
	case "tristeza":
		c = "tristeza"
	}
	switch c {
	case "ira", "miedo", "asco", "tristeza", "odio", "enfado", "enojo":
		return true
	default:
		return false
	}
}
