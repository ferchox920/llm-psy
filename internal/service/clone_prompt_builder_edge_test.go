package service

import (
	"strings"
	"testing"

	"clone-llm/internal/domain"
)

func TestBuildClonePrompt_NilProfileDoesNotPanic(t *testing.T) {
	builder := ClonePromptBuilder{}
	prompt := builder.BuildClonePrompt(nil, nil, "", "", "hola", false)

	if strings.TrimSpace(prompt) == "" {
		t.Fatalf("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "Eres Clon.") {
		t.Fatalf("expected default profile name in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "MENSAJE DEL USUARIO") {
		t.Fatalf("expected user message section")
	}
}

func TestBuildClonePrompt_EmptyTraitsSection(t *testing.T) {
	builder := ClonePromptBuilder{}
	prompt := builder.BuildClonePrompt(&domain.CloneProfile{Name: "X"}, nil, "", "", "hola", false)
	if !strings.Contains(prompt, "- Sin rasgos inferidos aun.") {
		t.Fatalf("expected fallback traits line, got %q", prompt)
	}
}
