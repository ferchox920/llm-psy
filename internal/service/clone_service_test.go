package service

import (
	"strings"
	"testing"

	"clone-llm/internal/domain"
)

func TestParseLLMResponseSafe_UnescapesEscapedQuotes(t *testing.T) {
	raw := `{"inner_monologue":"x","public_response":"Dijo: \"hola\" y luego \\ fin","trust_delta":0,"intimacy_delta":0,"respect_delta":0}`

	resp, ok := parseLLMResponseSafe(raw)
	if !ok {
		t.Fatalf("parseLLMResponseSafe returned ok=false")
	}

	want := `Dijo: "hola" y luego \ fin`
	if resp.PublicResponse != want {
		t.Fatalf("public_response mismatch: got %q want %q", resp.PublicResponse, want)
	}
	if resp.InnerMonologue != "" {
		t.Fatalf("expected inner monologue to be empty in safe parse, got %q", resp.InnerMonologue)
	}
}

func TestDetectHighTensionFromNarrative(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "estado interno con ira",
			text: "[ESTADO INTERNO]\n- Emocion residual dominante: IRA (por un conflicto reciente; el clon todavia siente esa emocion).",
			want: true,
		},
		{
			name: "trivial sin tension",
			text: "El cielo nublado y tostadas con cafe",
			want: false,
		},
		{
			name: "reproches y tension",
			text: "Hubo reproches y tension",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectHighTensionFromNarrative(tt.text); got != tt.want {
				t.Fatalf("detectHighTensionFromNarrative(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestBuildClonePromptIncludesTensionDirectiveWhenStatePresent(t *testing.T) {
	svc := &CloneService{}
	profile := domain.CloneProfile{Name: "Test", Bio: "bio"}
	narrative := "[ESTADO INTERNO]\n- Emocion residual dominante: IRA (por un conflicto reciente; el clon todavia siente esa emocion)."

	prompt := svc.buildClonePrompt(&profile, nil, "", narrative, "hola", false)

	if !strings.Contains(prompt, "Si aparece [ESTADO INTERNO]") {
		t.Fatalf("expected tension directive when state present; got %q", prompt)
	}
}

func TestBuildClonePromptOmitsTensionDirectiveWhenNoState(t *testing.T) {
	svc := &CloneService{}
	profile := domain.CloneProfile{Name: "Test", Bio: "bio"}
	narrative := "Resumen cualquiera sin estado interno"

	prompt := svc.buildClonePrompt(&profile, nil, "", narrative, "hola", false)

	if strings.Contains(prompt, "Si aparece [ESTADO INTERNO]") {
		t.Fatalf("did not expect tension directive without state; got %q", prompt)
	}
}
