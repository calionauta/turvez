package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/danmestas/dagnats/worker"
	"github.com/solo-operator/dagnats-automation-template/internal/secret"
)

// HTTPRequestInput is the task payload for the "http.request" handler.
// Secrets are referenced by NAME only — values never appear in workflow JSON.
type HTTPRequestInput struct {
	Method       string            `json:"method"`
	URL          string            `json:"url"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         json.RawMessage   `json:"body,omitempty"`
	TimeoutMs    int               `json:"timeout_ms,omitempty"`
	AuthSecret   string            `json:"auth_secret,omitempty"`
	AuthHeader   string            `json:"auth_header,omitempty"`
	AuthScheme   string            `json:"auth_scheme,omitempty"`
	ResponseJSON bool              `json:"response_json,omitempty"`
}

// HTTPRequestOutput is the structured result passed downstream.
type HTTPRequestOutput struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	JSON    any               `json:"json,omitempty"`
}

// httpClientFor builds a client honoring the per-task timeout.
func httpClientFor(timeoutMs int) *http.Client {
	if timeoutMs <= 0 {
		timeoutMs = 15000
	}
	if timeoutMs > 120000 {
		timeoutMs = 120000
	}
	return &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
}

// DoHTTPRequest executes in with client. Pure (testable) core of http.request.
func DoHTTPRequest(ctx context.Context, in HTTPRequestInput, client *http.Client) (HTTPRequestOutput, error) {
	var out HTTPRequestOutput
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = "GET"
	}
	if in.URL == "" {
		return out, fmt.Errorf("http.request: url is required")
	}
	var bodyReader io.Reader
	if len(in.Body) > 0 {
		bodyReader = bytes.NewReader(in.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, in.URL, bodyReader)
	if err != nil {
		return out, fmt.Errorf("http.request: build request: %w", err)
	}
	for k, v := range in.Headers {
		req.Header.Set(k, v)
	}
	if len(in.Body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// Auth header comes from a secret NAME in the payload; the value is
	// resolved here, at runtime, and never logged or echoed.
	if in.AuthSecret != "" {
		token, err := secret.Get(in.AuthSecret)
		if err != nil {
			return out, fmt.Errorf("http.request: %w", err)
		}
		header := in.AuthHeader
		if header == "" {
			header = "Authorization"
		}
		value := token
		if scheme := strings.TrimSpace(in.AuthScheme); scheme != "" {
			value = scheme + " " + token
		} else if strings.EqualFold(header, "Authorization") {
			value = "Bearer " + token
		}
		req.Header.Set(header, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return out, fmt.Errorf("http.request: %s %s: %w", method, in.URL, err)
	}
	defer resp.Body.Close() //nolint:errcheck // close error after full read is unactionable
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return out, fmt.Errorf("http.request: read body: %w", err)
	}
	out.Status = resp.StatusCode
	out.Headers = map[string]string{}
	for k := range resp.Header {
		out.Headers[k] = resp.Header.Get(k)
	}
	out.Body = string(raw)
	if in.ResponseJSON && len(raw) > 0 {
		var parsed any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return out, fmt.Errorf("http.request: response is not valid JSON: %w", err)
		}
		out.JSON = parsed
	}
	if resp.StatusCode >= 500 {
		return out, fmt.Errorf("http.request: upstream %d (retryable)", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return out, worker.NewNonRetryableError(fmt.Errorf("http.request: upstream %d: %s", resp.StatusCode, truncate(out.Body, 500)))
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// resolveHTTP fills static recipe fields (method, url, secret names) from
// step metadata; task input wins on conflict.
func resolveHTTP(in HTTPRequestInput, md map[string]string) HTTPRequestInput {
	if in.Method == "" {
		in.Method = md["method"]
	}
	if in.URL == "" {
		in.URL = md["url"]
	}
	if in.AuthSecret == "" {
		in.AuthSecret = md["auth_secret"]
	}
	if in.AuthHeader == "" {
		in.AuthHeader = md["auth_header"]
	}
	if in.AuthScheme == "" {
		in.AuthScheme = md["auth_scheme"]
	}
	if !in.ResponseJSON {
		in.ResponseJSON = boolFromMeta(md, "response_json")
	}
	if in.TimeoutMs == 0 {
		in.TimeoutMs = intFromMeta(md, "timeout_ms")
	}
	return in
}

func handleHTTPRequest(ctx worker.TaskContext) error {
	var in HTTPRequestInput
	if err := decodeInput(ctx.Input(), &in); err != nil {
		return worker.NewNonRetryableError(err)
	}
	in = resolveHTTP(in, metadataOf(ctx.Metadata()))
	out, err := DoHTTPRequest(ctx.Context(), in, httpClientFor(in.TimeoutMs))
	if err != nil {
		return err
	}
	return ctx.Complete(marshalOutput(out))
}
