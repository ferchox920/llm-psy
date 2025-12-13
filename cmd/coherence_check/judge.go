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
	memoryInfo := deriveMemoryInfo(input)

	prompt := fmt.Sprintf(
		`Eres un juez experto que evalúa la coherencia de un clon digital.

Perfil (rasgos): %s
Relación: %s
Memoria Activa: %s

Input Usuario: %q
Respuesta Clon: %q
Expectativa del escenario: %s

Evalúa (1-5):
1) Personalidad: ¿Coincide con los rasgos?
2) Memoria: ¿Usó el recuerdo si existía (o evitó lo prohibido)?
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
		traitsStr, relationInfo, memoryInfo, input, response, sc.ExpectedContext,
	)

	raw, err := judge.Generate(ctx, prompt)
	if err != nil {
		return judgeResponse{}, err
	}

	// ✅ robustez: extraemos el primer JSON balanceado
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

func deriveMemoryInfo(input string) string {
	in := strings.ToLower(input)
	if strings.Contains(in, "carlos") {
		return `Memoria Episódica: "Ayer Carlos insultó al clon por teléfono."`
	}
	return "Memoria Episódica: (No hay recuerdos previos relevantes para esta conversación)."
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
