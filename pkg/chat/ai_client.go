package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Tuananh165-art/NexusChat/pkg/config"
)

type AIRewriteRequest struct {
	Text   string `json:"text" binding:"required"`
	Tone   string `json:"tone,omitempty"`
	Locale string `json:"locale,omitempty"`
}

type AIRewriteResponse struct {
	Text     string `json:"text"`
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
}

type AIClient interface {
	Rewrite(ctx context.Context, req *AIRewriteRequest) (*AIRewriteResponse, error)
}

type AIClientImpl struct {
	baseURL string
	client  *http.Client
}

func NewAIClientImpl(config *config.Config) *AIClientImpl {
	baseURL := "http://localhost:8090"
	timeoutMilli := int64(30000)
	if config.AI != nil {
		if config.AI.BaseURL != "" {
			baseURL = config.AI.BaseURL
		}
		if config.AI.RequestTimeoutMilli > 0 {
			timeoutMilli = config.AI.RequestTimeoutMilli
		}
	}
	timeout := time.Duration(timeoutMilli) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &AIClientImpl{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *AIClientImpl) Rewrite(ctx context.Context, req *AIRewriteRequest) (*AIRewriteResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal ai rewrite request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/assistant/rewrite",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create ai rewrite request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send ai rewrite request: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if err := resp.Body.Close(); err != nil {
			return nil, fmt.Errorf("close ai rewrite error response: %w", err)
		}
		return nil, fmt.Errorf("ai rewrite request failed with status %d", resp.StatusCode)
	}

	var result AIRewriteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, fmt.Errorf("decode ai rewrite response: %w; close response body: %v", err, closeErr)
		}
		return nil, fmt.Errorf("decode ai rewrite response: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("close ai rewrite response: %w", err)
	}
	return &result, nil
}

type AIService interface {
	Rewrite(ctx context.Context, req *AIRewriteRequest) (*AIRewriteResponse, error)
}

type AIServiceImpl struct {
	client AIClient
}

func NewAIServiceImpl(client AIClient) *AIServiceImpl {
	return &AIServiceImpl{client: client}
}

func (svc *AIServiceImpl) Rewrite(ctx context.Context, req *AIRewriteRequest) (*AIRewriteResponse, error) {
	return svc.client.Rewrite(ctx, req)
}
