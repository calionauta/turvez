package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostWebhookJSON(t *testing.T) {
	t.Setenv("SECRET_TEST_HOOK_URL", "http://example.invalid/hook")
	var gotBody string
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.WriteHeader(200)
	}))
	defer srv.Close()
	t.Setenv("SECRET_TEST_HOOK_REAL", srv.URL)

	status, err := PostWebhookJSON(context.Background(), srv.Client(), "SECRET_TEST_HOOK_REAL", map[string]any{"text": "hi"})
	if err != nil || status != 200 {
		t.Fatalf("got %d, %v", status, err)
	}
	if gotCT != "application/json" || !strings.Contains(gotBody, `"hi"`) {
		t.Fatalf("ct=%q body=%q", gotCT, gotBody)
	}
	if _, err := PostWebhookJSON(context.Background(), srv.Client(), "SECRET_TEST_HOOK_MISSING", nil); err == nil {
		t.Fatal("expected error for missing secret")
	}
	if _, err := PostWebhookJSON(context.Background(), srv.Client(), "", nil); err == nil {
		t.Fatal("expected error for empty secret name")
	}
}

func TestDecodeWebhookForms(t *testing.T) {
	// Envelope.
	in, err := decodeWebhook([]byte(`{"webhook_secret":"S","payload":{"a":1}}`), nil)
	if err != nil || in.WebhookSecret != "S" {
		t.Fatalf("got %+v, %v", in, err)
	}
	// Raw data becomes the payload; secret from metadata.
	in, err = decodeWebhook([]byte(`{"failed_step":"fetch","error":"boom"}`), map[string]string{"webhook_secret": "S"})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := in.Payload.(map[string]any)
	if !ok || m["failed_step"] != "fetch" || in.WebhookSecret != "S" {
		t.Fatalf("got %+v", in)
	}
	// Empty input: metadata-only (on_failure-safe shape).
	in, err = decodeWebhook([]byte{}, map[string]string{"webhook_secret": "S", "message": "m"})
	if err != nil || in.Message != "m" {
		t.Fatalf("got %+v, %v", in, err)
	}
}

func TestBuildWebhookPayloadEnrichment(t *testing.T) {
	got := buildWebhookPayload(
		NotifyWebhookInput{Payload: map[string]any{"failed_step": "f"}},
		map[string]string{"client": "acme", "workflow": "acme/x"},
		"run-1", "step-2",
	)
	m := got.(map[string]any)
	for k, want := range map[string]string{"client": "acme", "workflow": "acme/x", "run_id": "run-1", "step_id": "step-2"} {
		if m[k] != want {
			t.Fatalf("%s = %v, want %s (payload %#v)", k, m[k], want, m)
		}
	}
	// Message shorthand + non-map payload wrap.
	got2 := buildWebhookPayload(NotifyWebhookInput{Message: "hello"}, nil, "r", "s")
	raw, _ := json.Marshal(got2)
	if !strings.Contains(string(raw), "hello") {
		t.Fatalf("message not wrapped: %s", raw)
	}
}

func TestChatWrappers(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		got = string(buf[:n])
		w.WriteHeader(200)
	}))
	defer srv.Close()
	t.Setenv("SECRET_TEST_CHAT_HOOK", srv.URL)

	in, err := decodeTelegram([]byte(`"plain"`), map[string]string{"webhook_secret": "SECRET_TEST_CHAT_HOOK"})
	if err != nil || in.Message != "plain" {
		t.Fatalf("got %+v, %v", in, err)
	}
	inD, err := decodeDiscord([]byte(`{"webhook_secret":"X"}`), map[string]string{"message": "fallback"})
	if err != nil || inD.Message != "fallback" {
		t.Fatalf("got %+v, %v", inD, err)
	}
	payload := map[string]any{"text": "x"}
	if _, err := PostWebhookJSON(context.Background(), srv.Client(), "SECRET_TEST_CHAT_HOOK", payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"x"`) {
		t.Fatalf("body = %q", got)
	}
}
