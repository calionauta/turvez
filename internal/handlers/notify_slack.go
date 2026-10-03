package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/danmestas/dagnats/worker"
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
	if _, err := PostWebhookJSON(ctx, client, secretName, map[string]string{"text": message}); err != nil {
		return fmt.Errorf("notify.slack: %w", err)
	}
	return nil
}

// decodeNotify accepts {"webhook_secret","message"}, a bare JSON string
// (message text, secret name from metadata), or empty input
// (metadata-only — the on_failure-safe shape).
func decodeNotify(raw []byte, md map[string]string) (NotifySlackInput, error) {
	var in NotifySlackInput
	if len(bytes.TrimSpace(raw)) == 0 {
		// metadata-only; validated below.
	} else if isJSONObject(raw) {
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
