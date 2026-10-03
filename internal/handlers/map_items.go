package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/danmestas/dagnats/worker"
	"golang.org/x/sync/errgroup"
)

// MapInput is the task payload for the "map" handler (worker-side fallback).
//
// Prefer the engine-native `type: map` step for fan-out across workers
// (see docs/writing-recipes.md). This handler is for small in-worker
// reshapes: apply one jq expression to each item with a concurrency cap.
type MapInput struct {
	Items       []any  `json:"items"`
	Expression  string `json:"expression,omitempty"`
	Concurrency int    `json:"concurrency,omitempty"`
}

// MapOutput carries per-item results in input order.
type MapOutput struct {
	Results []any `json:"results"`
	Count   int   `json:"count"`
}

// ApplyMap runs expression per item with at most concurrency workers.
// A nil/empty expression passes items through unchanged.
func ApplyMap(ctx context.Context, in MapInput) (MapOutput, error) {
	out := MapOutput{Results: make([]any, len(in.Items)), Count: len(in.Items)}
	if len(in.Items) == 0 {
		out.Results = []any{}
		return out, nil
	}
	if in.Items != nil && len(in.Items) > 10000 {
		return out, worker.NewNonRetryableError(fmt.Errorf("map: %d items exceeds 10000 cap; use native type:map fan-out", len(in.Items)))
	}
	limit := in.Concurrency
	if limit <= 0 {
		limit = 8
	}
	if limit > 32 {
		limit = 32
	}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	for i, item := range in.Items {
		i, item := i, item
		g.Go(func() error {
			select {
			case <-gctx.Done():
				return gctx.Err()
			default:
			}
			if in.Expression == "" {
				out.Results[i] = item
				return nil
			}
			res, err := ApplyJQLight(item, in.Expression)
			if err != nil {
				return fmt.Errorf("map: item %d: %w", i, err)
			}
			out.Results[i] = res
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return MapOutput{}, err
	}
	return out, nil
}

// decodeMap accepts the {"items",...} envelope, a bare JSON array
// (whole payload becomes Items), or a bare null (empty).
func decodeMap(raw []byte, md map[string]string) (MapInput, error) {
	var in MapInput
	if len(raw) == 0 {
		return in, fmt.Errorf("handlers: empty task input")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []any
		if err := json.Unmarshal(raw, &items); err != nil {
			return in, fmt.Errorf("handlers: invalid map array: %w", err)
		}
		in.Items = items
	} else {
		if err := json.Unmarshal(raw, &in); err != nil {
			return in, fmt.Errorf("handlers: invalid map input: %w", err)
		}
		var normalized []any
		round, _ := json.Marshal(in.Items)
		_ = json.Unmarshal(round, &normalized)
		in.Items = normalized
	}
	if in.Expression == "" {
		in.Expression = md["expression"]
	}
	if in.Concurrency == 0 {
		in.Concurrency = intFromMeta(md, "concurrency")
	}
	return in, nil
}

func handleMap(ctx worker.TaskContext) error {
	in, err := decodeMap(ctx.Input(), metadataOf(ctx.Metadata()))
	if err != nil {
		return worker.NewNonRetryableError(err)
	}
	out, err := ApplyMap(ctx.Context(), in)
	if err != nil {
		return err
	}
	return ctx.Complete(marshalOutput(out))
}
