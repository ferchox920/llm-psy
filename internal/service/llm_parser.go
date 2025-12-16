package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"clone-llm/internal/domain"
)

// LLMResponseParser centraliza la lógica de limpieza y parseo de respuestas del LLM.
type LLMResponseParser struct{}

// DefaultLLMResponseParser permite uso directo sin instanciar.
var DefaultLLMResponseParser = LLMResponseParser{}

// ParseLLMResponseSafe intenta parsear la respuesta del LLM como JSON de manera robusta.
// Regla: nunca devolvemos inner_monologue en fallback.
func (LLMResponseParser) ParseLLMResponseSafe(raw string) (domain.LLMResponse, bool) {
	cleaned := CleanLLMJSONResponse(raw)

	jsonObj := extractFirstJSONObject(cleaned)
	if jsonObj == "" {
		jsonObj = extractFirstJSONObject(raw)
	}

	tryUnmarshal := func(candidate string) (domain.LLMResponse, bool) {
		var tmp struct {
			InnerMonologue string   `json:"inner_monologue"`
			PublicResponse string   `json:"public_response"`
			TrustDelta     *float64 `json:"trust_delta,omitempty"`
			IntimacyDelta  *float64 `json:"intimacy_delta,omitempty"`
			RespectDelta   *float64 `json:"respect_delta,omitempty"`
			NewState       string   `json:"new_state,omitempty"`
		}
		if err := json.Unmarshal([]byte(candidate), &tmp); err != nil {
			return domain.LLMResponse{}, false
		}
		pub := strings.TrimSpace(tmp.PublicResponse)
		if pub == "" {
			return domain.LLMResponse{}, false
		}

		pub = UnescapeMaybeDoubleEscaped(pub)

		return domain.LLMResponse{
			PublicResponse: pub,
			InnerMonologue: "",
		}, true
	}

	if jsonObj != "" {
		if resp, ok := tryUnmarshal(jsonObj); ok {
			return resp, true
		}
	}
	if resp, ok := tryUnmarshal(cleaned); ok {
		return resp, true
	}
	if resp, ok := tryUnmarshal(raw); ok {
		return resp, true
	}

	if pr, ok := ExtractPublicResponseByRegex(cleaned); ok {
		return domain.LLMResponse{PublicResponse: pr}, true
	}
	if pr, ok := ExtractPublicResponseByRegex(raw); ok {
		return domain.LLMResponse{PublicResponse: pr}, true
	}

	fallback := SanitizeFallbackPublicText(raw)
	if strings.TrimSpace(fallback) == "" {
		return domain.LLMResponse{}, false
	}
	return domain.LLMResponse{PublicResponse: fallback}, true
}

// JSONUnmarshalLLMResponse delega al parseo robusto para poblar un LLMResponse.
func (p LLMResponseParser) JSONUnmarshalLLMResponse(raw string, out *domain.LLMResponse) error {
	resp, ok := p.ParseLLMResponseSafe(raw)
	if !ok || strings.TrimSpace(resp.PublicResponse) == "" {
		return fmt.Errorf("could not extract public_response")
	}
	*out = resp
	return nil
}

// CleanLLMJSONResponse quita fences ```json ... ``` y BOM, dejando el contenido usable.
func CleanLLMJSONResponse(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	s = strings.TrimPrefix(s, "\uFEFF")

	reStart := regexp.MustCompile("(?is)^\\s*```(?:json)?\\s*")
	reEnd := regexp.MustCompile("(?is)\\s*```\\s*$")
	s = reStart.ReplaceAllString(s, "")
	s = reEnd.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// SanitizeFallbackPublicText es el último recurso cuando no hay JSON parseable.
// Regla: nunca devolvemos inner_monologue aunque venga en texto plano.
func SanitizeFallbackPublicText(raw string) string {
	t := strings.TrimSpace(CleanLLMJSONResponse(raw))
	if t == "" {
		return ""
	}

	if pr, ok := ExtractPublicResponseByRegex(t); ok {
		return pr
	}
	if pr, ok := ExtractPublicResponseByRegex(raw); ok {
		return pr
	}

	lower := strings.ToLower(t)
	if strings.Contains(lower, "inner_monologue") {
		lines := strings.Split(t, "\n")
		out := lines[:0]
		for _, ln := range lines {
			if strings.Contains(strings.ToLower(ln), "inner_monologue") {
				continue
			}
			out = append(out, ln)
		}
		t = strings.TrimSpace(strings.Join(out, "\n"))
	}

	if obj := extractFirstJSONObject(t); obj != "" {
		if pr, ok := ExtractPublicResponseByRegex(obj); ok {
			return pr
		}
	}

	return strings.TrimSpace(t)
}

// ExtractPublicResponseByRegex intenta extraer el valor de "public_response" aunque el JSON esté sucio.
// IMPORTANTE: evita leaks porque solo toma public_response.
func ExtractPublicResponseByRegex(s string) (string, bool) {
	re := regexp.MustCompile(`(?is)"public_response"\s*:\s*"((?:\\.|[^"\\])*)"`)
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return "", false
	}

	raw := m[1]
	unq, err := strconv.Unquote(`"` + raw + `"`)
	if err != nil {
		unq = unescapeMinimalEscapes(raw)
	}
	unq = strings.TrimSpace(UnescapeMaybeDoubleEscaped(unq))
	if unq == "" {
		return "", false
	}
	return unq, true
}

// UnescapeMaybeDoubleEscaped intenta arreglar casos donde el modelo manda texto doble-escapado.
func UnescapeMaybeDoubleEscaped(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	if !strings.Contains(s, `\`) {
		return s
	}

	quoted := `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	if unq, err := strconv.Unquote(quoted); err == nil {
		return strings.TrimSpace(unq)
	}

	return unescapeMinimalEscapes(s)
}

func unescapeMinimalEscapes(s string) string {
	replacer := strings.NewReplacer(
		`\\`, `\`,
		`\"`, `"`,
		`\n`, "\n",
		`\r`, "\r",
		`\t`, "\t",
	)
	return replacer.Replace(s)
}
