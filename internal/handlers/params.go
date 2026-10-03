package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Step metadata (the recipe-static half) meets task input (the dynamic
// half) here. Convention, documented in docs/writing-recipes.md:
//
//   - Static config lives in the step's `metadata` (strings only):
//     jq expressions, secret NAMES, branch names, timeouts.
//   - Dynamic data flows through task input (parent step outputs).
//   - When both set the same key, task input wins.
//
// Handlers additionally accept "raw" input: when the payload is not an
// object with the handler's envelope keys, the whole payload is treated
// as the data (e.g. a bare array for map, a bare string for delay).

// metadataOf returns the step metadata or an empty map.
func metadataOf(md map[string]string) map[string]string {
	if md == nil {
		return map[string]string{}
	}
	return md
}

// isJSONObject reports whether raw starts (past whitespace) with '{'.
func isJSONObject(raw []byte) bool {
	for _, b := range raw {
		switch b {
		case ' ', '\n', '\t', '\r':
			continue
		default:
			return b == '{'
		}
	}
	return false
}

// hasAnyKey reports whether the JSON object at raw contains any of keys.
func hasAnyKey(raw []byte, keys ...string) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	for _, k := range keys {
		if _, ok := probe[k]; ok {
			return true
		}
	}
	return false
}

// decodeData unmarshals any JSON value into any.
func decodeData(raw []byte) (any, error) {
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("handlers: invalid task input JSON: %w", err)
	}
	return data, nil
}

// decodeDataInto unmarshals raw JSON into v with a wrapped error.
func decodeDataInto(raw []byte, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("handlers: invalid task input JSON: %w", err)
	}
	return nil
}

// normalizeJSON round-trips a decoded value through JSON so numbers are
// float64, matching what handlers see from real task payloads in tests
// that construct values in Go (gojq preserves Go ints otherwise).
func normalizeJSON(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("handlers: cannot normalize input: %w", err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("handlers: cannot normalize input: %w", err)
	}
	return out, nil
}

// intFromMeta parses an int metadata value; 0 when unset/invalid.
func intFromMeta(md map[string]string, key string) int {
	s, ok := md[key]
	if !ok || s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// boolFromMeta parses a bool metadata value.
func boolFromMeta(md map[string]string, key string) bool {
	s, ok := md[key]
	return ok && (s == "true" || s == "1" || s == "yes")
}

// valueFromMeta parses a metadata string as JSON when possible,
// falling back to the raw string (for condition value comparisons).
func valueFromMeta(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	return v
}
