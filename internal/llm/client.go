package llm

import "context"

// LLMClient define la interfaz para generar respuestas con un LLM.
type LLMClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
}
