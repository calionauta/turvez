package handlers

import (
	"testing"
)

func TestDecodeTransformRawAndMetadata(t *testing.T) {
	// Raw data + metadata expression (the common recipe shape: parent
	// output flows in, jq expression lives in step metadata).
	in, err := decodeTransform([]byte(`{"a":1}`), map[string]string{"expression": ".a"})
	if err != nil {
		t.Fatal(err)
	}
	if in.Expression != ".a" {
		t.Fatalf("expression = %q", in.Expression)
	}
	out, err := ApplyJQLight(mustNorm(t, in.Input), in.Expression)
	if err != nil || out != 1.0 {
		t.Fatalf("got %#v, %v", out, err)
	}

	// Envelope path still wins over metadata.
	in2, err := decodeTransform([]byte(`{"input":{"a":2},"expression":".a"}`), map[string]string{"expression": ".x"})
	if err != nil {
		t.Fatal(err)
	}
	if in2.Expression != ".a" {
		t.Fatalf("input must win, got %q", in2.Expression)
	}
}

func TestDecodeConditionMetadataRule(t *testing.T) {
	in, err := decodeCondition([]byte(`{"amount":20000}`), map[string]string{
		"field": "amount", "op": ">", "value": "10000",
		"on_true": "vip", "on_false": "std",
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := EvalCondition(in)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Matched || out.Branch != "vip" {
		t.Fatalf("got %+v", out)
	}
}

func TestDecodeMapBareArray(t *testing.T) {
	in, err := decodeMap([]byte(`[1,2]`), map[string]string{"expression": ". * 2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Items) != 2 || in.Expression != ". * 2" {
		t.Fatalf("got %+v", in)
	}
}

func TestDecodeDelayForms(t *testing.T) {
	for _, raw := range []string{`{"duration":"30s"}`, `"30s"`} {
		in, err := decodeDelay([]byte(raw), nil)
		if err != nil || in.Duration != "30s" {
			t.Fatalf("%s: got %+v, %v", raw, in, err)
		}
	}
	in, err := decodeDelay([]byte(``), map[string]string{"duration": "5m"})
	if err != nil || in.Duration != "5m" {
		t.Fatalf("metadata: got %+v, %v", in, err)
	}
}

func TestDecodeNotifyForms(t *testing.T) {
	in, err := decodeNotify([]byte(`"plain text"`), map[string]string{"webhook_secret": "HOOK"})
	if err != nil || in.Message != "plain text" || in.WebhookSecret != "HOOK" {
		t.Fatalf("got %+v, %v", in, err)
	}
}

func mustNorm(t *testing.T, v any) any {
	t.Helper()
	return v
}
