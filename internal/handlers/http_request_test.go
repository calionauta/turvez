package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoHTTPRequest(t *testing.T) {
	t.Setenv("SECRET_TEST_API_TOKEN", "tok-123")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			if r.Header.Get("Authorization") != "Bearer tok-123" {
				w.WriteHeader(401)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"lead":{"id":"1"}}`))
		case "/boom":
			w.WriteHeader(500)
		case "/nope":
			w.WriteHeader(404)
			_, _ = w.Write([]byte("missing"))
		}
	}))
	defer srv.Close()

	tests := []struct {
		name       string
		in         HTTPRequestInput
		wantStatus int
		wantErr    string
		isRetry    bool // retryable (plain error) vs permanent
	}{
		{name: "get with bearer secret", in: HTTPRequestInput{Method: "GET", URL: srv.URL + "/ok", AuthSecret: "SECRET_TEST_API_TOKEN", ResponseJSON: true}, wantStatus: 200},
		{name: "missing url", in: HTTPRequestInput{}, wantErr: "url is required"},
		{name: "missing secret", in: HTTPRequestInput{Method: "GET", URL: srv.URL + "/ok", AuthSecret: "SECRET_TEST_MISSING"}, wantErr: "not set"},
		{name: "server error retryable", in: HTTPRequestInput{Method: "GET", URL: srv.URL + "/boom"}, wantErr: "retryable", isRetry: true},
		{name: "client error permanent", in: HTTPRequestInput{Method: "GET", URL: srv.URL + "/nope"}, wantErr: "404"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := DoHTTPRequest(context.Background(), tc.in, srv.Client())
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if out.Status != tc.wantStatus {
					t.Fatalf("status = %d, want %d", out.Status, tc.wantStatus)
				}
				if tc.in.ResponseJSON && out.JSON == nil {
					t.Fatal("expected parsed JSON body")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestDoHTTPRequestPostsBody(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := json.Marshal(r.Header.Get("Content-Type"))
		_ = raw
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	in := HTTPRequestInput{Method: "POST", URL: srv.URL, Body: json.RawMessage(`{"a":1}`)}
	out, err := DoHTTPRequest(context.Background(), in, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != 200 || !strings.Contains(gotBody, `"a":1`) {
		t.Fatalf("body not forwarded: %q status %d", gotBody, out.Status)
	}
}
