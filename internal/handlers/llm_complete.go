package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/calionauta/turvez/internal/secret"
	"github.com/danmestas/dagnats/worker"
)

// LLMMessage is one OpenAI-compatible chat message.
type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLMCompleteInput is the task payload for the "llm.complete" handler.
type LLMCompleteInput struct {
	Model        string       `json:"model"`
	Messages     []LLMMessage `json:"messages"`
	APIKeySecret string       `json:"api_key_secret,omitempty"`
	BaseURL      string       `json:"base_url,omitempty"`
	MaxTokens    int          `json:"max_tokens,omitempty"`
	Temperature  *float64     `json:"temperature,omitempty"`
	TimeoutMs    int          `json:"timeout_ms,omitempty"`
}

// LLMCompleteOutput carries the assistant text plus usage metadata.
type LLMCompleteOutput struct {
	Content string         `json:"content"`
	Usage   map[string]any `json:"usage,omitempty"`
	Model   string         `json:"model,omitempty"`
}

// CallLLM POSTs to {baseURL}/chat/completions. Pure (testable) core.
func CallLLM(ctx context.Context, client *http.Client, in LLMCompleteInput) (LLMCompleteOutput, error) {
	var out LLMCompleteOutput
	if len(in.Messages) == 0 {
		return out, worker.NewNonRetryableError(fmt.Errorf("llm.complete: messages is required"))
	}
	keyName := in.APIKeySecret
	if keyName == "" {
		keyName = "LLM_API_KEY"
	}
	apiKey, err := secret.Get(keyName)
	if err != nil {
		return out, fmt.Errorf("llm.complete: %w", err)
	}
	base := strings.TrimRight(in.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	body := map[string]any{"model": in.Model, "messages": in.Messages}
	if in.MaxTokens > 0 {
		body["max_tokens"] = in.MaxTokens
	}
	if in.Temperature != nil {
		body["temperature"] = *in.Temperature
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return out, fmt.Errorf("llm.complete: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return out, fmt.Errorf("llm.complete: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return out, fmt.Errorf("llm.complete: post: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // close error after full read is unactionable
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return out, fmt.Errorf("llm.complete: read body: %w", err)
	}
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return out, fmt.Errorf("llm.complete: provider %d (retryable)", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return out, worker.NewNonRetryableError(fmt.Errorf("llm.complete: provider %d: %s", resp.StatusCode, truncate(string(raw), 300)))
	}
	var parsed struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return out, fmt.Errorf("llm.complete: invalid provider JSON: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return out, fmt.Errorf("llm.complete: provider returned no choices")
	}
	out.Content = parsed.Choices[0].Message.Content
	out.Usage = parsed.Usage
	out.Model = parsed.Model
	return out, nil
}

// resolveLLM fills static recipe fields from step metadata.
func resolveLLM(in LLMCompleteInput, md map[string]string) LLMCompleteInput {
	if in.Model == "" {
		in.Model = md["model"]
	}
	if in.APIKeySecret == "" {
		in.APIKeySecret = md["api_key_secret"]
	}
	if in.BaseURL == "" {
		in.BaseURL = md["base_url"]
	}
	return in
}

func handleLLMComplete(ctx worker.TaskContext) error {
	var in LLMCompleteInput
	if err := decodeInput(ctx.Input(), &in); err != nil {
		return worker.NewNonRetryableError(err)
	}
	in = resolveLLM(in, metadataOf(ctx.Metadata()))
	if in.Model == "" {
		return worker.NewNonRetryableError(fmt.Errorf("llm.complete: model is required"))
	}
	out, err := CallLLM(ctx.Context(), httpClientFor(firstPositive(in.TimeoutMs, 120000)), in)
	if err != nil {
		return err
	}
	return ctx.Complete(marshalOutput(out))
}

func firstPositive(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}
