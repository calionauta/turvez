package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/danmestas/dagnats/worker"
	"github.com/solo-operator/dagnats-automation-template/internal/secret"
)

// NotifySlackInput is the task payload for the "notify.slack" handler.
type NotifySlackInput struct {
	WebhookSecret string `json:"webhook_secret"`
	Message       string `json:"message"`
}

// NotifySlackOutput confirms delivery.
type NotifySlackOutput struct {
	OK bool `json:"ok"`
}

// PostSlackMessage POSTs text to the webhook URL held in secretName.
// client is injected for tests; production passes httpClientFor(10000).
func PostSlackMessage(ctx context.Context, client *http.Client, secretName, message string) error {
	if message == "" {
		return worker.NewNonRetryableError(fmt.Errorf("notify.slack: message is required"))
	}
	webhookURL, err := secret.Get(secretName)
	if err != nil {
		return fmt.Errorf("notify.slack: %w", err)
	}
	payload, err := json.Marshal(map[string]string{"text": message})
	if err != nil {
		return fmt.Errorf("notify.slack: encode message: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("notify.slack: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("notify.slack: post: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // close error after drain is unactionable
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)); err != nil {
		return fmt.Errorf("notify.slack: drain body: %w", err)
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("notify.slack: slack %d (retryable)", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return worker.NewNonRetryableError(fmt.Errorf("notify.slack: slack %d", resp.StatusCode))
	}
	return nil
}

// decodeNotify accepts {"webhook_secret","message"} or a bare JSON string
// (message text, secret name from metadata).
func decodeNotify(raw []byte, md map[string]string) (NotifySlackInput, error) {
	var in NotifySlackInput
	if len(raw) == 0 {
		return in, fmt.Errorf("handlers: empty task input")
	}
	if isJSONObject(raw) {
		if err := json.Unmarshal(raw, &in); err != nil {
			return in, fmt.Errorf("handlers: invalid notify.slack input: %w", err)
		}
	} else {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return in, fmt.Errorf("handlers: invalid notify.slack input: %w", err)
		}
		in.Message = s
	}
	if in.WebhookSecret == "" {
		in.WebhookSecret = md["webhook_secret"]
	}
	if in.Message == "" {
		in.Message = md["message"]
	}
	return in, nil
}

func handleNotifySlack(ctx worker.TaskContext) error {
	in, err := decodeNotify(ctx.Input(), metadataOf(ctx.Metadata()))
	if err != nil {
		return worker.NewNonRetryableError(err)
	}
	if in.WebhookSecret == "" {
		return worker.NewNonRetryableError(fmt.Errorf("notify.slack: webhook_secret is required"))
	}
	if err := PostSlackMessage(ctx.Context(), httpClientFor(10000), in.WebhookSecret, in.Message); err != nil {
		return err
	}
	return ctx.Complete(marshalOutput(NotifySlackOutput{OK: true}))
}
