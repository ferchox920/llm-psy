package llm

import "context"

// MockClient permite tests sin llamar a un LLM real.
type MockClient struct {
	Response string
	Err      error
}

func (m *MockClient) Generate(ctx context.Context, prompt string) (string, error) {
	return m.Response, m.Err
}
