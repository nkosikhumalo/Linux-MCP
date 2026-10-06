// client.go: OpenRouter HTTP client (OpenAI chat/completions JSON).
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	local   bool
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

// NewClientFromEnv loads .env and selects a model transport. Explicit local mode
// never falls back to cloud. If AI_MODE is unset, an existing OpenRouter key is
// treated as the users prior explicit configuration for backward compatibility.
func NewClientFromEnv() (*Client, error) {
	LoadDotEnvDefault()
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("AI_MODE")))
	baseURL := strings.TrimSpace(os.Getenv("LOCAL_MODEL_BASE_URL"))
	model := strings.TrimSpace(os.Getenv("LOCAL_MODEL_NAME"))
	key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	switch mode {
	case "":
		if baseURL != "" && model != "" {
			if !isLocalEndpoint(baseURL) {
				return nil, fmt.Errorf("LOCAL_MODEL_BASE_URL must use HTTP and a loopback IP address (127.0.0.1 or ::1)")
			}
			return newLocalClient(baseURL), nil
		}
		if key != "" {
			return NewClient(key), nil
		}
		return nil, fmt.Errorf("configure a local model with LOCAL_MODEL_BASE_URL and LOCAL_MODEL_NAME, or configure OPENROUTER_API_KEY and set AI_MODE=openrouter")
	case "local":
		if baseURL == "" || model == "" {
			return nil, fmt.Errorf("AI_MODE=local requires LOCAL_MODEL_BASE_URL and LOCAL_MODEL_NAME; cloud fallback is disabled")
		}
		if !isLocalEndpoint(baseURL) {
			return nil, fmt.Errorf("LOCAL_MODEL_BASE_URL must use HTTP and a loopback IP address (127.0.0.1 or ::1)")
		}
		return newLocalClient(baseURL), nil
	case "openrouter":
		if key == "" {
			return nil, fmt.Errorf("OPENROUTER_API_KEY is not set — add it to .env in the project root")
		}
		return NewClient(key), nil
	default:
		return nil, fmt.Errorf("AI_MODE must be local or openrouter")
	}
}

func newLocalClient(baseURL string) *Client {
	return &Client{baseURL: baseURL, local: true, http: &http.Client{Timeout: httpTimeout, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func isLocalEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback() && (u.Port() == "" || validPort(u.Port()))
}

func validPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

// ChatCompletion POSTs /chat/completions and returns the parsed body.
func (c *Client) ChatCompletion(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("nil client")
	}
	if !c.local && c.apiKey == "" {
		return nil, fmt.Errorf("missing API key")
	}
	if c.local && !isLocalEndpoint(c.baseURL) {
		return nil, fmt.Errorf("local model endpoint must be an HTTP loopback address (127.0.0.1 or ::1)")
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
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if !c.local {
		httpReq.Header.Set("HTTP-Referer", "https://localhost/ubuntu-dev-assistant")
		httpReq.Header.Set("X-Title", "Ubuntu Dev Assistant")
	}

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
		return &out, fmt.Errorf("model API: %s", out.Error.Message)
	}
	if res.StatusCode >= 300 {
		return &out, fmt.Errorf("model API HTTP %d: %s", res.StatusCode, truncate(string(raw), 300))
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
