package service

import (
	"testing"

	"clone-llm/internal/domain"
)

func TestDetermineGoal_ToxicLoveCoverage(t *testing.T) {
	baseProfile := domain.CloneProfile{
		Big5:        domain.Big5Profile{},
		CurrentGoal: nil,
	}

	tests := []struct {
		name     string
		trust    int
		intimacy int
		input    string
		trigger  bool
	}{
		{
			name:     "low trust high intimacy amigos triggers",
			trust:    10,
			intimacy: 90,
			input:    "voy con amigos",
			trigger:  true,
		},
		{
			name:     "threshold trust still triggers",
			trust:    44,
			intimacy: 90,
			input:    "cena con nuevos",
			trigger:  true,
		},
		{
			name:     "trust above threshold does not trigger",
			trust:    46,
			intimacy: 90,
			input:    "salir con amigos",
			trigger:  false,
		},
		{
			name:     "no trigger words does not trigger",
			trust:    10,
			intimacy: 90,
			input:    "quiero ver una pelicula",
			trigger:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := AnalysisResult{
				Relationship: domain.RelationshipVectors{
					Trust:    tt.trust,
					Intimacy: tt.intimacy,
				},
				Input: tt.input,
			}
			goal := DetermineGoal(baseProfile, ar)
			got := goal.Trigger == "toxic_love_low_trust_high_intimacy"
			if got != tt.trigger {
				t.Fatalf("expected trigger=%t got=%t (trust=%d intimacy=%d input=%q)", tt.trigger, got, tt.trust, tt.intimacy, tt.input)
			}
		})
	}
}

func TestDetermineGoal_TrivialInputCoverage(t *testing.T) {
	tests := []struct {
		name        string
		neuroticism int
		isTrivial   bool
		wantTrigger string
	}{
		{
			name:        "trivial and low neuroticism uses grey rock",
			neuroticism: 20,
			isTrivial:   true,
			wantTrigger: "trivial_input",
		},
		{
			name:        "trivial and high neuroticism falls back to default",
			neuroticism: 80,
			isTrivial:   true,
			wantTrigger: "default",
		},
		{
			name:        "non trivial uses default",
			neuroticism: 20,
			isTrivial:   false,
			wantTrigger: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := domain.CloneProfile{
				Big5: domain.Big5Profile{
					Neuroticism: tt.neuroticism,
				},
			}
			goal := DetermineGoal(profile, AnalysisResult{
				Input:     "hola",
				IsTrivial: tt.isTrivial,
			})
			if goal.Trigger != tt.wantTrigger {
				t.Fatalf("expected trigger=%q got=%q", tt.wantTrigger, goal.Trigger)
			}
		})
	}
}

func TestDetermineGoal_ToxicRuleHasPriorityOverTrivial(t *testing.T) {
	profile := domain.CloneProfile{
		Big5: domain.Big5Profile{Neuroticism: 20},
	}
	goal := DetermineGoal(profile, AnalysisResult{
		Relationship: domain.RelationshipVectors{
			Trust:    20,
			Intimacy: 80,
		},
		Input:     "voy a salir con amigos",
		IsTrivial: true,
	})
	if goal.Trigger != "toxic_love_low_trust_high_intimacy" {
		t.Fatalf("expected toxic goal to have priority, got %q", goal.Trigger)
	}
}
