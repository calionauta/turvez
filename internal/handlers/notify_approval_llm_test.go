package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danmestas/dagnats/worker"
)

func TestPostSlackMessage(t *testing.T) {
	t.Setenv("SECRET_TEST_SLACK_URL", "http://example.invalid/hook")
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		got = string(buf[:n])
		w.WriteHeader(200)
	}))
	defer srv.Close()
	t.Setenv("SECRET_TEST_SLACK_REAL", srv.URL)

	if err := PostSlackMessage(context.Background(), srv.Client(), "SECRET_TEST_SLACK_REAL", "hello ops"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "hello ops") {
		t.Fatalf("message not posted: %q", got)
	}
	if err := PostSlackMessage(context.Background(), srv.Client(), "SECRET_TEST_SLACK_REAL", ""); err == nil {
		t.Fatal("expected error for empty message")
	}
	if err := PostSlackMessage(context.Background(), srv.Client(), "SECRET_TEST_MISSING_HOOK", "x"); err == nil {
		t.Fatal("expected error for missing secret")
	}
}

// stubWaiter fakes the signal wait without NATS.
type stubWaiter struct {
	ctx  context.Context
	data []byte
	err  error
}

func (s stubWaiter) WaitForSignal(string, time.Duration) ([]byte, error) { return s.data, s.err }
func (s stubWaiter) Context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

func TestWaitForApproval(t *testing.T) {
	out, err := WaitForApproval(stubWaiter{data: []byte(`{"approved":true}`)}, "approval", time.Minute)
	if err != nil || !out.Approved {
		t.Fatalf("got %+v, %v", out, err)
	}
	out, err = WaitForApproval(stubWaiter{data: []byte(`{"rejected":true}`)}, "approval", time.Minute)
	if err != nil || out.Approved {
		t.Fatalf("got %+v, %v", out, err)
	}
	if _, err := WaitForApproval(stubWaiter{data: []byte(`{}`)}, "approval", time.Minute); err == nil {
		t.Fatal("expected error for vote-less payload")
	}
	if _, err := WaitForApproval(stubWaiter{err: errors.New("timeout")}, "approval", time.Millisecond); err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestCallLLM(t *testing.T) {
	t.Setenv("SECRET_TEST_LLM_KEY", "sk-test")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"content":"hi"}}],"usage":{"total_tokens":3}}`))
	}))
	defer srv.Close()
	temp := 0.2
	out, err := CallLLM(context.Background(), srv.Client(), LLMCompleteInput{
		Model: "m", Messages: []LLMMessage{{Role: "user", Content: "hi"}},
		APIKeySecret: "SECRET_TEST_LLM_KEY", BaseURL: srv.URL, Temperature: &temp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Content != "hi" {
		t.Fatalf("content = %q", out.Content)
	}
	if _, err := CallLLM(context.Background(), srv.Client(), LLMCompleteInput{}); err == nil {
		t.Fatal("expected error for empty messages")
	}
}

func TestCallLLMRequiresBaseURL(t *testing.T) {
	t.Setenv("LLM_API_KEY", "sk-test")
	_, err := CallLLM(context.Background(), http.DefaultClient, LLMCompleteInput{
		Model: "m", Messages: []LLMMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil || !strings.Contains(err.Error(), "base_url is required") {
		t.Fatalf("err = %v, want base_url is required", err)
	}
}

func TestResolveLLMBaseURLPrecedence(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://gateway.example/v1")

	if got := resolveLLM(LLMCompleteInput{}, nil).BaseURL; got != "http://gateway.example/v1" {
		t.Fatalf("env fallback = %q", got)
	}
	if got := resolveLLM(LLMCompleteInput{}, map[string]string{"base_url": "http://step.example/v1"}).BaseURL; got != "http://step.example/v1" {
		t.Fatalf("metadata must win over env, got %q", got)
	}
	if got := resolveLLM(LLMCompleteInput{BaseURL: "http://input.example/v1"}, nil).BaseURL; got != "http://input.example/v1" {
		t.Fatalf("task input must win over env, got %q", got)
	}
}

func TestCallLLMPathAndAuth(t *testing.T) {
	t.Setenv("SECRET_TEST_LLM_KEY", "sk-test")
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"served-model","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`))
	}))
	defer srv.Close()

	out, err := CallLLM(context.Background(), srv.Client(), LLMCompleteInput{
		Model: "m", Messages: []LLMMessage{{Role: "user", Content: "hi"}},
		APIKeySecret: "SECRET_TEST_LLM_KEY", BaseURL: srv.URL + "/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if out.Model != "served-model" {
		t.Fatalf("model = %q", out.Model)
	}
	if out.Usage["prompt_tokens"] != 7 || out.Usage["completion_tokens"] != 2 || out.Usage["total_tokens"] != 9 {
		t.Fatalf("usage = %#v", out.Usage)
	}
}

func TestCallLLMDoesNotRetryInternally(t *testing.T) {
	t.Setenv("SECRET_TEST_LLM_KEY", "sk-test")
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit"}}`))
	}))
	defer srv.Close()

	_, err := CallLLM(context.Background(), srv.Client(), LLMCompleteInput{
		Model: "m", Messages: []LLMMessage{{Role: "user", Content: "hi"}},
		APIKeySecret: "SECRET_TEST_LLM_KEY", BaseURL: srv.URL,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("http calls = %d, want 1 (retry belongs to the engine)", calls)
	}
	var nonRetryable *worker.NonRetryableError
	if errors.As(err, &nonRetryable) {
		t.Fatalf("429 must stay retryable, got %v", err)
	}
}

func TestCallLLMErrorClassification(t *testing.T) {
	t.Setenv("SECRET_TEST_LLM_KEY", "sk-test")
	cases := []struct {
		status        int
		wantRetryable bool
	}{
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusNotFound, false},
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
		}))
		_, err := CallLLM(context.Background(), srv.Client(), LLMCompleteInput{
			Model: "m", Messages: []LLMMessage{{Role: "user", Content: "hi"}},
			APIKeySecret: "SECRET_TEST_LLM_KEY", BaseURL: srv.URL,
		})
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		var nonRetryable *worker.NonRetryableError
		gotNonRetryable := errors.As(err, &nonRetryable)
		if gotNonRetryable == tc.wantRetryable {
			t.Fatalf("status %d: nonRetryable=%v, want retryable=%v (err=%v)",
				tc.status, gotNonRetryable, tc.wantRetryable, err)
		}
	}
}

func TestCallLLMEmptyCompletionFails(t *testing.T) {
	t.Setenv("SECRET_TEST_LLM_KEY", "sk-test")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":10,"completion_tokens":100,"total_tokens":110,"completion_tokens_details":{"reasoning_tokens":100}}}`))
	}))
	defer srv.Close()

	_, err := CallLLM(context.Background(), srv.Client(), LLMCompleteInput{
		Model: "m", Messages: []LLMMessage{{Role: "user", Content: "hi"}},
		APIKeySecret: "SECRET_TEST_LLM_KEY", BaseURL: srv.URL,
	})
	var nonRetryable *worker.NonRetryableError
	if !errors.As(err, &nonRetryable) {
		t.Fatalf("empty completion must fail non-retryably, got %v", err)
	}
	if !strings.Contains(err.Error(), "empty completion") {
		t.Fatalf("err = %v", err)
	}
}
