package nango

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateListRevoke(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/connect/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connect/sessions" {
			t.Fatal(r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"connect_link":"https://connect.example/x","expires_at":"2026-11-07T15:00:00Z"}}`))
	})
	mux.HandleFunc("/connections", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"connections":[{"connection_id":"c1","provider_config_key":"github","provider":"github","tags":{"turvez_client":"acme"},"created":"now"},{"connection_id":"c2","provider_config_key":"slack","provider":"slack","tags":{"turvez_client":"other"},"created":"now"}]}`))
	})
	mux.HandleFunc("/connections/c1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Query().Get("provider_config_key") != "github" {
			t.Fatalf("bad delete %s %s", r.Method, r.URL.Query().Encode())
		}
		w.WriteHeader(204)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	a := New(srv.URL, "key")

	g, err := a.CreateGrant("acme", "github")
	if err != nil || g.ConnectLink != "https://connect.example/x" {
		t.Fatalf("grant %+v err %v", g, err)
	}
	conns, err := a.ListForTenant("acme")
	if err != nil || len(conns) != 1 || conns[0].ConnectionID != "c1" {
		t.Fatalf("list %+v err %v", conns, err)
	}
	if err := a.Revoke("c1", "github"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
}

func TestErrorDoesNotEchoBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"code":"missing_environment"}}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "key").ListForTenant("acme")
	if err == nil || !strings.Contains(err.Error(), "missing_environment") || strings.Contains(err.Error(), "missing_environment\"") {
		t.Fatalf("unexpected %v", err)
	}
	_ = io.Discard
}
