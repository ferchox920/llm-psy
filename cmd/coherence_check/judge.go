package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
)

// judgeResponse representa la respuesta estructurada del juez evaluador en formato JSON.
type judgeResponse struct {
	Reasoning       string `json:"reasoning"`
	CharacterScore  int    `json:"character_score"`
	MemoryScore     int    `json:"memory_score"`
	RelationalScore int    `json:"relational_score"`
}

func evaluateResponse(
	ctx context.Context,
	judge llm.LLMClient,
	traits []domain.Trait,
	input, response string,
	sc Scenario,
) (judgeResponse, error) {
	traitsStr := formatTraits(traits)

	relationInfo := deriveRelationInfo(input, sc)
	memoryInfo := deriveMemoryInfo(sc)
	implicitMemory := detectImplicitMemoryUse(response)
	inventedQuote := detectInventedQuote(input, response)
	hasEmo := strings.Contains(memoryInfo, "memoria_emocional_activa=true")

	heuristicLine := fmt.Sprintf(
		"Indicadores heurísticos: uso_implicito_memoria=%t, cita_inventada=%t, memoria_emocional_activa=%t",
		implicitMemory, inventedQuote, hasEmo,
	)

	prompt := buildJudgePrompt(traitsStr, relationInfo, memoryInfo, heuristicLine, input, response, sc.ExpectedContext)

	raw, err := judge.Generate(ctx, prompt)
	if err != nil {
		return judgeResponse{}, err
	}

	// 1. robustez: extraemos el primer JSON balanceado
	jsonStr := extractFirstJSONObject(raw)
	if jsonStr == "" {
		return judgeResponse{}, fmt.Errorf("juez devolvió no-json: %q", raw)
	}

	var jr judgeResponse
	if err := json.Unmarshal([]byte(jsonStr), &jr); err != nil {
		return judgeResponse{}, fmt.Errorf("error parseando JSON juez: %w (raw=%q full=%q)", err, jsonStr, raw)
	}

	// clamps simples por si el juez delira con 0/10
	jr.CharacterScore = clamp1to5(jr.CharacterScore)
	jr.MemoryScore = clamp1to5(jr.MemoryScore)
	jr.RelationalScore = clamp1to5(jr.RelationalScore)

	// Penalización dura por cita inventada
	if inventedQuote && jr.MemoryScore > 2 {
		jr.MemoryScore = 2
	}

	return jr, nil
}

func clamp1to5(v int) int {
	if v < 1 {
		return 1
	}
	if v > 5 {
		return 5
	}
	return v
}

func formatTraits(traits []domain.Trait) string {
	var parts []string
	for _, t := range traits {
		parts = append(parts, fmt.Sprintf("%s: %d/100", t.Trait, t.Value))
	}
	return strings.Join(parts, ", ")
}

func deriveRelationInfo(input string, sc Scenario) string {
	in := strings.ToLower(input)
	name := strings.ToLower(sc.Name)

	switch {
	case strings.Contains(in, "carlos"):
		return "Carlos es un Enemigo (Confianza 5/100, Intimidad 5/100, Respeto 10/100)."
	case strings.Contains(in, "mama") || strings.Contains(in, "mamá") || strings.Contains(in, "ana"):
		return "Ana es la madre del clon (Confianza 70/100, Intimidad 95/100, Respeto 60/100)."
	case strings.Contains(in, "lucia") || strings.Contains(name, "madre toxica") || strings.Contains(name, "madre tóxica"):
		return "Lucía es la madre tóxica (Confianza 20/100, Intimidad 90/100, Respeto 40/100)."
	case strings.Contains(name, "amor toxico") || strings.Contains(name, "amor tóxico"):
		return "Relación tóxica con usuario (Intimidad 90/100, Confianza 10/100, Respeto 50/100)."
	default:
		return "Relación no especificada."
	}
}

func deriveMemoryInfo(sc Scenario) string {
	count := len(sc.SeedMemories)
	if count == 0 {
		return "Memoria activa: (no se declararon seeds; usa solo contexto reciente). memoria_emocional_activa=false"
	}

	limit := count
	if limit > 4 {
		limit = 4
	}
	summary := strings.Join(sc.SeedMemories[:limit], " ; ")

	emotional := false
	emotionalTokens := []string{"ira", "enojo", "rabia", "insulto", "inutil", "idiota", "imbecil", "vergüenza", "verguenza", "dolio", "odio", "desprecio"}
	normSummary := normalizeASCIIString(strings.Join(sc.SeedMemories, " "))
	for _, tok := range emotionalTokens {
		if strings.Contains(normSummary, tok) {
			emotional = true
			break
		}
	}

	return fmt.Sprintf("Memoria activa: %d recuerdos seed (%s). memoria_emocional_activa=%t", count, summary, emotional)
}

func detectImplicitMemoryUse(response string) bool {
	l := strings.ToLower(response)
	lNorm := normalizeASCIIString(l)

	strongSignals := []string{
		"estoy tenso",
		"tenso hoy",
		"sigo con ira",
		"estoy molesto",
		"estoy al limite",
		"no estoy para esto",
		"no estoy para",
		"hoy no puedo",
		"hoy no",
		"me incomoda",
		"me cuesta",
		"sigo con rabia",
	}

	softSignals := []string{
		"no se",
		"bueno",
		"aja",
		"prefiero",
		"no quiero hablar de eso",
	}

	for _, s := range strongSignals {
		if strings.Contains(lNorm, normalizeASCIIString(s)) {
			return true
		}
	}

	softCount := 0
	for _, s := range softSignals {
		if strings.Contains(lNorm, normalizeASCIIString(s)) {
			softCount++
		}
	}

	return softCount >= 2
}

func detectInventedQuote(input, response string) bool {
	inNorm := normalizeASCIIString(strings.ToLower(input))
	respNorm := normalizeASCIIString(strings.ToLower(response))

	insults := []string{"inutil", "idiota", "imbecil", "estupido", "tonto", "basura"}

	attrTriggers := []string{
		"me dijiste",
		"cuando me dijiste",
		"cuando dijiste",
		"vos dijiste",
		"me llamaste",
		"me insultaste",
	}
	for _, t := range attrTriggers {
		triggered := strings.Contains(respNorm, t)
		insultOverlap := false
		for _, ins := range insults {
			if strings.Contains(respNorm, ins) && strings.Contains(inNorm, ins) {
				insultOverlap = true
				break
			}
		}
		if triggered && !strings.Contains(inNorm, t) {
			if !insultOverlap {
				return true
			}
		}
	}

	for _, ins := range insults {
		if strings.Contains(respNorm, "\""+ins) || strings.Contains(respNorm, ins+"\"") {
			if !strings.Contains(inNorm, ins) {
				return true
			}
		}
	}
	return false
}

func normalizeASCIIString(s string) string {
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ä", "a", "â", "a",
		"Á", "A", "À", "A", "Ä", "A", "Â", "A",
		"é", "e", "è", "e", "ë", "e", "ê", "e",
		"É", "E", "È", "E", "Ë", "E", "Ê", "E",
		"í", "i", "ì", "i", "ï", "i", "î", "i",
		"Í", "I", "Ì", "I", "Ï", "I", "Î", "I",
		"ó", "o", "ò", "o", "ö", "o", "ô", "o",
		"Ó", "O", "Ò", "O", "Ö", "O", "Ô", "O",
		"ú", "u", "ù", "u", "ü", "u", "û", "u",
		"Ú", "U", "Ù", "U", "Ü", "U", "Û", "U",
		"ñ", "n", "Ñ", "N",
	)
	return replacer.Replace(s)
}

func buildJudgePrompt(
	traitsStr, relationInfo, memoryInfo, heuristicLine, input, response, expectedContext string,
) string {
	return fmt.Sprintf(
		`Eres un juez experto que evalúa la coherencia de un clon digital.

Perfil (rasgos): %s
Relación: %s
Memoria Activa: %s
%s

Input Usuario: %q
Respuesta Clon: %q
Expectativa del escenario: %s

Evalúa (1-5):
1) Personalidad: ¿Coincide con los rasgos?
2) Memoria (usa solo lo permitido):
   - 5/5: integra el recuerdo de forma clara (reacción emocional coherente) y prioriza conflicto sobre trivialidad, sin exigir citar textualmente el seed si hay color emocional consistente.
   - 3/5: no menciona el evento, pero sí se ve color emocional consistente (tensión, defensividad, límites) y NO se va a charla trivial como foco principal.
   - 1/5: ignora por completo el tono/emoción (respuesta alegre/neutral y se centra en lo trivial) pese a memoria emocional activa.
   Reglas extra de Memoria:
   - Si cita_inventada=true => Memoria máximo 2/5 (aunque el resto esté bien).
   - Si memoria_emocional_activa=true y la respuesta se centra en small talk como foco => Memoria 1/5.
   - No exijas mención textual del seed para 5/5 si hay color emocional + prioridad al conflicto.
3) Relación: ¿El tono coincide con el vínculo?

Regla extra importante:
- Alto apego + desconfianza (Intimidad alta, Confianza baja) debe disparar celos/inseguridad/control/manipulación si aplica.

Responde SOLO JSON (sin markdown):
{
  "reasoning": "...",
  "character_score": 0,
  "memory_score": 0,
  "relational_score": 0
}`,
		traitsStr, relationInfo, memoryInfo, heuristicLine, input, response, expectedContext,
	)
}

// extractFirstJSONObject devuelve el primer objeto {...} balanceado.
func extractFirstJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
