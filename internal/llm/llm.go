package llm

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

type Request struct {
	System      string
	User        string
	JSON        bool
	Temperature float64
	MaxTokens   int
}

type Response struct {
	Text         string
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
}

type Client interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

type Config struct {
	Provider  string
	BaseURL   string
	Model     string
	APIKey    string
	APIKeyEnv string
	HTTP      *http.Client
}

func New(cfg Config) (Client, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = "openai"
	}
	key := cfg.APIKey
	if key == "" && cfg.APIKeyEnv != "" {
		key = os.Getenv(cfg.APIKeyEnv)
	}
	if key == "" {
		switch provider {
		case "anthropic":
			key = os.Getenv("ANTHROPIC_API_KEY")
		default:
			key = os.Getenv("OPENAI_API_KEY")
		}
	}
	if key == "" {
		return nil, fmt.Errorf("missing api key for provider %s", provider)
	}
	httpClient := cfg.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}
	switch provider {
	case "anthropic":
		return &anthropicClient{http: httpClient, model: cfg.Model, key: key, baseURL: firstNonEmpty(cfg.BaseURL, "https://api.anthropic.com")}, nil
	case "openai", "openai-compatible":
		base := cfg.BaseURL
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		return &openAIClient{http: httpClient, model: cfg.Model, key: key, baseURL: strings.TrimRight(base, "/")}, nil
	default:
		return nil, fmt.Errorf("unknown llm provider %q", provider)
	}
}

type openAIClient struct {
	http    *http.Client
	model   string
	key     string
	baseURL string
}

func (c *openAIClient) Complete(ctx context.Context, req Request) (Response, error) {
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = 2048
	}
	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": req.System},
			{"role": "user", "content": req.User},
		},
		"temperature": req.Temperature,
		"max_tokens":  maxTok,
	}
	if req.JSON {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return Response{}, fmt.Errorf("encoding openai request: %w", err)
	}
	start := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return Response{}, fmt.Errorf("building openai request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.key)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("reading openai response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("openai status %d: %s", resp.StatusCode, truncate(string(payload), 400))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return Response{}, fmt.Errorf("decoding openai response: %w", err)
	}
	text := ""
	if len(parsed.Choices) > 0 {
		text = parsed.Choices[0].Message.Content
	}
	return Response{
		Text:         text,
		InputTokens:  parsed.Usage.PromptTokens,
		OutputTokens: parsed.Usage.CompletionTokens,
		Latency:      time.Since(start),
	}, nil
}

type anthropicClient struct {
	http    *http.Client
	model   string
	key     string
	baseURL string
}

func (c *anthropicClient) Complete(ctx context.Context, req Request) (Response, error) {
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = 2048
	}
	user := req.User
	if req.JSON {
		user += "\n\nReturn a single JSON object only, with no markdown."
	}
	body := map[string]any{
		"model":       c.model,
		"max_tokens":  maxTok,
		"temperature": req.Temperature,
		"system":      req.System,
		"messages": []map[string]string{
			{"role": "user", "content": user},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return Response{}, fmt.Errorf("encoding anthropic request: %w", err)
	}
	start := time.Now()
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(c.baseURL, "/")+"/v1/messages",
		bytes.NewReader(raw),
	)
	if err != nil {
		return Response{}, fmt.Errorf("building anthropic request: %w", err)
	}
	httpReq.Header.Set("x-api-key", c.key)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("reading anthropic response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, truncate(string(payload), 400))
	}
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return Response{}, fmt.Errorf("decoding anthropic response: %w", err)
	}
	var b strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	return Response{
		Text:         b.String(),
		InputTokens:  parsed.Usage.InputTokens,
		OutputTokens: parsed.Usage.OutputTokens,
		Latency:      time.Since(start),
	}, nil
}

func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			return s[i : j+1]
		}
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Scripted is a test double that returns canned responses in order.
type Scripted struct {
	Responses []Response
	Errs      []error
	Calls     []Request
}

func (s *Scripted) Complete(_ context.Context, req Request) (Response, error) {
	s.Calls = append(s.Calls, req)
	i := len(s.Calls) - 1
	if i < len(s.Errs) && s.Errs[i] != nil {
		return Response{}, s.Errs[i]
	}
	if i >= len(s.Responses) {
		return Response{}, fmt.Errorf("scripted llm: no response for call %d", i)
	}
	return s.Responses[i], nil
}
