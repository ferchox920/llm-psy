package llm

import (
	"bytes"
	"context"
	"errors"
	"net/http"
)

// HTTPClient implementa LLMClient usando HTTP contra un proveedor externo.
type HTTPClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewHTTPClient(baseURL, apiKey string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &HTTPClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  httpClient,
	}
}

func (c *HTTPClient) Generate(ctx context.Context, prompt string) (string, error) {
	// Placeholder: request ficticio; reemplazar con la API real.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/generate", bytes.NewBufferString(prompt))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// En el sprint actual no llamamos realmente; devuelve error o respuesta fija.
	return "", errors.New("llm http client not implemented")
}
