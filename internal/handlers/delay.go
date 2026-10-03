package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/danmestas/dagnats/worker"
)

// DelayInput is the task payload for the "delay" handler.
//
// Prefer the engine-native `type: sleep` step for waits: it is durable
// and holds no worker slot. This handler exists for input-driven waits
// inside worker-only flows. It holds one worker slot while sleeping —
// fine for minutes, wrong for days (see docs/writing-recipes.md).
type DelayInput struct {
	Duration string `json:"duration"`
}

// DelayOutput reports how long was actually waited.
type DelayOutput struct {
	WaitedMs int64  `json:"waited_ms"`
	Duration string `json:"duration"`
}

// decodeDelay accepts {"duration":"..."}, a bare JSON string, or metadata.
func decodeDelay(raw []byte, md map[string]string) (DelayInput, error) {
	var in DelayInput
	if len(bytes.TrimSpace(raw)) == 0 {
		in.Duration = md["duration"]
		return in, nil
	}
	if isJSONObject(raw) {
		if err := json.Unmarshal(raw, &in); err != nil {
			return in, fmt.Errorf("handlers: invalid delay input: %w", err)
		}
	} else {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return in, fmt.Errorf("handlers: invalid delay input: %w", err)
		}
		in.Duration = s
	}
	if in.Duration == "" {
		in.Duration = md["duration"]
	}
	return in, nil
}

// ParseDelay validates the duration string. Engine sleep caps are long;
// the worker fallback refuses multi-day waits so a stuck run is loud.
func ParseDelay(raw string) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, worker.NewNonRetryableError(fmt.Errorf("delay: invalid duration %q: %w", raw, err))
	}
	if d <= 0 {
		return 0, worker.NewNonRetryableError(fmt.Errorf("delay: duration must be positive, got %q", raw))
	}
	if d > 4*time.Hour {
		return 0, worker.NewNonRetryableError(fmt.Errorf("delay: %v exceeds 4h worker cap; use a native type:sleep step", d))
	}
	return d, nil
}

func handleDelay(ctx worker.TaskContext) error {
	in, err := decodeDelay(ctx.Input(), metadataOf(ctx.Metadata()))
	if err != nil {
		return worker.NewNonRetryableError(err)
	}
	d, err := ParseDelay(in.Duration)
	if err != nil {
		return err
	}
	start := time.Now()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Context().Done():
			return ctx.Context().Err()
		case <-ticker.C:
			if err := ctx.Heartbeat(); err != nil {
				return err
			}
		case <-timer.C:
			return ctx.Complete(marshalOutput(DelayOutput{
				WaitedMs: time.Since(start).Milliseconds(),
				Duration: in.Duration,
			}))
		}
	}
}
