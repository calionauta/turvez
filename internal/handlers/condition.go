package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danmestas/dagnats/worker"
)

// ConditionInput is the task payload for the "condition" handler.
//
// Prefer the engine-native `skip_if` on the step for simple cases
// (single field comparison) — it skips without spending a task
// execution. Use this handler when the rule needs a jq expression,
// a dot path into nested output, or explicit branch names.
type ConditionInput struct {
	Input      any    `json:"input"`
	Field      string `json:"field,omitempty"`
	Op         string `json:"op,omitempty"`
	Value      any    `json:"value,omitempty"`
	Expression string `json:"expression,omitempty"`
	OnTrue     string `json:"on_true,omitempty"`
	OnFalse    string `json:"on_false,omitempty"`
}

// ConditionOutput tells downstream steps which branch was taken.
type ConditionOutput struct {
	Matched bool   `json:"matched"`
	Branch  string `json:"branch,omitempty"`
}

// lookupDotPath resolves "a.b.c" inside a decoded JSON value.
func lookupDotPath(v any, path string) (any, bool) {
	if path == "" || path == "." {
		return v, true
	}
	cur := v
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// Compare applies op to left/right with dagnats skip_if semantics:
// numbers compare as float64, strings as strings, bools equality-only.
func Compare(left any, op string, right any) (bool, error) {
	switch op {
	case "==", "!=", "<", ">", "<=", ">=":
	default:
		return false, worker.NewNonRetryableError(fmt.Errorf("condition: invalid op %q (want ==, !=, <, >, <=, >=)", op))
	}
	lf, lok := toFloat(left)
	rf, rok := toFloat(right)
	if lok && rok {
		switch op {
		case "==":
			return lf == rf, nil
		case "!=":
			return lf != rf, nil
		case "<":
			return lf < rf, nil
		case ">":
			return lf > rf, nil
		case "<=":
			return lf <= rf, nil
		case ">=":
			return lf >= rf, nil
		}
	}
	if op != "==" && op != "!=" {
		ls, lok := left.(string)
		rs, rok := right.(string)
		if lok && rok {
			switch op {
			case "<":
				return ls < rs, nil
			case ">":
				return ls > rs, nil
			case "<=":
				return ls <= rs, nil
			case ">=":
				return ls >= rs, nil
			}
		}
		return false, fmt.Errorf("condition: op %q needs numbers (or strings), got %T vs %T", op, left, right)
	}
	lb, lok := left.(bool)
	rb, rok := right.(bool)
	if lok && rok {
		if op == "==" {
			return lb == rb, nil
		}
		return lb != rb, nil
	}
	ls, lok := stringify(left)
	rs, rok := stringify(right)
	if lok && rok {
		if op == "==" {
			return ls == rs, nil
		}
		return ls != rs, nil
	}
	return false, fmt.Errorf("condition: cannot compare %T with %q", left, op)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func stringify(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// EvalCondition evaluates in and returns the matched/branch output.
func EvalCondition(in ConditionInput) (ConditionOutput, error) {
	var matched bool
	if in.Expression != "" {
		res, err := ApplyJQLight(in.Input, in.Expression+" | . == true")
		if err != nil {
			return ConditionOutput{}, err
		}
		b, _ := res.(bool)
		matched = b
	} else {
		if in.Field == "" || in.Op == "" {
			return ConditionOutput{}, worker.NewNonRetryableError(fmt.Errorf("condition: need field+op+value or expression"))
		}
		var normalized any
		raw, _ := json.Marshal(in.Input)
		_ = json.Unmarshal(raw, &normalized)
		left, ok := lookupDotPath(normalized, in.Field)
		if !ok {
			return ConditionOutput{}, fmt.Errorf("condition: field %q not found in input", in.Field)
		}
		m, err := Compare(left, in.Op, in.Value)
		if err != nil {
			return ConditionOutput{}, err
		}
		matched = m
	}
	out := ConditionOutput{Matched: matched}
	if matched && in.OnTrue != "" {
		out.Branch = in.OnTrue
	} else if !matched && in.OnFalse != "" {
		out.Branch = in.OnFalse
	}
	return out, nil
}

// decodeCondition accepts the full envelope or raw data with the rule
// (field/op/value or expression) from step metadata.
func decodeCondition(raw []byte, md map[string]string) (ConditionInput, error) {
	var in ConditionInput
	if len(raw) == 0 {
		return in, fmt.Errorf("handlers: empty task input")
	}
	if isJSONObject(raw) && hasAnyKey(raw, "input", "field", "op", "value", "expression", "on_true", "on_false") {
		if err := json.Unmarshal(raw, &in); err != nil {
			return in, fmt.Errorf("handlers: invalid condition input: %w", err)
		}
	} else {
		data, err := decodeData(raw)
		if err != nil {
			return in, err
		}
		var normalized any
		round, _ := json.Marshal(data)
		_ = json.Unmarshal(round, &normalized)
		in.Input = normalized
	}
	if in.Field == "" {
		in.Field = md["field"]
	}
	if in.Op == "" {
		in.Op = md["op"]
	}
	if in.Value == nil && md["value"] != "" {
		in.Value = valueFromMeta(md["value"])
	}
	if in.Expression == "" {
		in.Expression = md["expression"]
	}
	if in.OnTrue == "" {
		in.OnTrue = md["on_true"]
	}
	if in.OnFalse == "" {
		in.OnFalse = md["on_false"]
	}
	return in, nil
}

func handleCondition(ctx worker.TaskContext) error {
	in, err := decodeCondition(ctx.Input(), metadataOf(ctx.Metadata()))
	if err != nil {
		return worker.NewNonRetryableError(err)
	}
	out, err := EvalCondition(in)
	if err != nil {
		return err
	}
	return ctx.Complete(marshalOutput(out))
}
