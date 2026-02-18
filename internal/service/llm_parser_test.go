package service

import (
	"strings"
	"testing"

	"clone-llm/internal/domain"
)

func TestSanitizeFallbackPublicText_RemovesInnerMonologueJSON(t *testing.T) {
	raw := `{"inner_monologue":"secreto interno del modelo"}`
	got := SanitizeFallbackPublicText(raw)
	if got != "" {
		t.Fatalf("expected empty fallback when only inner_monologue exists, got %q", got)
	}
}

func TestSanitizeFallbackPublicText_PreservesPlainTextWithoutJSON(t *testing.T) {
	raw := "respuesta directa sin json"
	got := SanitizeFallbackPublicText(raw)
	if got != "respuesta directa sin json" {
		t.Fatalf("unexpected fallback text: %q", got)
	}
}

func TestJSONUnmarshalLLMResponse_NilOutput(t *testing.T) {
	parser := LLMResponseParser{}
	err := parser.JSONUnmarshalLLMResponse(`{"public_response":"ok"}`, nil)
	if err == nil || !strings.Contains(err.Error(), "nil output") {
		t.Fatalf("expected nil output error, got %v", err)
	}
}

func TestJSONUnmarshalLLMResponse_SetsOutput(t *testing.T) {
	parser := LLMResponseParser{}
	var out domain.LLMResponse
	if err := parser.JSONUnmarshalLLMResponse(`{"public_response":"hola"}`, &out); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.PublicResponse != "hola" {
		t.Fatalf("expected public_response hola, got %q", out.PublicResponse)
	}
	if out.InnerMonologue != "" {
		t.Fatalf("expected redacted inner_monologue, got %q", out.InnerMonologue)
	}
}
