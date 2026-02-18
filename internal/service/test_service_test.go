package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestTestServiceGenerateInitialQuestions(t *testing.T) {
	svc := NewTestService(nil, nil, zap.NewNop())
	questions := svc.GenerateInitialQuestions()
	if len(questions) != 15 {
		t.Fatalf("expected 15 questions, got %d", len(questions))
	}
	if strings.TrimSpace(questions[0]) == "" || strings.TrimSpace(questions[len(questions)-1]) == "" {
		t.Fatalf("questions must not be empty")
	}
}

func TestAnalyzeTestResponses_Validation(t *testing.T) {
	var nilSvc *TestService
	if err := nilSvc.AnalyzeTestResponses(context.Background(), "u1", map[string]string{"q": "a"}); !errors.Is(err, ErrTestServiceNotConfigured) {
		t.Fatalf("expected ErrTestServiceNotConfigured, got %v", err)
	}

	svc := NewTestService(nil, nil, zap.NewNop())
	if err := svc.AnalyzeTestResponses(context.Background(), "u1", map[string]string{"q": "a"}); !errors.Is(err, ErrTestServiceNotConfigured) {
		t.Fatalf("expected ErrTestServiceNotConfigured without analyze function, got %v", err)
	}

	svc.analyzeFn = func(ctx context.Context, userID, text string) error { return nil }
	if err := svc.AnalyzeTestResponses(context.Background(), "   ", map[string]string{"q": "a"}); !errors.Is(err, ErrTestServiceInvalidInput) {
		t.Fatalf("expected ErrTestServiceInvalidInput for empty user id, got %v", err)
	}
	if err := svc.AnalyzeTestResponses(context.Background(), "u1", map[string]string{}); !errors.Is(err, ErrTestServiceInvalidInput) {
		t.Fatalf("expected ErrTestServiceInvalidInput for empty responses, got %v", err)
	}
}

func TestAnalyzeTestResponses_DelegatesAndBuildsStableText(t *testing.T) {
	svc := NewTestService(nil, nil, zap.NewNop())
	var capturedUserID, capturedText string
	svc.analyzeFn = func(ctx context.Context, userID, text string) error {
		capturedUserID = userID
		capturedText = text
		return nil
	}

	questions := svc.GenerateInitialQuestions()
	responses := map[string]string{
		questions[2]:         " respuesta 3 ",
		questions[0]:         " respuesta 1 ",
		"zzz pregunta extra": "  extra  ",
	}
	if err := svc.AnalyzeTestResponses(context.Background(), " u1 ", responses); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedUserID != "u1" {
		t.Fatalf("expected trimmed user id, got %q", capturedUserID)
	}

	idxQ1 := strings.Index(capturedText, "P: "+questions[0])
	idxQ3 := strings.Index(capturedText, "P: "+questions[2])
	idxExtra := strings.Index(capturedText, "P: zzz pregunta extra")
	if idxQ1 == -1 || idxQ3 == -1 || idxExtra == -1 {
		t.Fatalf("expected all questions in text, got: %q", capturedText)
	}
	if !(idxQ1 < idxQ3 && idxQ3 < idxExtra) {
		t.Fatalf("expected deterministic order (preferred then extras), got: %q", capturedText)
	}
	if strings.Contains(capturedText, " respuesta 3 ") || strings.Contains(capturedText, "  extra  ") {
		t.Fatalf("expected trimmed answers in analysis text, got: %q", capturedText)
	}
}

func TestAnalyzeTestResponses_PropagatesAnalysisError(t *testing.T) {
	svc := NewTestService(nil, nil, zap.NewNop())
	svc.analyzeFn = func(ctx context.Context, userID, text string) error {
		return errors.New("boom")
	}
	err := svc.AnalyzeTestResponses(context.Background(), "u1", map[string]string{"q": "a"})
	if err == nil || !strings.Contains(err.Error(), "failed to analyze and persist test traits") {
		t.Fatalf("expected wrapped analysis error, got %v", err)
	}
}
