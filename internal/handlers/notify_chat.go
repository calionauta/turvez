package handlers

import (
	"fmt"

	"github.com/danmestas/dagnats/worker"
)

// NotifyTelegramInput is the task payload for the "notify.telegram"
// handler. webhook_secret holds the full Bot API sendMessage URL
// (https://api.telegram.org/bot<token>/sendMessage) — the token stays
// in env, never in JSON. chat_id rides in input or metadata.
type NotifyTelegramInput struct {
	WebhookSecret string `json:"webhook_secret"`
	Message       string `json:"message"`
	ChatID        string `json:"chat_id,omitempty"`
}

// NotifyDiscordInput is the task payload for the "notify.discord"
// handler. webhook_secret holds the Discord webhook URL.
type NotifyDiscordInput struct {
	WebhookSecret string `json:"webhook_secret"`
	Message       string `json:"message"`
}

func decodeTelegram(raw []byte, md map[string]string) (NotifyTelegramInput, error) {
	var in NotifyTelegramInput
	if len(raw) == 0 {
		return in, fmt.Errorf("handlers: empty task input")
	}
	if isJSONObject(raw) {
		if err := decodeInput(raw, &in); err != nil {
			return in, err
		}
	} else {
		var s string
		if err := decodeDataInto(raw, &s); err != nil {
			return in, err
		}
		in.Message = s
	}
	if in.WebhookSecret == "" {
		in.WebhookSecret = md["webhook_secret"]
	}
	if in.Message == "" {
		in.Message = md["message"]
	}
	if in.ChatID == "" {
		in.ChatID = md["chat_id"]
	}
	return in, nil
}

func decodeDiscord(raw []byte, md map[string]string) (NotifyDiscordInput, error) {
	var in NotifyDiscordInput
	if len(raw) == 0 {
		return in, fmt.Errorf("handlers: empty task input")
	}
	if isJSONObject(raw) {
		if err := decodeInput(raw, &in); err != nil {
			return in, err
		}
	} else {
		var s string
		if err := decodeDataInto(raw, &s); err != nil {
			return in, err
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

func handleNotifyTelegram(ctx worker.TaskContext) error {
	in, err := decodeTelegram(ctx.Input(), metadataOf(ctx.Metadata()))
	if err != nil {
		return worker.NewNonRetryableError(err)
	}
	if in.WebhookSecret == "" {
		return worker.NewNonRetryableError(fmt.Errorf("notify.telegram: webhook_secret is required"))
	}
	if in.Message == "" {
		return worker.NewNonRetryableError(fmt.Errorf("notify.telegram: message is required"))
	}
	payload := map[string]any{"text": in.Message}
	if in.ChatID != "" {
		payload["chat_id"] = in.ChatID
	}
	if _, err := PostWebhookJSON(ctx.Context(), httpClientFor(10000), in.WebhookSecret, payload); err != nil {
		return fmt.Errorf("notify.telegram: %w", err)
	}
	return ctx.Complete(marshalOutput(map[string]bool{"ok": true}))
}

func handleNotifyDiscord(ctx worker.TaskContext) error {
	in, err := decodeDiscord(ctx.Input(), metadataOf(ctx.Metadata()))
	if err != nil {
		return worker.NewNonRetryableError(err)
	}
	if in.WebhookSecret == "" {
		return worker.NewNonRetryableError(fmt.Errorf("notify.discord: webhook_secret is required"))
	}
	if in.Message == "" {
		return worker.NewNonRetryableError(fmt.Errorf("notify.discord: message is required"))
	}
	if _, err := PostWebhookJSON(ctx.Context(), httpClientFor(10000), in.WebhookSecret, map[string]string{"content": in.Message}); err != nil {
		return fmt.Errorf("notify.discord: %w", err)
	}
	return ctx.Complete(marshalOutput(map[string]bool{"ok": true}))
}
