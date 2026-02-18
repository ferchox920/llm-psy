package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"clone-llm/internal/domain"
	"clone-llm/internal/llm"
)

type mockCloneProfileRepo struct {
	profile       domain.CloneProfile
	err           error
	lastGetByUser string
}

func (m *mockCloneProfileRepo) Create(context.Context, domain.CloneProfile) error {
	return errors.New("not implemented")
}

func (m *mockCloneProfileRepo) GetByID(context.Context, string) (domain.CloneProfile, error) {
	return m.profile, m.err
}

func (m *mockCloneProfileRepo) GetByUserID(_ context.Context, userID string) (domain.CloneProfile, error) {
	m.lastGetByUser = userID
	return m.profile, m.err
}

type mockCloneTraitRepo struct {
	traits []domain.Trait
	err    error
}

func (m *mockCloneTraitRepo) Upsert(context.Context, domain.Trait) error {
	return errors.New("not implemented")
}

func (m *mockCloneTraitRepo) FindByProfileID(context.Context, string) ([]domain.Trait, error) {
	return m.traits, m.err
}

func (m *mockCloneTraitRepo) FindByCategory(context.Context, string, string) ([]domain.Trait, error) {
	return nil, errors.New("not implemented")
}

type mockCloneMessageRepo struct {
	created []domain.Message
	err     error
}

func (m *mockCloneMessageRepo) Create(_ context.Context, message domain.Message) error {
	if m.err != nil {
		return m.err
	}
	m.created = append(m.created, message)
	return nil
}

func (m *mockCloneMessageRepo) ListBySessionID(context.Context, string) ([]domain.Message, error) {
	return nil, nil
}

type mockContextService struct {
	context string
	err     error
}

func (m *mockContextService) GetContext(context.Context, string) (string, error) {
	return m.context, m.err
}

func TestParseLLMResponseSafe_UnescapesEscapedQuotes(t *testing.T) {
	parser := DefaultLLMResponseParser
	raw := `{"inner_monologue":"x","public_response":"Dijo: \"hola\" y luego \\ fin","trust_delta":0,"intimacy_delta":0,"respect_delta":0}`

	resp, ok := parser.ParseLLMResponseSafe(raw)
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

func TestParseLLMResponseSafe_FallbackWithEscapedQuotes(t *testing.T) {
	parser := DefaultLLMResponseParser
	raw := `inner_monologue: bla bla "public_response":"Ah, \"amigos nuevos\", 隅eh? 隅Quiゼnes son?"`

	resp, ok := parser.ParseLLMResponseSafe(raw)
	if !ok {
		t.Fatalf("parseLLMResponseSafe returned ok=false")
	}

	want := `Ah, "amigos nuevos", 隅eh? 隅Quiゼnes son?`
	if resp.PublicResponse != want {
		t.Fatalf("public_response mismatch: got %q want %q", resp.PublicResponse, want)
	}
	if resp.InnerMonologue != "" {
		t.Fatalf("expected inner monologue to be empty in safe parse, got %q", resp.InnerMonologue)
	}
}

func TestParseLLMResponseSafe_EscapedQuotesAndSpanish(t *testing.T) {
	parser := DefaultLLMResponseParser
	raw := `{"inner_monologue":"...","public_response":"Ah, \"amigos nuevos\", 隅eh? 隅Quiゼnes son?"}`
	resp, ok := parser.ParseLLMResponseSafe(raw)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	want := `Ah, "amigos nuevos", 隅eh? 隅Quiゼnes son?`
	if resp.PublicResponse != want {
		t.Fatalf("unexpected public_response: got %q want %q", resp.PublicResponse, want)
	}
}

func TestParseLLMResponseSafe_EscapedBackslashesAndNewlines(t *testing.T) {
	parser := DefaultLLMResponseParser
	raw := `{"public_response":"Linea1\\nLinea2 con \\\\ ruta","trust_delta":0}`
	resp, ok := parser.ParseLLMResponseSafe(raw)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	want := "Linea1\nLinea2 con \\ ruta"
	if resp.PublicResponse != want {
		t.Fatalf("unexpected public_response: got %q want %q", resp.PublicResponse, want)
	}
}

func TestExtractPublicResponseByRegex_InvalidClosingQuote(t *testing.T) {
	parser := DefaultLLMResponseParser
	raw := `{"public_response":"Ah, \"amigos nuevos}`
	val, ok := ExtractPublicResponseByRegex(raw)
	if ok {
		t.Fatalf("expected regex extraction to fail on unterminated string, got %q", val)
	}
	resp, parsed := parser.ParseLLMResponseSafe(raw)
	if parsed && strings.HasSuffix(resp.PublicResponse, `\`) {
		t.Fatalf("fallback must not end with backslash: %q", resp.PublicResponse)
	}
}

func TestDetectHighTensionFromNarrative(t *testing.T) {
	engine := DefaultReactionEngine
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
			if got := engine.DetectHighTensionFromNarrative(tt.text); got != tt.want {
				t.Fatalf("DetectHighTensionFromNarrative(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestBuildClonePromptIncludesTensionDirectiveWhenStatePresent(t *testing.T) {
	builder := ClonePromptBuilder{}
	profile := domain.CloneProfile{Name: "Test", Bio: "bio"}
	narrative := "[ESTADO INTERNO]\n- Emocion residual dominante: IRA (por un conflicto reciente; el clon todavia siente esa emocion)."

	prompt := builder.BuildClonePrompt(&profile, nil, "", narrative, "hola", false)

	if !strings.Contains(prompt, "Si aparece [ESTADO INTERNO]") {
		t.Fatalf("expected tension directive when state present; got %q", prompt)
	}
}

func TestBuildClonePromptOmitsTensionDirectiveWhenNoState(t *testing.T) {
	builder := ClonePromptBuilder{}
	profile := domain.CloneProfile{Name: "Test", Bio: "bio"}
	narrative := "Resumen cualquiera sin estado interno"

	prompt := builder.BuildClonePrompt(&profile, nil, "", narrative, "hola", false)

	if strings.Contains(prompt, "Si aparece [ESTADO INTERNO]") {
		t.Fatalf("did not expect tension directive without state; got %q", prompt)
	}
	if strings.Contains(prompt, "REGLA DE PRIORIDAD") {
		t.Fatalf("did not expect conflict priority rule without state; got %q", prompt)
	}
}

func TestBuildClonePromptConflictOverridesTrivialities(t *testing.T) {
	builder := ClonePromptBuilder{}
	profile := domain.CloneProfile{Name: "Test", Bio: "bio"}

	cases := []struct {
		name           string
		narrative      string
		expectPriority bool
	}{
		{
			name:           "estado interno presente",
			narrative:      "[ESTADO INTERNO]\n- Emocion residual dominante: IRA (por un conflicto reciente; el clon todavia siente esa emocion).",
			expectPriority: true,
		},
		{
			name:           "conflicto presente",
			narrative:      "[CONFLICTO]\n- Hubo reproches y tension.",
			expectPriority: true,
		},
		{
			name:           "sin conflicto",
			narrative:      "",
			expectPriority: false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			prompt := builder.BuildClonePrompt(&profile, nil, "", tt.narrative, "hola", false)
			hasRule := strings.Contains(prompt, "REGLA DE PRIORIDAD")
			hasException := strings.Contains(prompt, "EXCEPTO cuando el CONTEXTO Y MEMORIA indiquen conflicto")
			hasMemoryRule := strings.Contains(prompt, "REGLA DE MEMORIA")
			hasNoPast := strings.Contains(prompt, "antes/la otra vez/intercambio anterior")
			hasOpening := strings.Contains(prompt, "REGLA DE APERTURA (OBLIGATORIA)")
			hasAntiMetaphor := strings.Contains(prompt, "REGLA ANTI-METAFORA TRIVIAL")
			hasQuota := strings.Contains(prompt, "REGLA DE CUOTA TRIVIAL")
			hasDirectQuestion := strings.Contains(prompt, "REGLA DE PREGUNTA DIRECTA")
			if tt.expectPriority {
				if !hasRule {
					t.Fatalf("expected conflict priority rule; got %q", prompt)
				}
				if !hasException {
					t.Fatalf("expected triviality exception when conflict context present; got %q", prompt)
				}
				if !hasMemoryRule {
					t.Fatalf("expected memory safeguard rule when conflict context present; got %q", prompt)
				}
				if !hasNoPast {
					t.Fatalf("expected prohibition of past references when conflict context present; got %q", prompt)
				}
				if !hasOpening {
					t.Fatalf("expected opening rule present; got %q", prompt)
				}
				if !hasAntiMetaphor {
					t.Fatalf("expected anti-metaphor rule present; got %q", prompt)
				}
				if !hasQuota {
					t.Fatalf("expected trivial quota rule present; got %q", prompt)
				}
				if !hasDirectQuestion {
					t.Fatalf("expected direct question rule present; got %q", prompt)
				}
			} else {
				if hasRule || hasException || hasMemoryRule {
					t.Fatalf("did not expect conflict priority rule/exception; got %q", prompt)
				}
				if hasNoPast {
					t.Fatalf("did not expect past-reference rule without conflict context; got %q", prompt)
				}
			}
		})
	}
}

func TestBuildClonePromptTrivialInputWithNegativeState(t *testing.T) {
	builder := ClonePromptBuilder{}
	profile := domain.CloneProfile{Name: "Test", Bio: "bio"}
	narrative := "[ESTADO INTERNO]\n- Emocion residual dominante: IRA (estado tenso)\n\n[CONFLICTO]\n- Recuerdo: eres un inカtil."

	prompt := builder.BuildClonePrompt(&profile, nil, "", narrative, "hola", true)

	if !strings.Contains(prompt, "REGLA DE TRIVIALIDAD CONFLICTIVA") {
		t.Fatalf("expected triviality-conflict rule in prompt; got %q", prompt)
	}
	if !strings.Contains(prompt, "no hagas small talk largo") {
		t.Fatalf("expected explicit no small talk rule; got %q", prompt)
	}
	if !strings.Contains(prompt, "antes/la otra vez/intercambio anterior") {
		t.Fatalf("expected prohibition of past references to remain; got %q", prompt)
	}
	if !strings.Contains(prompt, "NO cites frases textuales") {
		t.Fatalf("expected memory citation guard to remain; got %q", prompt)
	}
	if !strings.Contains(prompt, "Pregunta una sola cosa para aclarar") {
		t.Fatalf("expected guidance to ask only one question; got %q", prompt)
	}
}

func TestBuildClonePromptTensionTable(t *testing.T) {
	builder := ClonePromptBuilder{}
	profile := domain.CloneProfile{Name: "Test", Bio: "bio"}

	cases := []struct {
		name      string
		narrative string
		trivial   bool
		expect    []string
	}{
		{
			name:      "estado interno exige tension contenida",
			narrative: "[ESTADO INTERNO]\n- Emocion residual dominante: IRA (sigue dolido)\n",
			trivial:   false,
			expect: []string{
				"Si aparece [ESTADO INTERNO] con emocion negativa residual",
			},
		},
		{
			name:      "conflicto pide prioridad sobre small talk",
			narrative: "[CONFLICTO]\n- Hubo reproches y tension.\n[ESTADO INTERNO]\n- Emocion residual dominante: IRA",
			trivial:   false,
			expect: []string{
				"REGLA DE PRIORIDAD",
				"EXCEPTO cuando el CONTEXTO Y MEMORIA indiquen conflicto",
				"REGLA DE MEMORIA",
				"antes/la otra vez/intercambio anterior",
				"REGLA DE APERTURA (OBLIGATORIA)",
				"REGLA ANTI-METAFORA TRIVIAL",
				"REGLA DE CUOTA TRIVIAL",
				"REGLA DE PREGUNTA DIRECTA",
			},
		},
		{
			name:      "trivial input pero estado interno negativo",
			narrative: "[ESTADO INTERNO]\n- Emocion residual dominante: IRA\n[CONFLICTO]\n- Me dijeron inutil",
			trivial:   true,
			expect: []string{
				"REGLA DE APERTURA (OBLIGATORIA)",
				"REGLA ANTI-METAFORA TRIVIAL",
				"REGLA DE CUOTA TRIVIAL",
				"REGLA DE PREGUNTA DIRECTA",
				"REGLA DE TRIVIALIDAD CONFLICTIVA",
				"no hagas small talk largo",
				"NO cites frases textuales",
				"Pregunta una sola cosa para aclarar",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prompt := builder.BuildClonePrompt(&profile, nil, "", tc.narrative, "hola", tc.trivial)
			for _, sub := range tc.expect {
				if !strings.Contains(prompt, sub) {
					t.Fatalf("expected substring %q in prompt; got %q", sub, prompt)
				}
			}
		})
	}
}

func TestBuildClonePromptRelationshipGuidanceHints(t *testing.T) {
	builder := ClonePromptBuilder{}
	profile := domain.CloneProfile{Name: "Test", Bio: "bio"}
	narrative := "[CONFLICTO]\n- Hubo reproches y tension.\n[ESTADO INTERNO]\n- Emocion residual dominante: IRA"

	prompt := builder.BuildClonePrompt(&profile, nil, "context", narrative, "hola", false)

	if !strings.Contains(prompt, "Evita interrogatorio") {
		t.Fatalf("expected guidance to avoid interrogatorio; got %q", prompt)
	}
	if !strings.Contains(prompt, "Maximo 1 pregunta") {
		t.Fatalf("expected guidance about max 1 pregunta; got %q", prompt)
	}
}

func TestCloneServiceChat_HappyPathPersistsMessage(t *testing.T) {
	profileRepo := &mockCloneProfileRepo{
		profile: domain.CloneProfile{
			ID:   "8f4ac247-4ec1-4f8f-9d1c-f53d8e7b4207",
			Name: "Clone",
			Bio:  "Bio",
			Big5: domain.Big5Profile{Neuroticism: 10},
		},
	}
	traitRepo := &mockCloneTraitRepo{
		traits: []domain.Trait{{Trait: "openness", Value: 50}},
	}
	messageRepo := &mockCloneMessageRepo{}
	contextSvc := &mockContextService{context: "User: Hola"}
	llmClient := &llm.MockClient{
		Response: `{"public_response":"Respuesta del clon"}`,
	}
	svc := NewCloneService(
		llmClient,
		messageRepo,
		profileRepo,
		traitRepo,
		contextSvc,
		nil,
		nil,
		ClonePromptBuilder{},
		LLMResponseParser{},
		ReactionEngine{},
	)

	msg, dbg, err := svc.Chat(context.Background(), " user-1 ", " s1 ", " hola ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if dbg == nil {
		t.Fatalf("expected interaction debug")
	}
	if profileRepo.lastGetByUser != "user-1" {
		t.Fatalf("expected trimmed user id for profile lookup, got %q", profileRepo.lastGetByUser)
	}
	if msg.Content != "Respuesta del clon" {
		t.Fatalf("expected parsed response, got %q", msg.Content)
	}
	if msg.UserID != "user-1" || msg.SessionID != "s1" {
		t.Fatalf("expected trimmed ids in message, got user=%q session=%q", msg.UserID, msg.SessionID)
	}
	if len(messageRepo.created) != 1 {
		t.Fatalf("expected one persisted message, got %d", len(messageRepo.created))
	}
	if messageRepo.created[0].Role != "clone" {
		t.Fatalf("expected role clone, got %q", messageRepo.created[0].Role)
	}
	if time.Since(messageRepo.created[0].CreatedAt) > 2*time.Minute {
		t.Fatalf("expected recent created_at, got %v", messageRepo.created[0].CreatedAt)
	}
}

func TestCloneServiceChat_InvalidInput(t *testing.T) {
	svc := NewCloneService(
		&llm.MockClient{},
		&mockCloneMessageRepo{},
		&mockCloneProfileRepo{},
		&mockCloneTraitRepo{},
		&mockContextService{},
		nil,
		nil,
		ClonePromptBuilder{},
		LLMResponseParser{},
		ReactionEngine{},
	)

	_, _, err := svc.Chat(context.Background(), "", "s1", "hola")
	if !errors.Is(err, ErrCloneInvalidInput) {
		t.Fatalf("expected ErrCloneInvalidInput for empty user id, got %v", err)
	}

	_, _, err = svc.Chat(context.Background(), "u1", "s1", "   ")
	if !errors.Is(err, ErrCloneInvalidInput) {
		t.Fatalf("expected ErrCloneInvalidInput for empty message, got %v", err)
	}
}

func TestCloneServiceChat_FallbackResponseWhenParserReturnsEmpty(t *testing.T) {
	profileRepo := &mockCloneProfileRepo{
		profile: domain.CloneProfile{
			ID:   "8f4ac247-4ec1-4f8f-9d1c-f53d8e7b4207",
			Name: "Clone",
			Bio:  "Bio",
		},
	}
	svc := NewCloneService(
		&llm.MockClient{Response: "   "},
		&mockCloneMessageRepo{},
		profileRepo,
		&mockCloneTraitRepo{},
		&mockContextService{},
		nil,
		nil,
		ClonePromptBuilder{},
		LLMResponseParser{},
		ReactionEngine{},
	)

	msg, _, err := svc.Chat(context.Background(), "u1", "s1", "hola")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if msg.Content == "" {
		t.Fatalf("expected non-empty fallback response")
	}
}
