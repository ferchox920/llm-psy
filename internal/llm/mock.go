package llm

import "context"

// MockClient permite tests sin llamar a un LLM real.
type MockClient struct {
	Response       string
	Err            error
	Embedding      []float32
	EmbeddingError error
}

func (m *MockClient) Generate(ctx context.Context, prompt string) (string, error) {
	return m.Response, m.Err
}

func (m *MockClient) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if m.EmbeddingError != nil {
		return nil, m.EmbeddingError
	}
	return m.Embedding, nil
}
