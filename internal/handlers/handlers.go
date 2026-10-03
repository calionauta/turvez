// Shared wiring for the generic task handlers.
//
// Task names (stable — recipes depend on them):
//
//	http.request, transform.json, condition, map, delay,
//	notify.slack, request.approval, llm.complete
package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/danmestas/dagnats/worker"
)

// decodeInput unmarshals the raw task payload into v.
func decodeInput(raw []byte, v any) error {
	if len(raw) == 0 {
		return fmt.Errorf("handlers: empty task input")
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("handlers: invalid task input JSON: %w", err)
	}
	return nil
}

// marshalOutput marshals a handler result for ctx.Complete. An
// unserializable value cannot occur for JSON-derived data; the static
// fallback keeps the failure visible instead of panicking.
func marshalOutput(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte(`{"error":"unserializable handler output"}`)
	}
	return data
}

// RegisterAll registers every generic handler on w. Called once by cmd/worker.
func RegisterAll(w *worker.Worker) {
	w.Handle("http.request", handleHTTPRequest)
	w.Handle("transform.json", handleTransformJSON)
	w.Handle("condition", handleCondition)
	w.Handle("map", handleMap)
	w.Handle("delay", handleDelay)
	w.Handle("notify.slack", handleNotifySlack)
	w.Handle("request.approval", handleRequestApproval)
	w.Handle("llm.complete", handleLLMComplete)
}
