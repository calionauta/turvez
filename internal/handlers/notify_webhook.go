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

// NotifyWebhookInput is the task payload for the "notify.webhook" handler:
// POST any JSON payload to a URL held in a secret. The generic operator
// path — Slack/Telegram/Discord wrappers all funnel through PostWebhookJSON.
type NotifyWebhookInput struct {
	WebhookSecret string `json:"webhook_secret"`
	Payload       any    `json:"payload,omitempty"`
	Message       string `json:"message,omitempty"`
}

// NotifyWebhookOutput confirms delivery with run context attached.
type NotifyWebhookOutput struct {
	OK     bool   `json:"ok"`
	Status int    `json:"status"`
	RunID  string `json:"run_id"`
}

// PostWebhookJSON POSTs payload as JSON to the URL held in secretName.
// client is injected for tests; production passes httpClientFor(10000).
func PostWebhookJSON(ctx context.Context, client *http.Client, secretName string, payload any) (int, error) {
	if secretName == "" {
		return 0, worker.NewNonRetryableError(fmt.Errorf("notify.webhook: webhook_secret is required"))
	}
	webhookURL, err := secret.Get(secretName)
	if err != nil {
		return 0, fmt.Errorf("notify.webhook: %w", err)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, worker.NewNonRetryableError(fmt.Errorf("notify.webhook: encode payload: %w", err))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("notify.webhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("notify.webhook: post: %w", err)
	}
	defer resp.Body.Close()                                       //nolint:errcheck // close error after drain is unactionable
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)) //nolint:errcheck // best-effort drain
	if resp.StatusCode >= 500 {
		return resp.StatusCode, fmt.Errorf("notify.webhook: webhook %d (retryable)", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, worker.NewNonRetryableError(fmt.Errorf("notify.webhook: webhook %d", resp.StatusCode))
	}
	return resp.StatusCode, nil
}

// decodeWebhook accepts the {"webhook_secret","payload","message"} envelope,
// raw data (whole payload, secret from metadata), or empty input
// (metadata-only — the on_failure-safe shape).
func decodeWebhook(raw []byte, md map[string]string) (NotifyWebhookInput, error) {
	var in NotifyWebhookInput
	if len(bytes.TrimSpace(raw)) == 0 {
		// metadata-only; validated below.
	} else if isJSONObject(raw) && hasAnyKey(raw, "webhook_secret", "payload", "message") {
		if err := json.Unmarshal(raw, &in); err != nil {
			return in, fmt.Errorf("handlers: invalid notify.webhook input: %w", err)
		}
	} else {
		data, err := decodeData(raw)
		if err != nil {
			return in, err
		}
		in.Payload = data
	}
	if in.WebhookSecret == "" {
		in.WebhookSecret = md["webhook_secret"]
	}
	if in.Message == "" {
		in.Message = md["message"]
	}
	return in, nil
}

// buildWebhookPayload merges message/client/workflow metadata and run
// context into the payload. Map payloads are enriched in place; anything
// else is wrapped as {"data": …}.
func buildWebhookPayload(in NotifyWebhookInput, md map[string]string, runID, stepID string) any {
	var payload map[string]any
	switch p := in.Payload.(type) {
	case map[string]any:
		payload = p
	case nil:
		payload = map[string]any{}
	default:
		payload = map[string]any{"data": p}
	}
	if in.Message != "" {
		if _, ok := payload["text"]; !ok {
			payload["text"] = in.Message
		}
	}
	if _, ok := payload["client"]; !ok && md["client"] != "" {
		payload["client"] = md["client"]
	}
	if _, ok := payload["workflow"]; !ok && md["workflow"] != "" {
		payload["workflow"] = md["workflow"]
	}
	payload["run_id"] = runID
	payload["step_id"] = stepID
	return payload
}

func handleNotifyWebhook(ctx worker.TaskContext) error {
	in, err := decodeWebhook(ctx.Input(), metadataOf(ctx.Metadata()))
	if err != nil {
		return worker.NewNonRetryableError(err)
	}
	status, err := PostWebhookJSON(ctx.Context(), httpClientFor(10000), in.WebhookSecret,
		buildWebhookPayload(in, metadataOf(ctx.Metadata()), ctx.RunID(), ctx.StepID()))
	if err != nil {
		return err
	}
	return ctx.Complete(marshalOutput(NotifyWebhookOutput{OK: true, Status: status, RunID: ctx.RunID()}))
}
