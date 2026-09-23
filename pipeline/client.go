// client.go: OpenRouter HTTP client (OpenAI chat/completions JSON).
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	httpTimeout    = 90 * time.Second
	maxBodyBytes   = 2 << 20 // 2 MiB
)

// Client talks to OpenRouter's chat completions API.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewClient builds a client. apiKey may be empty only for tests with a custom baseURL.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: httpTimeout},
	}
}

// NewClientFromEnv loads .env (if present) then reads OPENROUTER_API_KEY.
func NewClientFromEnv() (*Client, error) {
	LoadDotEnvDefault()
	key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is not set — add it to .env in the project root")
	}
	return NewClient(key), nil
}

// ChatCompletion POSTs /chat/completions and returns the parsed body.
func (c *Client) ChatCompletion(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("nil client")
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("missing API key")
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(c.baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "https://localhost/ubuntu-dev-assistant")
	httpReq.Header.Set("X-Title", "Ubuntu Dev Assistant")

	res, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}

	var out CompletionResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w (%s)", err, truncate(string(raw), 200))
	}
	if out.Error != nil && out.Error.Message != "" {
		return &out, fmt.Errorf("openrouter: %s", out.Error.Message)
	}
	if res.StatusCode >= 300 {
		return &out, fmt.Errorf("openrouter HTTP %d: %s", res.StatusCode, truncate(string(raw), 300))
	}
	if len(out.Choices) == 0 {
		return &out, fmt.Errorf("openrouter: empty choices (%s)", truncate(string(raw), 200))
	}
	return &out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
