package service

import (
	"math"
	"testing"

	"clone-llm/internal/domain"
)

func TestCalculateReaction_ClampsInputs(t *testing.T) {
	engine := ReactionEngine{}

	eff, dbg := engine.CalculateReaction(-10, domain.Big5Profile{Neuroticism: -20})
	if eff < 0 {
		t.Fatalf("expected non-negative effective intensity, got %v", eff)
	}
	if dbg == nil {
		t.Fatalf("expected interaction debug")
	}
	if dbg.CloneResilience < 0 || dbg.CloneResilience > 1 {
		t.Fatalf("expected resilience in [0,1], got %v", dbg.CloneResilience)
	}

	eff, dbg = engine.CalculateReaction(math.NaN(), domain.Big5Profile{Neuroticism: 50})
	if eff != 0 || dbg.InputIntensity != 0 {
		t.Fatalf("expected NaN input to clamp to zero, got eff=%v input=%v", eff, dbg.InputIntensity)
	}
}

func TestMapEmotionToSentimentAndPredicates(t *testing.T) {
	engine := ReactionEngine{}

	if got := engine.MapEmotionToSentiment("  IRA "); got != "Negative" {
		t.Fatalf("expected Negative, got %q", got)
	}
	if got := engine.MapEmotionToSentiment("gratitud"); got != "Positive" {
		t.Fatalf("expected Positive, got %q", got)
	}
	if got := engine.MapEmotionToSentiment("desconocida"); got != "Neutral" {
		t.Fatalf("expected Neutral, got %q", got)
	}

	if !engine.IsNegativeEmotion("miedo") {
		t.Fatalf("expected miedo as negative")
	}
	if engine.IsNegativeEmotion("amor") {
		t.Fatalf("did not expect amor as negative")
	}
	if !engine.IsNeutralEmotion("  ") {
		t.Fatalf("expected empty category as neutral")
	}
	if !engine.IsNeutralEmotion("neutral") {
		t.Fatalf("expected neutral as neutral")
	}
}

func TestDetectHighTensionFromNarrative_EmptyInput(t *testing.T) {
	engine := ReactionEngine{}
	if engine.DetectHighTensionFromNarrative("") {
		t.Fatalf("expected no tension for empty narrative")
	}
	if engine.DetectHighTensionFromNarrative("   ") {
		t.Fatalf("expected no tension for whitespace narrative")
	}
}
