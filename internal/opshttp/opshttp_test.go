package opshttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApprovalForward(t *testing.T) {
	var gotPath string
	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	s := &Server{Token: "sekret", DagnatsAPI: upstream.URL, Client: upstream.Client()}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Unauthorized first.
	resp, err := http.Post(ts.URL+"/ops/approvals/run-1", "application/json", strings.NewReader(`{"approved":true}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/ops/approvals/run-1", strings.NewReader(`{"approved":false}`))
	req.Header.Set("Authorization", "Bearer sekret")
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp2.StatusCode)
	}
	if gotPath != "/runs/run-1/signal/approval" {
		t.Fatalf("forwarded path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"approved":false`) {
		t.Fatalf("forwarded body = %q", gotBody)
	}

	// Bad body rejected before touching upstream.
	req3, _ := http.NewRequest(http.MethodPost, ts.URL+"/ops/approvals/run-1", strings.NewReader(`{}`))
	req3.Header.Set("Authorization", "Bearer sekret")
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", resp3.StatusCode)
	}
}

func TestNativeApprovalForward(t *testing.T) {
	var gotURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.RequestURI()
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"approved"}`))
	}))
	defer upstream.Close()

	s := &Server{Token: "sekret", DagnatsAPI: upstream.URL, Client: upstream.Client()}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/ops/approvals/run-9/step-approve?action=approve&token=tok123", strings.NewReader(`{"by":"ops"}`))
	req.Header.Set("Authorization", "Bearer sekret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if gotURL != "/runs/run-9/approval/step-approve?action=approve&token=tok123" {
		t.Fatalf("forwarded URL = %q", gotURL)
	}

	// Missing action rejected.
	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/ops/approvals/run-9/step-approve?token=tok123", nil)
	req2.Header.Set("Authorization", "Bearer sekret")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", resp2.StatusCode)
	}
}
