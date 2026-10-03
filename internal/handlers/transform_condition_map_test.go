package handlers

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestApplyJQLight(t *testing.T) {
	tests := []struct {
		name  string
		input any
		expr  string
		want  any
	}{
		{name: "field", input: map[string]any{"a": 1.0}, expr: ".a", want: float64(1)},
		{name: "map array", input: []any{1.0, 2.0}, expr: "map(. * 2)", want: []any{2.0, 4.0}},
		{name: "null on missing", input: map[string]any{}, expr: ".missing", want: nil},
		{name: "object build", input: map[string]any{"x": 1.0}, expr: "{v: .x}", want: map[string]any{"v": 1.0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ApplyJQLight(tc.input, tc.expr)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
	if _, err := ApplyJQLight(map[string]any{}, "[["); err == nil {
		t.Fatal("expected compile error")
	}
}

func TestEvalCondition(t *testing.T) {
	tests := []struct {
		name  string
		in    ConditionInput
		match bool
	}{
		{name: "gt true", in: ConditionInput{Input: map[string]any{"amount": 20000.0}, Field: "amount", Op: ">", Value: 10000.0}, match: true},
		{name: "gt false", in: ConditionInput{Input: map[string]any{"amount": 5.0}, Field: "amount", Op: ">", Value: 10000.0}, match: false},
		{name: "eq string", in: ConditionInput{Input: map[string]any{"s": "x"}, Field: "s", Op: "==", Value: "x"}, match: true},
		{name: "neq bool", in: ConditionInput{Input: map[string]any{"b": true}, Field: "b", Op: "!=", Value: false}, match: true},
		{name: "nested path", in: ConditionInput{Input: map[string]any{"a": map[string]any{"b": 3.0}}, Field: "a.b", Op: "==", Value: 3.0}, match: true},
		{name: "jq expr", in: ConditionInput{Input: map[string]any{"n": 7.0}, Expression: ".n > 5"}, match: true},
		{name: "jq expr false", in: ConditionInput{Input: map[string]any{"n": 1.0}, Expression: ".n > 5"}, match: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := EvalCondition(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if out.Matched != tc.match {
				t.Fatalf("matched = %v, want %v", out.Matched, tc.match)
			}
		})
	}
}

func TestEvalConditionBranches(t *testing.T) {
	out, err := EvalCondition(ConditionInput{Input: map[string]any{"a": 1.0}, Field: "a", Op: "==", Value: 1.0, OnTrue: "vip", OnFalse: "std"})
	if err != nil || out.Branch != "vip" {
		t.Fatalf("got %+v, %v", out, err)
	}
	if _, err := EvalCondition(ConditionInput{Input: map[string]any{}}); err == nil {
		t.Fatal("expected error without field/expression")
	}
}

func TestApplyMap(t *testing.T) {
	in := MapInput{Items: []any{1.0, 2.0, 3.0}, Expression: ". * 10", Concurrency: 2}
	out, err := ApplyMap(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 3 || len(out.Results) != 3 {
		t.Fatalf("got %+v", out)
	}
	if out.Results[0] != 10.0 || out.Results[2] != 30.0 {
		t.Fatalf("order not preserved: %#v", out.Results)
	}
	empty, err := ApplyMap(context.Background(), MapInput{Items: []any{}})
	if err != nil || empty.Count != 0 {
		t.Fatalf("empty: %+v, %v", empty, err)
	}
}

func TestParseDelay(t *testing.T) {
	if _, err := ParseDelay("30s"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "nope", "-5s", "0s", "72h"} {
		if _, err := ParseDelay(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
	_ = time.Second
}
