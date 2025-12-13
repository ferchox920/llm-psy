package service

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"clone-llm/internal/domain"
)

/*
========================
 Normalización de texto
========================
*/

// normalize baja a minúsculas y elimina diacríticos (acentos).
// Ej: "café" -> "cafe", "humillación" -> "humillacion"
func normalize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func containsAny(s string, list []string) bool {
	for _, x := range list {
		if strings.Contains(s, x) {
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

// Requiere:
// - marcador de negación
// - referencia explícita a recuerdo/memoria
// - trigger temático
func hasNegationSemantic(msgLower string) bool {
	msg := normalize(msgLower)

	neg := []string{"nunca", "jamas", "jamás", "ya no", "no me", "no"}
	mem := []string{
		"recuerdo", "recuerdos", "recordar",
		"me recuerda", "me trae recuerdos", "no me recuerda",
	}
	trg := []string{
		"abandon", "funeral", "tierra mojada", "lluvia",
	}

	return containsAny(msg, neg) &&
		containsAny(msg, mem) &&
		containsAny(msg, trg)
}

/*
========================
 Personajes activos
========================
*/

// Detecta personajes mencionados por:
// - nombre completo
// - tokens del nombre (>=3 chars)
// - normalización sin acentos
func detectActiveCharacters(chars []domain.Character, userMessage string) []domain.Character {
	var out []domain.Character
	msg := normalize(userMessage)

	for _, c := range chars {
		nameNorm := normalize(c.Name)
		if strings.Contains(msg, nameNorm) {
			out = append(out, c)
			continue
		}

		// Match por tokens del nombre (Juan Carlos -> Juan)
		for _, tok := range strings.Fields(nameNorm) {
			if len(tok) >= 3 && strings.Contains(msg, tok) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

/*
========================
 Dinámica del vínculo
========================
*/

func deriveBondDynamics(trust, intimacy, respect int) string {
	var parts []string

	if intimacy >= 80 && trust <= 20 {
		parts = append(parts,
			"apego alto + desconfianza alta (celos, control, sospecha, pasivo-agresividad)",
		)
	}
	if respect <= 30 {
		parts = append(parts, "tendencia a reproches/hostilidad")
	}
	if len(parts) == 0 {
		return "vínculo relativamente estable/neutral"
	}
	return strings.Join(parts, "; ")
}

/*
========================
 Tiempo humanizado
========================
*/

func humanizeRelative(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		d = 0
	}

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
 Filtro de trauma
========================
*/

// EmotionalIntensity está en escala 0–100
func shouldSkipTrauma(m domain.NarrativeMemory) bool {
	if isNegativeCategory(m.EmotionCategory) {
		return m.EmotionalIntensity >= 60
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
		if isNegativeCategory(m.EmotionCategory) && m.EmotionalIntensity >= 70 {
			negHigh++
		}
	}
	if negHigh*2 >= len(memories) {
		return "=== ASOCIACIONES TRAUMÁTICAS ==="
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
