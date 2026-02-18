package service

import (
	"strings"
	"testing"
)

func TestPromptsIncludeJealousyGuidance(t *testing.T) {
	triggers := []string{
		"salir con amigos",
		"no me esperes",
		"conoci gente nueva",
		"me dejaron en visto",
		"me celas",
		"con quien estas",
		"por que no respondes",
	}
	concepts := []string{
		"celos",
		"desconfianza",
		"control",
		"inseguridad",
		"miedo al abandono",
	}

	for _, trig := range triggers {
		if !strings.Contains(evocationPromptTemplate, trig) {
			t.Fatalf("evocationPromptTemplate missing trigger %q", trig)
		}
	}
	for _, term := range concepts {
		if !strings.Contains(evocationPromptTemplate, term) {
			t.Fatalf("evocationPromptTemplate missing concept %q", term)
		}
	}

	count := 0
	for _, trig := range triggers {
		if strings.Contains(rerankJudgePrompt, trig) {
			count++
		}
	}
	if count < 2 {
		t.Fatalf("rerankJudgePrompt should include at least 2 jealousy triggers, found %d", count)
	}
	for _, term := range []string{"celos", "inseguridad"} {
		if !strings.Contains(rerankJudgePrompt, term) {
			t.Fatalf("rerankJudgePrompt missing jealousy concept %q", term)
		}
	}
}

func TestPromptsContainCriticalGuardrails(t *testing.T) {
	evocationMustHave := []string{
		`"No hables de X"`,
		"funeral de descuentos",
		"Salida (Texto plano o vacio)",
		`Mensaje del Usuario: "%s"`,
	}
	for _, s := range evocationMustHave {
		if !strings.Contains(evocationPromptTemplate, s) {
			t.Fatalf("evocationPromptTemplate missing %q", s)
		}
	}

	if count := strings.Count(evocationFallbackPrompt, "%s"); count != 1 {
		t.Fatalf("evocationFallbackPrompt must have exactly one %%s placeholder, got %d", count)
	}

	rerankMustHave := []string{
		"Responde SOLO un JSON estricto",
		"EXCEPCION CRITICA",
		"EmotionalIntensity >= 80",
		`Usuario: %q`,
		`Memoria: %q`,
	}
	for _, s := range rerankMustHave {
		if !strings.Contains(rerankJudgePrompt, s) {
			t.Fatalf("rerankJudgePrompt missing %q", s)
		}
	}
	if strings.Count(rerankJudgePrompt, "%q") != 2 {
		t.Fatalf("rerankJudgePrompt must have exactly two %%q placeholders")
	}
}
