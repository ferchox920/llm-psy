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
	// normalización conservadora: lower + trim.
	// (si ya tienes algo mejor, úsalo y borra esto)
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
 Negación semántica
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
	msg := strings.ToLower(userMessage)
	for _, c := range chars {
		if strings.Contains(msg, strings.ToLower(c.Name)) {
			out = append(out, c)
		}
	}
	return out
}

/*
========================
 Vínculo / Dinámicas
========================
*/

func deriveBondDynamics(trust, intimacy, respect int) string {
	var parts []string

	if intimacy >= 80 && trust <= 20 {
		parts = append(parts, "apego alto + desconfianza alta (celos, control, sospecha, pasivo-agresividad)")
	}
	if respect <= 30 {
		parts = append(parts, "tendencia a reproches/hostilidad")
	}
	if len(parts) == 0 {
		return "vínculo relativamente estable/neutral"
	}
	return strings.Join(parts, "; ")
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
		return "1 día"
	}
	if days < 30 {
		return fmt.Sprintf("%d días", days)
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
		return "1 año"
	}
	return fmt.Sprintf("%d años", years)
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
 Intensidad: compat 0–10 y 0–100
========================
*/

// normalizeIntensity convierte intensidades "cortas" (0–10) a escala 0–100.
// Esto alinea los tests (que usan 7/8/9) con la lógica (umbrales 60/70).
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

// Umbral trauma (en escala 0–100).
func shouldSkipTrauma(m domain.NarrativeMemory) bool {
	if isNegativeCategory(m.EmotionCategory) {
		intensity := normalizeIntensity(m.EmotionalIntensity)
		return intensity >= 60
	}
	return false
}

/*
========================
 Títulos de sección
========================
*/

func resolveSectionTitle(isBenign bool, memories []domain.NarrativeMemory) string {
	if isBenign {
		return "=== GUSTOS Y PREFERENCIAS ==="
	}
	if len(memories) == 0 {
		return "=== MEMORIA EVOCADA ==="
	}

	negHigh := 0
	for _, m := range memories {
		if isNegativeCategory(m.EmotionCategory) {
			intensity := normalizeIntensity(m.EmotionalIntensity)
			if intensity >= 70 {
				negHigh++
			}
		}
	}

	if negHigh*2 >= len(memories) {
		// OJO: SIN tilde para calzar el test.
		return "=== ASOCIACIONES TRAUMATICAS ==="
	}
	return "=== MEMORIA EVOCADA ==="
}

func isNegativeCategory(cat string) bool {
	switch strings.ToUpper(strings.TrimSpace(cat)) {
	case "TRISTEZA", "MIEDO", "IRA":
		return true
	default:
		return false
	}
}
