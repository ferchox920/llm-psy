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
