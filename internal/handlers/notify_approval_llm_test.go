package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
