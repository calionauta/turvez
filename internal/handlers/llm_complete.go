package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/calionauta/turvez/internal/secret"
	"github.com/danmestas/dagnats/worker"
	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
	"github.com/zendev-sh/goai/provider/openai"
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

// CallLLM performs one bounded chat completion through a provider client.
//
// Transport is GoAI (provider quirks, retries, streaming live there); the
// engine keeps ownership of retry/backoff, so MaxRetries is pinned to 0 — two
// retry loops on one 429 multiply attempts and burn the step budget.
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
		// No silent default to api.openai.com: a direct provider call skips the
		// client's gateway, so there is no cost/latency/error log to observe.
		return out, worker.NewNonRetryableError(fmt.Errorf(
			"llm.complete: base_url is required (recipe metadata, or LLM_BASE_URL) — " +
				"point it at the client's OpenAI-compatible gateway, e.g. http://host:8081/v1"))
	}

	model := openai.Chat(in.Model,
		openai.WithAPIKey(apiKey),
		openai.WithBaseURL(base),
		openai.WithHTTPClient(client),
	)
	opts := []goai.Option{
		goai.WithMessages(toProviderMessages(in.Messages)...),
		goai.WithMaxRetries(0),
		// GoAI v0.10 defaults every model to /v1/responses. OpenAI-compatible
		// gateways (Bifrost, LiteLLM, most proxies) are built around
		// /v1/chat/completions, so pin the dialect this handler has always used.
		goai.WithProviderOptions(map[string]any{"useResponsesAPI": false}),
	}
	if in.MaxTokens > 0 {
		opts = append(opts, goai.WithMaxOutputTokens(in.MaxTokens))
	}
	if in.Temperature != nil {
		opts = append(opts, goai.WithTemperature(*in.Temperature))
	}
	if in.TimeoutMs > 0 {
		opts = append(opts, goai.WithTimeout(time.Duration(in.TimeoutMs)*time.Millisecond))
	}

	res, err := goai.GenerateText(ctx, model, opts...)
	if err != nil {
		return out, classifyLLMError(err)
	}
	// An empty completion is never useful here (no tools in this contract) and
	// is the signature of a reasoning model that spent max_tokens thinking.
	// Failing loudly beats persisting a "successful" blank step output.
	if strings.TrimSpace(res.Text) == "" {
		return out, worker.NewNonRetryableError(fmt.Errorf(
			"llm.complete: empty completion (finish=%s, reasoning_tokens=%d) — "+
				"raise max_tokens; reasoning models spend the budget before the answer",
			res.FinishReason, res.TotalUsage.ReasoningTokens))
	}

	out.Content = res.Text
	out.Model = res.Response.Model
	if out.Model == "" {
		out.Model = in.Model
	}
	u := res.TotalUsage
	out.Usage = map[string]any{
		"prompt_tokens":     u.InputTokens,
		"completion_tokens": u.OutputTokens,
		"total_tokens":      u.TotalTokens,
	}
	if u.ReasoningTokens > 0 {
		out.Usage["reasoning_tokens"] = u.ReasoningTokens
	}
	if res.FinishReason != "" {
		out.Usage["finish_reason"] = string(res.FinishReason)
	}
	return out, nil
}

// toProviderMessages maps the recipe's flat messages to GoAI parts.
func toProviderMessages(msgs []LLMMessage) []provider.Message {
	out := make([]provider.Message, 0, len(msgs))
	for _, m := range msgs {
		role := provider.RoleUser
		switch m.Role {
		case "system":
			role = provider.RoleSystem
		case "assistant":
			role = provider.RoleAssistant
		}
		out = append(out, provider.Message{
			Role:    role,
			Content: []provider.Part{{Type: provider.PartText, Text: m.Content}},
		})
	}
	return out
}

// classifyLLMError preserves the handler's retry contract: rate limits and
// 5xx stay retryable, everything else is permanent. GoAI returns a typed
// APIError, so this is a status-code decision instead of string matching.
func classifyLLMError(err error) error {
	var apiErr *goai.APIError
	if errors.As(err, &apiErr) {
		msg := truncate(apiErr.Message, 300)
		if apiErr.StatusCode == 429 || apiErr.StatusCode >= 500 {
			return fmt.Errorf("llm.complete: provider %d (retryable): %s", apiErr.StatusCode, msg)
		}
		return worker.NewNonRetryableError(fmt.Errorf("llm.complete: provider %d: %s", apiErr.StatusCode, msg))
	}
	if goai.IsOverflow(err.Error()) {
		return worker.NewNonRetryableError(fmt.Errorf("llm.complete: context overflow: %s", truncate(err.Error(), 300)))
	}
	return fmt.Errorf("llm.complete: %w", err)
}

// resolveLLM fills static recipe fields from step metadata. base_url falls
// back to LLM_BASE_URL so one gateway can be configured per deployment
// without repeating the URL in every step.
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
	if in.BaseURL == "" {
		in.BaseURL = secret.Optional("LLM_BASE_URL", "")
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
