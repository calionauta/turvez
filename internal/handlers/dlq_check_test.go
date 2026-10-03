package handlers

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCheckDLQ(t *testing.T) {
	old := dlqDepthFunc
	defer func() { dlqDepthFunc = old }()
	dlqDepthFunc = func(ctx context.Context, natsURL, stream string) (uint64, error) {
		if stream != "DEAD_LETTERS" {
			t.Fatalf("stream = %q", stream)
		}
		return 3, nil
	}
	out, err := CheckDLQ(context.Background(), DLQCheckInput{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Depth != 3 || !out.Alert || out.Stream != "DEAD_LETTERS" {
		t.Fatalf("got %+v", out)
	}

	// Threshold above depth: no alert.
	out, err = CheckDLQ(context.Background(), DLQCheckInput{Threshold: 10}, nil)
	if err != nil || out.Alert {
		t.Fatalf("got %+v, %v", out, err)
	}

	// Backend error propagates (retryable by the engine).
	dlqDepthFunc = func(ctx context.Context, natsURL, stream string) (uint64, error) {
		return 0, errors.New("nats down")
	}
	if _, err := CheckDLQ(context.Background(), DLQCheckInput{}, nil); err == nil {
		t.Fatal("expected backend error")
	}
	_ = time.Second
}
