package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/danmestas/dagnats/worker"
)

// RequestApprovalInput is the task payload for the "request.approval"
// handler: notify a human, then block on a run signal until approved,
// rejected, or timed out.
//
// The signal arrives via `dagnats run signal <runID> approval
// '{"approved":true}'` (or the ops HTTP shim in internal/opshttp).
// For engine-native gates with multi-day timeouts and tokenized links,
// prefer a `type: approval` step instead (see docs/writing-recipes.md).
type RequestApprovalInput struct {
	Signal  string          `json:"signal,omitempty"`
	Timeout string          `json:"timeout,omitempty"`
	Message string          `json:"message,omitempty"`
	Notify  *ApprovalNotify `json:"notify,omitempty"`
}

// ApprovalNotify optionally pings Slack before waiting.
type ApprovalNotify struct {
	WebhookSecret string `json:"webhook_secret"`
	Message       string `json:"message"`
}

// RequestApprovalOutput is the vote plus the raw signal payload.
type RequestApprovalOutput struct {
	Approved bool            `json:"approved"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// signalWaiter is the subset of worker.TaskContext used here so tests can
// inject a fake without NATS.
type signalWaiter interface {
	WaitForSignal(name string, timeout time.Duration) ([]byte, error)
	Context() context.Context
}

// maxSignalChunk is the engine's WaitForSignal ceiling (must be in (0,1h]).
// Longer approvals loop in chunks until the total timeout elapses.
const maxSignalChunk = 55 * time.Minute

// WaitForApproval blocks in ≤55m chunks until signal or timeout, then
// interprets the payload as {"approved": bool}.
func WaitForApproval(w signalWaiter, signalName string, total time.Duration) (RequestApprovalOutput, error) {
	if signalName == "" {
		signalName = "approval"
	}
	deadline := time.Now().Add(total)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return RequestApprovalOutput{}, fmt.Errorf("request.approval: timed out waiting for %q signal after %v", signalName, total)
		}
		chunk := remaining
		if chunk > maxSignalChunk {
			chunk = maxSignalChunk
		}
		data, err := w.WaitForSignal(signalName, chunk)
		if err != nil {
			// Timeout chunk with time left: keep waiting. Context
			// cancellation aborts via ctx below.
			if w.Context().Err() != nil {
				return RequestApprovalOutput{}, w.Context().Err()
			}
			if time.Now().Before(deadline) && isTimeoutErr(err) {
				continue
			}
			return RequestApprovalOutput{}, fmt.Errorf("request.approval: signal wait: %w", err)
		}
		var vote struct {
			Approved *bool           `json:"approved"`
			Rejected *bool           `json:"rejected"`
			Raw      json.RawMessage `json:"-"`
		}
		raw := json.RawMessage(append([]byte(nil), data...))
		vote.Raw = raw
		if err := json.Unmarshal(data, &vote); err != nil {
			return RequestApprovalOutput{}, worker.NewNonRetryableError(fmt.Errorf("request.approval: signal payload is not JSON: %w", err))
		}
		approved := false
		switch {
		case vote.Approved != nil:
			approved = *vote.Approved
		case vote.Rejected != nil:
			approved = !*vote.Rejected
		default:
			return RequestApprovalOutput{}, worker.NewNonRetryableError(fmt.Errorf("request.approval: signal needs {\"approved\":bool}"))
		}
		return RequestApprovalOutput{Approved: approved, Data: raw}, nil
	}
}

// isTimeoutErr treats any error as a chunk timeout except explicit
// cancellation. The engine surfaces waits that way; fail-open would
// approve without a human, so only context errors abort.
func isTimeoutErr(err error) bool {
	return err != nil && !isContextErr(err)
}

func isContextErr(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

// decodeApproval accepts an empty payload (all defaults/metadata), the
// {"signal","timeout","notify"} envelope, or metadata-only configuration.
func decodeApproval(raw []byte, md map[string]string) RequestApprovalInput {
	var in RequestApprovalInput
	if len(bytes.TrimSpace(raw)) > 0 && isJSONObject(raw) {
		_ = json.Unmarshal(raw, &in)
	}
	if in.Signal == "" {
		in.Signal = md["signal"]
	}
	if in.Timeout == "" {
		in.Timeout = md["timeout"]
	}
	if in.Message == "" {
		in.Message = md["message"]
	}
	return in
}

func handleRequestApproval(ctx worker.TaskContext) error {
	in := decodeApproval(ctx.Input(), metadataOf(ctx.Metadata()))
	total := 72 * time.Hour
	if in.Timeout != "" {
		d, err := time.ParseDuration(in.Timeout)
		if err != nil || d <= 0 {
			return worker.NewNonRetryableError(fmt.Errorf("request.approval: invalid timeout %q", in.Timeout))
		}
		total = d
	}
	if in.Notify != nil && in.Notify.WebhookSecret != "" {
		msg := in.Notify.Message
		if msg == "" {
			msg = in.Message
		}
		if msg == "" {
			msg = "Approval requested for run — reply via ops endpoint."
		}
		// Best effort: a failed ping must not fail the approval gate.
		_ = PostSlackMessage(ctx.Context(), httpClientFor(10000), in.Notify.WebhookSecret, msg)
	}
	out, err := WaitForApproval(ctx, in.Signal, total)
	if err != nil {
		return err
	}
	return ctx.Complete(marshalOutput(out))
}
