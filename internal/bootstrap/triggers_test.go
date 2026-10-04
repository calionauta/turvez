package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// bootNATSWithBucket starts an in-process NATS with JetStream and creates the
// "triggers" bucket empty — the exact precondition that breaks the console.
func bootNATSWithBucket(t *testing.T) *nats.Conn {
	t.Helper()
	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1, // random free port
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	}
	ns, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		t.Fatal("nats server not ready")
	}
	t.Cleanup(ns.Shutdown)

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	t.Cleanup(nc.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream.New: %v", err)
	}
	if _, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: triggerBucket}); err != nil {
		t.Fatalf("create %s bucket: %v", triggerBucket, err)
	}
	return nc
}

func keys(t *testing.T, nc *nats.Conn) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream.New: %v", err)
	}
	kv, err := js.KeyValue(ctx, triggerBucket)
	if err != nil {
		t.Fatalf("open bucket: %v", err)
	}
	got, err := kv.Keys(ctx)
	if err != nil {
		// ErrNoKeysFound is the empty case, which is what is under test.
		return nil
	}
	return got
}

// TestEnsureTriggerBucket_SeedsEmptyBucket is the core behaviour: an empty
// bucket gets exactly one seed, and the seed is a real disabled definition
// rather than a marker key. A marker would render in the console as a phantom
// trigger of kind "unknown".
func TestEnsureTriggerBucket_SeedsEmptyBucket(t *testing.T) {
	nc := bootNATSWithBucket(t)

	if got := keys(t, nc); len(got) != 0 {
		t.Fatalf("precondition: expected an empty bucket, got %v", got)
	}

	if err := EnsureTriggerBucket(nc); err != nil {
		t.Fatalf("EnsureTriggerBucket: %v", err)
	}

	got := keys(t, nc)
	if len(got) != 1 || got[0] != bootstrapTriggerID {
		t.Fatalf("expected exactly [%s], got %v", bootstrapTriggerID, got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	js, _ := jetstream.New(nc)
	kv, _ := js.KeyValue(ctx, triggerBucket)
	entry, err := kv.Get(ctx, bootstrapTriggerID)
	if err != nil {
		t.Fatalf("get seed: %v", err)
	}

	var got2 placeholderTrigger
	if err := json.Unmarshal(entry.Value(), &got2); err != nil {
		t.Fatalf("seed is not valid JSON for a TriggerDef: %v", err)
	}
	if got2.Enabled {
		t.Error("seed must be DISABLED so it never fires, got enabled=true")
	}
	if got2.WorkflowID == "" {
		t.Error("seed must reference a workflow, got empty workflow_id")
	}
	if got2.Cron == nil || got2.Cron.Expression == "" {
		t.Errorf("seed must carry a cron expression, got %+v", got2.Cron)
	}
	if got2.Source != bootstrapSource {
		t.Errorf("seed source = %q, want %q", got2.Source, bootstrapSource)
	}
}

// TestEnsureTriggerBucket_Idempotent guards the property that matters for a
// boot-path call: running it on a populated bucket must not add or change
// anything. Without this, every worker restart would inject another row.
func TestEnsureTriggerBucket_Idempotent(t *testing.T) {
	nc := bootNATSWithBucket(t)

	if err := EnsureTriggerBucket(nc); err != nil {
		t.Fatalf("first call: %v", err)
	}
	first := keys(t, nc)

	for range 3 {
		if err := EnsureTriggerBucket(nc); err != nil {
			t.Fatalf("repeat call: %v", err)
		}
	}
	after := keys(t, nc)

	if len(after) != len(first) || len(after) != 1 {
		t.Fatalf("repeated calls changed the bucket: %v -> %v", first, after)
	}
}

// TestEnsureTriggerBucket_LeavesExistingTriggersAlone proves an installation
// that already has its own triggers is never touched. This project registers
// its triggers from recipes, so that is the normal state after the first
// deploy — the shim must not displace real data.
func TestEnsureTriggerBucket_LeavesExistingTriggersAlone(t *testing.T) {
	nc := bootNATSWithBucket(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	js, _ := jetstream.New(nc)
	kv, _ := js.KeyValue(ctx, triggerBucket)

	real := `{"id":"ops-nightly","workflow_id":"ops/dlq-watchdog","enabled":true,` +
		`"cron":{"expression":"0 3 * * *"},"source":"file"}`
	if _, err := kv.Put(ctx, "ops-nightly", []byte(real)); err != nil {
		t.Fatalf("seed real trigger: %v", err)
	}

	if err := EnsureTriggerBucket(nc); err != nil {
		t.Fatalf("EnsureTriggerBucket: %v", err)
	}

	got := keys(t, nc)
	if len(got) != 1 || got[0] != "ops-nightly" {
		t.Fatalf("existing triggers must be untouched; got %v", got)
	}
	entry, err := kv.Get(ctx, "ops-nightly")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(entry.Value()) != real {
		t.Errorf("the existing trigger was modified:\n got %s\nwant %s", entry.Value(), real)
	}
}

// TestEnsureTriggerBucket_Errors keeps the failure modes loud: a nil connection
// must be a returned error, and a missing bucket must surface rather than
// silently seeding nothing (it would mean the engine is down or the bucket was
// renamed upstream).
func TestEnsureTriggerBucket_Errors(t *testing.T) {
	if err := EnsureTriggerBucket(nil); err == nil {
		t.Error("nil connection must be an error")
	}

	opts := &natsserver.Options{
		Host: "127.0.0.1", Port: -1, JetStream: true,
		StoreDir: t.TempDir(), NoLog: true, NoSigs: true,
	}
	ns, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		t.Fatal("nats not ready")
	}
	t.Cleanup(ns.Shutdown)
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(nc.Close)

	// Note: no "triggers" bucket created.
	if err := EnsureTriggerBucket(nc); err == nil {
		t.Fatal("expected an error when the triggers bucket does not exist")
	}
}

// TestEnabled pins the opt-out contract: only the exact string "false" disables
// the shim, so an unset variable behaves like the documented default (on).
func TestEnabled(t *testing.T) {
	for _, tc := range []struct {
		value string
		set   bool
		want  bool
	}{
		{"", false, true},    // unset -> default on
		{"", true, true},     // empty -> on
		{"true", true, true}, // anything but "false" -> on
		{"1", true, true},    //
		{"false", true, false},
		{"FALSE", true, true}, // exact match only, matching the rest of the repo
	} {
		if tc.set {
			t.Setenv("DAGNATS_TRIGGER_BOOTSTRAP", tc.value)
		} else {
			os.Unsetenv("DAGNATS_TRIGGER_BOOTSTRAP")
		}
		if got := Enabled(); got != tc.want {
			t.Errorf("Enabled() with value=%q set=%v = %v, want %v",
				tc.value, tc.set, got, tc.want)
		}
	}
}
