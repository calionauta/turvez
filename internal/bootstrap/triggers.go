// Package bootstrap carries the startup shims this runtime needs before it can
// use the dagnats engine.
package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Dagnats v0.0.24 cannot create the first trigger through its console.
//
// # The upstream bug
//
// internal/api/service_triggers.go, listTriggersInner:
//
//	keys, err := s.triggerKV.Keys(ctx)
//	if err != nil {
//		return nil, err          // ErrNoKeysFound returned raw
//	}
//
// When the "triggers" bucket exists but holds no keys, Keys returns
// jetstream.ErrNoKeysFound and that error is propagated. The console calls
// ListTriggers BEFORE writing, to check for a duplicate id, so on a fresh
// engine it answers 500 and never writes:
//
//	POST /console/triggers -> 500 lookup failed: nats: no keys found
//
// The bucket therefore never becomes non-empty, and since the console is the
// only surface that creates triggers over HTTP, a fresh installation is stuck.
//
// Every other KV read in that package treats the condition as benign — 25
// call sites do `errors.Is(err, jetstream.ErrNoKeysFound)`, including
// checkHTTPRouteConflict, which runs inside CreateTrigger itself. The comment
// 58 lines above the bug in the same file states the intent verbatim:
// "No keys yet is a benign \"first trigger\" case." So this is an oversight
// upstream, not a design decision.
//
// # Why this project is not currently affected
//
// All triggers here are declared inline in recipes and applied with
// `dagnats workflow register`, which writes through CreateTrigger or straight
// to the KV. That path never calls the raising function, so nothing in the
// normal workflow hits the bug. This shim exists for the case where someone
// opens the console on a fresh engine — which docs/agent-tools.md documents as
// a supported surface — and would otherwise meet a 500 with no explanation.
//
// # Upstream
//
// Fix PR: https://github.com/danmestas/dagnats/pull/745
// Re-check this file on every dagnats bump; delete it in the SAME commit as
// the bump that includes the fix. See AGENTS.md, "dagnats v0.0.24 traps".

const (
	// triggerBucket is the KV bucket dagnats stores trigger definitions in.
	triggerBucket = "triggers"

	// bootstrapTriggerID is the id of the seeded placeholder. The underscore
	// prefix keeps it visibly non-user in the console listing.
	bootstrapTriggerID = "_bootstrap"

	// bootstrapTriggerWorkflow is the workflow the placeholder points at. Any
	// registered workflow works; the placeholder is disabled, so it never
	// fires. Keep it pointing at something real so the row is valid rather
	// than a dangling reference.
	bootstrapTriggerWorkflow = "ops/dlq-watchdog"

	// bootstrapCron is a cron expression used only to satisfy validation. The
	// trigger is disabled, so it is never scheduled.
	bootstrapCron = "0 3 * * *"

	bootstrapSource  = "bootstrap"
	bootstrapTimeout = 15 * time.Second
)

// placeholderTrigger mirrors the subset of dagnats' trigger.TriggerDef that the
// seed needs. It is defined locally because that type lives in dagnats'
// internal/trigger package, which cannot be imported here. The JSON tags are
// snake_case because they must match the upstream wire format exactly — the
// value is read back by dagnats' own unmarshaler.
//
//nolint:tagliatelle // wire format is fixed by the upstream contract
type placeholderTrigger struct {
	ID         string           `json:"id"`
	WorkflowID string           `json:"workflow_id"`
	Enabled    bool             `json:"enabled"`
	Cron       *placeholderCron `json:"cron,omitempty"`
	Source     string           `json:"source,omitempty"`
}

type placeholderCron struct {
	Expression string `json:"expression"`
}

// EnsureTriggerBucket makes the dagnats trigger bucket safe to use by keeping
// it non-empty.
//
// It reads first and writes only when the bucket has no keys, so an
// installation that already has triggers is never touched and repeated boots
// are no-ops. Set DAGNATS_TRIGGER_BOOTSTRAP=false to skip it entirely.
//
// Returns nil when the bucket is already usable. Errors are returned for the
// caller to log: this runs on the boot path and must never be fatal, so callers
// should warn and continue.
func EnsureTriggerBucket(nc *nats.Conn) error {
	if nc == nil {
		return errors.New("dagnats bootstrap: nil NATS connection")
	}

	ctx, cancel := context.WithTimeout(context.Background(), bootstrapTimeout)
	defer cancel()

	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("dagnats bootstrap: jetstream: %w", err)
	}

	kv, err := js.KeyValue(ctx, triggerBucket)
	if err != nil {
		// The bucket is created by the engine at startup. A miss means the
		// engine is not up yet or the name changed upstream — both worth
		// surfacing rather than guessing.
		return fmt.Errorf("dagnats bootstrap: open %s bucket: %w", triggerBucket, err)
	}

	keys, err := kv.Keys(ctx)
	switch {
	case err == nil && len(keys) > 0:
		log.Printf("dagnats bootstrap: trigger bucket already has %d entries, no seed needed", len(keys))
		return nil
	case err != nil && !errors.Is(err, jetstream.ErrNoKeysFound):
		return fmt.Errorf("dagnats bootstrap: list %s keys: %w", triggerBucket, err)
	}

	// Empty bucket: the exact condition that breaks the console.
	seed := placeholderTrigger{
		ID:         bootstrapTriggerID,
		WorkflowID: bootstrapTriggerWorkflow,
		Enabled:    false, // never fires
		Cron:       &placeholderCron{Expression: bootstrapCron},
		Source:     bootstrapSource,
	}
	payload, err := json.Marshal(seed)
	if err != nil {
		return fmt.Errorf("dagnats bootstrap: marshal seed: %w", err)
	}
	if _, err := kv.Put(ctx, bootstrapTriggerID, payload); err != nil {
		return fmt.Errorf("dagnats bootstrap: put seed: %w", err)
	}

	log.Printf("dagnats bootstrap: seeded a placeholder trigger (WORKAROUND for "+
		"DagNats v0.0.24 — the console cannot create the first trigger without "+
		"it). Safe to delete once you have created your own. Upstream fix: "+
		"https://github.com/danmestas/dagnats/pull/745 — id=%s workflow=%s enabled=false",
		bootstrapTriggerID, bootstrapTriggerWorkflow)
	return nil
}

// Enabled reports whether the workaround should run. Default true: without it a
// fresh engine's console cannot create its first trigger.
func Enabled() bool {
	return os.Getenv("DAGNATS_TRIGGER_BOOTSTRAP") != "false"
}
