// Package opshttp is the optional human-approval shim.
//
// A human clicks approve/reject (Slack button, email link, curl). This
// server validates a shared Bearer token and forwards the vote to dagnats.
// No workflow logic lives here — this is a thin authenticated forwarder.
//
// Two flows are supported:
//
//  1. Signal-based (pairs with the request.approval worker handler,
//     which blocks in WaitForSignal):
//
//     POST /ops/approvals/{runID}  {"approved": true}
//     → POST {DAGNATS_API_URL}/runs/{runID}/signal/approval {"approved":true}
//
//  2. Native approval step (pairs with `type: approval` steps, which the
//     engine gates with a per-step token for up to 7 days):
//
//     POST /ops/approvals/{runID}/{stepID}?action=approve|reject&token=…
//     → POST {DAGNATS_API_URL}/runs/{runID}/approval/{stepID}?action=…&token=…
package opshttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Server forwards approval votes to the dagnats HTTP API.
type Server struct {
	// Token is the shared secret from OPS_APPROVAL_TOKEN.
	Token string
	// DagnatsAPI is e.g. http://127.0.0.1:8080 (DAGNATS_API_URL).
	DagnatsAPI string
	// Client is injectable for tests.
	Client *http.Client
}

// Handler returns the mux. Mount it or run it standalone.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ops/approvals/", s.handleApproval)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok")) //nolint:errcheck // health probe body is best-effort
	})
	return mux
}

func (s *Server) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return http.DefaultClient
}

func (s *Server) handleApproval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !subtleCompare("Bearer "+s.Token, r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/ops/approvals/")
	parts := strings.Split(rest, "/")
	switch {
	case len(parts) == 1 && parts[0] != "":
		s.handleSignalApproval(w, r, parts[0])
	case len(parts) == 2 && parts[0] != "" && parts[1] != "":
		s.handleNativeApproval(w, r, parts[0], parts[1])
	default:
		http.Error(w, "want /ops/approvals/{runID} or /ops/approvals/{runID}/{stepID}", http.StatusBadRequest)
	}
}

// handleSignalApproval forwards {"approved":bool} as a run signal.
func (s *Server) handleSignalApproval(w http.ResponseWriter, r *http.Request, runID string) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil || len(raw) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}
	var vote struct {
		Approved *bool `json:"approved"`
	}
	if err := json.Unmarshal(raw, &vote); err != nil || vote.Approved == nil {
		http.Error(w, `body must be {"approved":bool}`, http.StatusBadRequest)
		return
	}
	url := strings.TrimRight(s.DagnatsAPI, "/") + "/runs/" + runID + "/signal/approval"
	s.forward(w, r, url, raw)
}

// handleNativeApproval forwards to the engine's tokenized approval endpoint.
// The engine token arrives in the approval.requested event (NATS subject)
// and is passed back here as ?token=…; action is approve|reject.
func (s *Server) handleNativeApproval(w http.ResponseWriter, r *http.Request, runID, stepID string) {
	action := r.URL.Query().Get("action")
	token := r.URL.Query().Get("token")
	if action != "approve" && action != "reject" {
		http.Error(w, "query needs action=approve|reject", http.StatusBadRequest)
		return
	}
	if token == "" {
		http.Error(w, "query needs token=… (from the approval.requested event)", http.StatusBadRequest)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	url := strings.TrimRight(s.DagnatsAPI, "/") + "/runs/" + runID + "/approval/" + stepID +
		"?action=" + action + "&token=" + token
	s.forward(w, r, url, raw)
}

func (s *Server) forward(w http.ResponseWriter, r *http.Request, url string, body []byte) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "build forward", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client().Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("signal dagnats: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close() //nolint:errcheck // close error after full read is unactionable
	out, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		http.Error(w, "read upstream body", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(out) //nolint:errcheck // partial write to client is unactionable here
}

// subtleCompare avoids short-circuit length leaks in a two-line helper.
func subtleCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
