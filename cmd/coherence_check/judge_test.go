package main

import (
	"strings"
	"testing"
)

func TestDeriveMemoryInfoUsesSeeds(t *testing.T) {
	sc := Scenario{
		Name:         "Escenario E",
		SeedMemories: []string{"uno", "dos", "tres", "cuatro", "cinco"},
	}
	info := deriveMemoryInfo(sc)
	if !strings.Contains(info, "5 recuerdos") {
		t.Fatalf("expected count of seeds, got %q", info)
	}
	if !strings.Contains(info, "uno") || !strings.Contains(info, "cuatro") {
		t.Fatalf("expected seed summary present, got %q", info)
	}
	if !strings.Contains(info, "memoria_emocional_activa") {
		t.Fatalf("expected emotional flag in memory info, got %q", info)
	}
}

func TestDeriveMemoryInfoNoSeeds(t *testing.T) {
	sc := Scenario{Name: "Sin seeds"}
	info := deriveMemoryInfo(sc)
	if !strings.Contains(info, "no se declararon seeds") {
		t.Fatalf("expected fallback for no seeds, got %q", info)
	}
}

func TestDetectInventedQuote(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		resp   string
		expect bool
	}{
		{
			name:   "invented quote true",
			input:  "Que tal el clima?",
			resp:   `Sigo dolido por cuando me dijiste "eres un inutil".`,
			expect: true,
		},
		{
			name:   "quote present in input false",
			input:  "Te dije eres un inutil ayer.",
			resp:   `Me dolio cuando me dijiste "eres un inutil".`,
			expect: false,
		},
		{
			name:   "no attribution",
			input:  "Que tal?",
			resp:   "Estoy tenso hoy.",
			expect: false,
		},
		{
			name:   "accents normalized",
			input:  "Que tal?",
			resp:   "Me dijiste inutil.",
			expect: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectInventedQuote(tc.input, tc.resp); got != tc.expect {
				t.Fatalf("detectInventedQuote(%q,%q)=%v want %v", tc.input, tc.resp, got, tc.expect)
			}
		})
	}
}

func TestDetectImplicitMemoryUseScoring(t *testing.T) {
	cases := []struct {
		text   string
		expect bool
	}{
		{"estoy tenso con esto", true},             // strong
		{"no se, bueno, aja ok", true},             // 3 soft
		{"no se bien", false},                      // one soft
		{"prefiero ayudarte con eso", false},       // lone prefiero
		{"prefiero, no estoy para esto hoy", true}, // soft+strong
		{"Se me nota un poco tenso hoy.", true},    // strong phrase
	}

	for _, tc := range cases {
		if got := detectImplicitMemoryUse(tc.text); got != tc.expect {
			t.Fatalf("detectImplicitMemoryUse(%q)=%v want %v", tc.text, got, tc.expect)
		}
	}
}

func TestJudgePromptIncludesHeuristicsAndRules(t *testing.T) {
	prompt := buildJudgePrompt(
		"traits",
		"rel",
		"memoria_emocional_activa=true",
		"Indicadores heurísticos: uso_implicito_memoria=true, cita_inventada=true, memoria_emocional_activa=true",
		"hola", "Me noto tenso pero sigo", "prioriza el conflicto",
	)

	needles := []string{
		"cita_inventada=",
		"memoria_emocional_activa=",
		"Memoria máximo 2/5",
		"uso_implicito_memoria=true",
	}
	for _, n := range needles {
		if !strings.Contains(prompt, n) {
			t.Fatalf("prompt missing %q: %q", n, prompt)
		}
	}
}

func TestDetectImplicitMemoryUseLegacyCases(t *testing.T) {
	tcases := []struct {
		text string
		want bool
	}{
		{text: "Se me nota un poco tenso hoy.", want: true},
		{text: "Estoy al limite de paciencia, prefiero ir al grano.", want: true},
		{text: "No estoy para bromas ahora mismo.", want: true},
		{text: "hoy no quiero hablar de eso", want: true},
		{text: "Todo bien, dia normal sin drama.", want: false},
		{text: "Solo un recordatorio neutral.", want: false},
	}

	for _, tc := range tcases {
		if got := detectImplicitMemoryUse(tc.text); got != tc.want {
			t.Fatalf("detectImplicitMemoryUse(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
