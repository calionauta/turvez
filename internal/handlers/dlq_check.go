package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/calionauta/turvez/internal/secret"
	"github.com/danmestas/dagnats/worker"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// DLQCheckInput is the task payload for the "dlq.check" handler: read the
// dead-letter stream depth so a cron watchdog recipe can page the operator.
// There is no dagnats HTTP route for the DLQ (CLI/NATS only), hence a small
// dedicated handler instead of http.request.
type DLQCheckInput struct {
	Stream    string `json:"stream,omitempty"`
	Threshold int    `json:"threshold,omitempty"`
}

// DLQCheckOutput carries depth plus the alert verdict downstream.
type DLQCheckOutput struct {
	Stream    string `json:"stream"`
	Depth     uint64 `json:"depth"`
	Threshold int    `json:"threshold"`
	Alert     bool   `json:"alert"`
}

// dlqDepthFunc is the injectable depth reader; production dials NATS.
var dlqDepthFunc = defaultDLQDepth

// defaultDLQDepth opens a short-lived connection (a watchdog fires every
// few minutes; no pooled client to manage) and returns State.Msgs.
func defaultDLQDepth(ctx context.Context, natsURL, stream string) (uint64, error) {
	nc, err := nats.Connect(natsURL, nats.Timeout(5*time.Second))
	if err != nil {
		return 0, fmt.Errorf("dlq.check: connect NATS: %w", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		return 0, fmt.Errorf("dlq.check: jetstream: %w", err)
	}
	st, err := js.Stream(ctx, stream)
	if err != nil {
		return 0, fmt.Errorf("dlq.check: stream %s: %w", stream, err)
	}
	si, err := st.Info(ctx)
	if err != nil {
		return 0, fmt.Errorf("dlq.check: stream %s info: %w", stream, err)
	}
	return si.State.Msgs, nil
}

// CheckDLQ resolves input defaults and returns the depth verdict.
func CheckDLQ(ctx context.Context, in DLQCheckInput, md map[string]string) (DLQCheckOutput, error) {
	stream := in.Stream
	if stream == "" {
		stream = md["stream"]
	}
	if stream == "" {
		stream = "DEAD_LETTERS"
	}
	threshold := in.Threshold
	if threshold == 0 {
		threshold = intFromMeta(md, "threshold")
	}
	if threshold < 0 {
		threshold = 0
	}
	natsURL := secret.Optional("DAGNATS_NATS_URL", "")
	if natsURL == "" {
		natsURL = secret.Optional("NATS_URL", nats.DefaultURL)
	}
	depth, err := dlqDepthFunc(ctx, natsURL, stream)
	if err != nil {
		return DLQCheckOutput{}, err
	}
	return DLQCheckOutput{
		Stream:    stream,
		Depth:     depth,
		Threshold: threshold,
		Alert:     depth > uint64(threshold),
	}, nil
}

func handleDLQCheck(ctx worker.TaskContext) error {
	var in DLQCheckInput
	if len(ctx.Input()) > 0 {
		if err := decodeInput(ctx.Input(), &in); err != nil {
			return worker.NewNonRetryableError(err)
		}
	}
	out, err := CheckDLQ(ctx.Context(), in, metadataOf(ctx.Metadata()))
	if err != nil {
		return err
	}
	return ctx.Complete(marshalOutput(out))
}
