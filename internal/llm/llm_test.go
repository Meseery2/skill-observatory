package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExtractJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "fenced", in: "```json\n{\"skills\":[\"a\"]}\n```", want: `{"skills":["a"]}`},
		{name: "prose wrapper", in: "here you go\n{\"skills\":[]}\n", want: `{"skills":[]}`},
		{name: "raw object", in: `{"winner":"a"}`, want: `{"winner":"a"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ExtractJSON(tt.in); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestNewMissingKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	_, err := New(Config{Provider: "openai"})
	if err == nil {
		t.Fatal("expected missing api key error")
	}
}

func TestNewUnknownProvider(t *testing.T) {
	_, err := New(Config{Provider: "palm", APIKey: "x"})
	if err == nil {
		t.Fatal("expected unknown provider error")
	}
}

func TestOpenAIComplete(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth = %s", got)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"skills":["canvas"]}`}},
			},
			"usage": map[string]int{"prompt_tokens": 11, "completion_tokens": 4},
		})
	}))
	t.Cleanup(srv.Close)

	client, err := New(Config{
		Provider: "openai-compatible",
		BaseURL:  srv.URL,
		Model:    "test-model",
		APIKey:   "test-key",
		HTTP:     srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Complete(context.Background(), Request{System: "sys", User: "hi", JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != `{"skills":["canvas"]}` {
		t.Fatalf("text = %q", resp.Text)
	}
	if resp.InputTokens != 11 || resp.OutputTokens != 4 {
		t.Fatalf("tokens = %d/%d", resp.InputTokens, resp.OutputTokens)
	}
	if resp.Latency < 0 || resp.Latency > time.Second {
		t.Fatalf("latency = %s", resp.Latency)
	}
}

func TestAnthropicComplete(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": `{"winner":"tie"}`}},
			"usage":   map[string]int{"input_tokens": 3, "output_tokens": 2},
		})
	}))
	t.Cleanup(srv.Close)

	client, err := New(Config{
		Provider: "anthropic",
		BaseURL:  srv.URL,
		Model:    "claude-test",
		APIKey:   "sk-ant",
		HTTP:     srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Complete(context.Background(), Request{User: "grade this", JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != `{"winner":"tie"}` {
		t.Fatalf("text = %q", resp.Text)
	}
}
