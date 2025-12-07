package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMClient define la interfaz para generar respuestas con un LLM.
type LLMClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
}

type logger interface {
	Printf(format string, v ...interface{})
}

// HTTPClient implementa LLMClient usando la API de OpenAI-compatible.
type HTTPClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
	logger  logger
}

// NewHTTPClient construye un cliente HTTP apuntando a la API de chat completions.
func NewHTTPClient(baseURL, apiKey, model string, log any) *HTTPClient {
	l, _ := log.(logger)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
		logger:  l,
	}
}

func (c *HTTPClient) Generate(ctx context.Context, prompt string) (string, error) {
	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if c.logger != nil {
			c.logger.Printf("llm error status %d: %s", resp.StatusCode, string(respBody))
		}
		return "", fmt.Errorf("llm http error: status=%d", resp.StatusCode)
	}

	var cr chatResponse
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if cr.Error != nil {
		return "", fmt.Errorf("llm api error: %s", cr.Error.Message)
	}

	if len(cr.Choices) == 0 || cr.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("llm empty response")
	}

	return cr.Choices[0].Message.Content, nil
}

// CreateEmbedding obtiene el embedding del texto usando el endpoint de embeddings.
func (c *HTTPClient) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	payload := map[string]any{
		"model": "text-embedding-3-small",
		"input": text,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	url := c.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do embedding request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if c.logger != nil {
			c.logger.Printf("embedding error status %d: %s", resp.StatusCode, string(respBody))
		}
		return nil, fmt.Errorf("embedding http error: status=%d", resp.StatusCode)
	}

	var er embeddingResponse
	if err := json.Unmarshal(respBody, &er); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response: %w", err)
	}
	if er.Error != nil {
		return nil, fmt.Errorf("embedding api error: %s", er.Error.Message)
	}
	if len(er.Data) == 0 || len(er.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedding empty response")
	}

	return er.Data[0].Embedding, nil
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}
