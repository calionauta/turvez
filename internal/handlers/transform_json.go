package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/danmestas/dagnats/worker"
	"github.com/itchyny/gojq"
)

// TransformInput is the task payload for the "transform.json" handler.
type TransformInput struct {
	Input      any    `json:"input"`
	Expression string `json:"expression"`
}

// TransformOutput is kept for documentation; the handler completes with
// the raw jq value (unwrapped) so downstream steps receive it directly.
type TransformOutput struct {
	Result any `json:"result"`
}

// CompileExpression parses and compiles a jq expression without running
// it. Used by tests (and any future validate tooling) to guarantee every
// expression embedded in a recipe is at least syntactically valid —
// eval-time behavior still needs sample coverage in TestRecipeExpressions.
func CompileExpression(expression string) error {
	if expression == "" {
		return fmt.Errorf("transform.json: expression is required")
	}
	query, err := gojq.Parse(expression)
	if err != nil {
		return fmt.Errorf("transform.json: invalid expression: %w", err)
	}
	_, err = gojq.Compile(query)
	if err != nil {
		return fmt.Errorf("transform.json: cannot compile expression: %w", err)
	}
	return nil
}

// ApplyJQLight compiles expression once and collects all outputs. A single
// output is unwrapped; multiple outputs become an array.
func ApplyJQLight(input any, expression string) (any, error) {
	if expression == "" {
		return nil, fmt.Errorf("transform.json: expression is required")
	}
	query, err := gojq.Parse(expression)
	if err != nil {
		return nil, worker.NewNonRetryableError(fmt.Errorf("transform.json: invalid expression: %w", err))
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, worker.NewNonRetryableError(fmt.Errorf("transform.json: cannot compile expression: %w", err))
	}
	iter := code.Run(input)
	var results []any
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return nil, fmt.Errorf("transform.json: eval error: %w", err)
		}
		results = append(results, v)
	}
	if len(results) == 0 {
		return nil, nil
	}
	if len(results) == 1 {
		return results[0], nil
	}
	return results, nil
}

// decodeTransform accepts either the {"input","expression"} envelope or raw
// data (whole payload becomes Input, expression comes from metadata).
func decodeTransform(raw []byte, md map[string]string) (TransformInput, error) {
	var in TransformInput
	if len(raw) == 0 {
		return in, fmt.Errorf("handlers: empty task input")
	}
	if isJSONObject(raw) && hasAnyKey(raw, "input", "expression") {
		if err := json.Unmarshal(raw, &in); err != nil {
			return in, fmt.Errorf("handlers: invalid transform input: %w", err)
		}
	} else {
		data, err := decodeData(raw)
		if err != nil {
			return in, err
		}
		normalized, err := normalizeJSON(data)
		if err != nil {
			return in, err
		}
		in.Input = normalized
	}
	if in.Expression == "" {
		in.Expression = md["expression"]
	}
	return in, nil
}

func handleTransformJSON(ctx worker.TaskContext) error {
	in, err := decodeTransform(ctx.Input(), metadataOf(ctx.Metadata()))
	if err != nil {
		return worker.NewNonRetryableError(err)
	}
	if in.Expression == "" {
		return worker.NewNonRetryableError(fmt.Errorf("transform.json: expression is required (task input or step metadata)"))
	}
	res, err := ApplyJQLight(in.Input, in.Expression)
	if err != nil {
		return err
	}
	// Complete with the raw value (not {"result":…}) so the next step's
	// handler receives it directly — recipes chain without unwrap steps.
	return ctx.Complete(marshalOutput(res))
}
