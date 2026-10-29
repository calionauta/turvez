// Package workflows verifies the example recipes structurally and by
// executing every embedded jq expression against sample payloads.
//
// This is the contract test for "automation code is workflow JSON":
// if a recipe's expression breaks, this fails without needing NATS.
package workflows

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/calionauta/turvez/internal/handlers"
)

var knownTasks = map[string]bool{
	"http.request": true, "transform.json": true, "condition": true,
	"map": true, "delay": true, "notify.webhook": true,
	"notify.slack": true, "notify.telegram": true, "notify.discord": true,
	"request.approval": true, "llm.complete": true, "dlq.check": true,
	"": true, // engine-native steps (sleep/approval) need no task
}

var validOps = map[string]bool{
	"==": true, "!=": true, "<": true, ">": true, "<=": true, ">=": true,
}

type stepDef struct {
	ID        string            `json:"id"`
	Task      string            `json:"task"`
	DependsOn []string          `json:"depends_on"`
	Timeout   any               `json:"timeout"`
	Type      string            `json:"type"`
	Config    map[string]any    `json:"config"`
	SkipIf    *skipIf           `json:"skip_if"`
	Metadata  map[string]string `json:"metadata"`
	OnFailure string            `json:"on_failure"`
}

type skipIf struct {
	StepID string `json:"step_id"`
	Field  string `json:"field"`
	Op     string `json:"op"`
	Value  any    `json:"value"`
}

type workflowFile struct {
	Name     string          `json:"name"`
	Version  string          `json:"version"`
	Steps    []stepDef       `json:"steps"`
	AuxSteps map[string]bool `json:"aux_steps"`
	Triggers []triggerDef    `json:"triggers"`
}

type triggerDef struct {
	ID      string `json:"id"`
	Webhook *struct {
		Path string `json:"path"`
	} `json:"webhook"`
	Cron    any `json:"cron"`
	Subject any `json:"subject"`
}

// recipeFiles lists every client recipe. _templates holds TODO-CLIENT
// placeholders (validated by `make validate`, not by these tests).
//
// WORKFLOWS_DIR override: points at another checkout's workflows dir
// (e.g. ../turvez-ops/workflows) so the template's contract tests can
// validate a private ops repo without copying Go code. Empty = "." (this
// package's dir, the template default).
func workflowsRoot() string {
	if v := os.Getenv("WORKFLOWS_DIR"); v != "" {
		return v
	}
	return "."
}

func recipeFiles(t *testing.T) []string {
	t.Helper()
	root := workflowsRoot()
	dirs, err := filepath.Glob(filepath.Join(root, "*"))
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, d := range dirs {
		if filepath.Base(d) == "_templates" {
			continue
		}
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(d, "*.json"))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, matches...)
	}
	if len(files) == 0 {
		t.Fatal("no client recipes found")
	}
	return files
}

func loadRecipes(t *testing.T) map[string]workflowFile {
	t.Helper()
	files := recipeFiles(t)
	out := map[string]workflowFile{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var wf workflowFile
		if err := json.Unmarshal(raw, &wf); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		// Stable key: <client>/<file>.json regardless of WORKFLOWS_DIR
		// (absolute or relative). Keeps curated expression tests working
		// against a private ops checkout.
		key := filepath.Join(filepath.Base(filepath.Dir(f)), filepath.Base(f))
		out[key] = wf
	}
	return out
}

// namingConfig is the single source of truth for how workflow names and
// trigger ids are shaped. It lives at the repo root (naming.json) so a
// convention change is one edit plus scripts/rename-workflows.sh.
type namingConfig struct {
	WorkflowNameRegex       string `json:"workflow_name_regex"`
	ClientWorkflowSeparator string `json:"client_workflow_separator"`
	TriggerIDSeparator      string `json:"trigger_id_separator"`
}

func loadNaming(t *testing.T) (namingConfig, *regexp.Regexp) {
	t.Helper()
	raw, err := os.ReadFile("../naming.json")
	if err != nil {
		t.Fatalf("naming.json: %v", err)
	}
	var cfg namingConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("naming.json: %v", err)
	}
	re, err := regexp.Compile(cfg.WorkflowNameRegex)
	if err != nil {
		t.Fatalf("naming.json workflow_name_regex: %v", err)
	}
	return cfg, re
}

func TestRecipeStructure(t *testing.T) {
	// The same loop also rejects embedded credential values: JSON carries
	// secret NAMES only. ($schema is accepted since dagnats v0.0.26 and is
	// optional; none of these recipes carry it.)
	var secretValuePatterns = []string{
		"hooks.slack.com/services/", "xoxb-", "xoxp-", "sk-live-",
		"sk-test-", "ghp_", "-----BEGIN",
	}
	for _, f := range recipeFiles(t) {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range secretValuePatterns {
			if strings.Contains(string(raw), p) {
				t.Fatalf("%s: looks like an embedded secret value (%q); use secret names only", f, p)
			}
		}
	}
	for path, wf := range loadRecipes(t) {
		client := filepath.Dir(path) // <client-id>/file.json (Base-normalized in loadRecipes)
		// filepath.Dir on "client/file" returns "client"; on absolute paths
		// fall back to Base to survive WORKFLOWS_DIR overrides.
		if strings.Contains(client, string(os.PathSeparator)) {
			client = filepath.Base(client)
		}
		t.Run(path, func(t *testing.T) {
			nm, nameRe := loadNaming(t)
			if wf.Name == "" || wf.Version == "" {
				t.Fatal("name and version are required")
			}
			// Tenancy: folder, workflow name, and trigger ids/paths
			// share the <client-id> prefix. No shared names. The separator
			// and the full name shape come from naming.json.
			if !strings.HasPrefix(wf.Name, client+nm.ClientWorkflowSeparator) {
				t.Fatalf("name %q must start with %q%s (folder prefix)",
					wf.Name, client, nm.ClientWorkflowSeparator)
			}
			if !nameRe.MatchString(wf.Name) {
				t.Fatalf("name %q does not match %q (naming.json)",
					wf.Name, nm.WorkflowNameRegex)
			}
			for _, tr := range wf.Triggers {
				if tr.ID == "" {
					t.Fatal("trigger with empty id")
				}
				if !strings.HasPrefix(tr.ID, client+nm.TriggerIDSeparator) {
					t.Fatalf("trigger id %q must start with %q%s",
						tr.ID, client, nm.TriggerIDSeparator)
				}
				if tr.Webhook != nil && !strings.Contains(tr.Webhook.Path, client) {
					t.Fatalf("webhook path %q must contain %q", tr.Webhook.Path, client)
				}
			}
			ids := map[string]bool{}
			auxTargets := map[string]bool{}
			for _, s := range wf.Steps {
				if s.OnFailure != "" {
					auxTargets[s.OnFailure] = true
				}
			}
			for _, s := range wf.Steps {
				if s.ID == "" {
					t.Fatal("step with empty id")
				}
				if ids[s.ID] {
					t.Fatalf("duplicate step id %q", s.ID)
				}
				ids[s.ID] = true
				if !knownTasks[s.Task] {
					t.Fatalf("step %q: unknown task %q", s.ID, s.Task)
				}
				for _, d := range s.DependsOn {
					if !ids[d] && !depExistsLater(wf.Steps, d) {
						t.Fatalf("step %q depends on unknown %q", s.ID, d)
					}
				}
				// Durations accept integer nanoseconds or a Go duration
				// string ("30s") since dagnats v0.0.26; both parse.
				if s.Timeout != nil {
					switch v := s.Timeout.(type) {
					case float64:
					case string:
						if _, err := time.ParseDuration(v); err != nil {
							t.Fatalf("step %q: timeout %q: %v", s.ID, v, err)
						}
					default:
						t.Fatalf("step %q: timeout must be ns or a duration string, got %T", s.ID, v)
					}
				}
				if s.Type == "map" && len(s.DependsOn) != 1 {
					t.Fatalf("map step %q needs exactly one dep", s.ID)
				}
				if s.SkipIf != nil {
					si := s.SkipIf
					found := false
					for _, d := range s.DependsOn {
						if d == si.StepID {
							found = true
						}
					}
					if !found {
						t.Fatalf("step %q: skip_if.step_id must be in depends_on", s.ID)
					}
					if !validOps[si.Op] {
						t.Fatalf("step %q: invalid skip_if op %q", s.ID, si.Op)
					}
				}
				if s.OnFailure != "" {
					if !depExistsLater(wf.Steps, s.OnFailure) && !ids[s.OnFailure] {
						t.Fatalf("step %q: on_failure target %q missing", s.ID, s.OnFailure)
					}
					if !wf.AuxSteps[s.OnFailure] {
						t.Fatalf("step %q: on_failure target %q must be listed in aux_steps (JSON registration does not derive it)", s.ID, s.OnFailure)
					}
				}
				if auxTargets[s.ID] && len(s.DependsOn) > 0 {
					t.Fatalf("step %q: on_failure targets must not have depends_on (engine rejects it)", s.ID)
				}
			}
			if err := checkAcyclic(wf.Steps); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestClientFieldbooks enforces the fieldbook-per-client structure: every
// client folder (except _templates, which holds the template itself) must
// have a README.md copied from workflows/_templates/FIELDBOOK.md.
func TestClientFieldbooks(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join(workflowsRoot(), "*"))
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	// Hint points at the template checkout when validating an ops repo.
	templateHint := "workflows/_templates/FIELDBOOK.md"
	if os.Getenv("WORKFLOWS_DIR") != "" {
		if tc := os.Getenv("TURVEZ_CHECKOUT"); tc != "" {
			templateHint = tc + "/workflows/_templates/FIELDBOOK.md"
		} else {
			templateHint = "<turvez-template>/workflows/_templates/FIELDBOOK.md (set TURVEZ_CHECKOUT)"
		}
	}
	for _, d := range dirs {
		fi, err := os.Stat(d)
		if err != nil || !fi.IsDir() || filepath.Base(d) == "_templates" {
			continue
		}
		found++
		readme := filepath.Join(d, "README.md")
		raw, err := os.ReadFile(readme)
		if err != nil {
			t.Errorf("%s: missing README.md (copy %s)", d, templateHint)
			continue
		}
		if !strings.Contains(string(raw), "Fieldbook:") {
			t.Errorf("%s/README.md: must start from the FIELDBOOK template (missing 'Fieldbook:' header)", d)
		}
	}
	if found == 0 {
		t.Fatal("no client folders found")
	}
}

// TestNoRealClientsInTemplate keeps this public repo template-only.
// Real customers live in a private turvez-ops checkout (see docs/ops-repo.md).
// Allowlist: _templates, example-client, ops, personal. Anything else fails
// with a move-to-ops message. Skipped when WORKFLOWS_DIR points at an ops
// repo (there, any client folder is legitimate).
func TestNoRealClientsInTemplate(t *testing.T) {
	if os.Getenv("WORKFLOWS_DIR") != "" {
		t.Skip("ops repo under test: any client folder is allowed")
	}
	allowed := map[string]bool{
		"_templates": true, "example-client": true, "ops": true, "personal": true,
	}
	dirs, err := filepath.Glob(filepath.Join(workflowsRoot(), "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		fi, err := os.Stat(d)
		if err != nil || !fi.IsDir() {
			continue
		}
		base := filepath.Base(d)
		if !allowed[base] {
			t.Errorf("real client %q in template repo — move to turvez-ops (docs/ops-repo.md); template keeps only _templates/example-client/ops/personal", base)
		}
	}
}

// TestRecipeExpressionsCompile guarantees every transform.json expression
// embedded in ANY recipe at least parses+compiles. Sample-based eval tests
// (TestRecipeExpressions) only cover curated recipes; this catches the
// common LLM failure mode — a syntactically broken jq expression in a new
// client recipe — without needing per-recipe fixtures.
func TestRecipeExpressionsCompile(t *testing.T) {
	checked := 0
	for path, wf := range loadRecipes(t) {
		for _, s := range wf.Steps {
			expr := s.Metadata["expression"]
			if s.Task != "transform.json" || expr == "" {
				continue
			}
			checked++
			if err := handlers.CompileExpression(expr); err != nil {
				t.Errorf("%s step %q: %v", path, s.ID, err)
			}
		}
		t.Logf("%s: expressions compile", path)
	}
	if checked == 0 {
		t.Fatal("no transform.json expressions found — check the glob")
	}
}

func depExistsLater(steps []stepDef, id string) bool {
	for _, s := range steps {
		if s.ID == id {
			return true
		}
	}
	return false
}

func checkAcyclic(steps []stepDef) error {
	indeg := map[string]int{}
	next := map[string][]string{}
	for _, s := range steps {
		indeg[s.ID] += 0
		for _, d := range s.DependsOn {
			indeg[s.ID]++
			next[d] = append(next[d], s.ID)
		}
	}
	var queue []string
	for id, n := range indeg {
		if n == 0 {
			queue = append(queue, id)
		}
	}
	seen := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		seen++
		for _, m := range next[id] {
			indeg[m]--
			if indeg[m] == 0 {
				queue = append(queue, m)
			}
		}
	}
	if seen != len(steps) {
		return errCycle("dependency graph has a cycle")
	}
	return nil
}

type cycleErr string

func errCycle(s string) error    { return cycleErr(s) }
func (e cycleErr) Error() string { return string(e) }

// isThree tolerates gojq returning int for length.
func isThree(v any) bool {
	switch n := v.(type) {
	case int:
		return n == 3
	case float64:
		return n == 3
	}
	return false
}

// eval runs a recipe's jq expression against sample data.
func eval(t *testing.T, expr string, input any) any {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal sample: %v", err)
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		t.Fatalf("normalize sample: %v", err)
	}
	res, err := handlers.ApplyJQLight(normalized, expr)
	if err != nil {
		t.Fatalf("expr %q: %v", expr, err)
	}
	return res
}

func exprOf(wf workflowFile, stepID string) string {
	for _, s := range wf.Steps {
		if s.ID == stepID {
			return s.Metadata["expression"]
		}
	}
	return ""
}

func TestRecipeExpressions(t *testing.T) {
	all := loadRecipes(t)
	require := func(tt *testing.T, key string) workflowFile {
		tt.Helper()
		wf, ok := all[key]
		if !ok {
			// Curated fixtures live only in the template. When WORKFLOWS_DIR
			// points at a private ops repo, that recipe legitimately absents.
			tt.Skipf("fixture %q absent (ops repo under test?)", key)
		}
		return wf
	}

	t.Run("hubspot-lead-to-slack", func(t *testing.T) {
		wf := require(t, "example-client/hubspot-lead-to-slack.json")
		// Manual run input and trigger-envelope input must both work.
		for _, runInput := range []any{
			map[string]any{"lead_id": "123"},
			map[string]any{"trigger": "webhook", "data": map[string]any{"lead_id": "123"}},
		} {
			fetch := eval(t, exprOf(wf, "build-fetch"), runInput)
			m := fetch.(map[string]any)
			if m["method"] != "GET" || !strings.Contains(m["url"].(string), "123") {
				t.Fatalf("fetch payload: %#v", m)
			}
		}
		notify := eval(t, exprOf(wf, "build-notify"), map[string]any{
			"status": 200, "json": map[string]any{"properties": map[string]any{
				"firstname": "Ada", "lastname": "L", "email": "a@x.io",
			}},
		})
		nm := notify.(map[string]any)
		if !strings.Contains(nm["message"].(string), "Ada") {
			t.Fatalf("notify payload: %#v", nm)
		}
	})

	t.Run("invoice-paid-route", func(t *testing.T) {
		wf := require(t, "example-client/invoice-paid-route.json")
		hi := eval(t, exprOf(wf, "shape"), map[string]any{
			"json": map[string]any{"total": 20000.0, "customer": "Acme"},
		}).(map[string]any)
		if hi["high_value"] != true {
			t.Fatalf("high_value: %#v", hi)
		}
		lo := eval(t, exprOf(wf, "shape"), map[string]any{
			"json": map[string]any{"total": 50.0, "customer": "Acme"},
		}).(map[string]any)
		if lo["high_value"] != false {
			t.Fatalf("high_value: %#v", lo)
		}
		vip := eval(t, exprOf(wf, "shape-vip"), hi).(map[string]any)
		if !strings.Contains(vip["message"].(string), "VIP") {
			t.Fatalf("vip message: %#v", vip)
		}
		// skip_if parity: engine skips shape-vip when high_value == false.
		matched, err := handlers.Compare(lo["high_value"], "==", false)
		if err != nil || !matched {
			t.Fatalf("skip_if parity: %v %v", matched, err)
		}
	})

	t.Run("nightly-sync", func(t *testing.T) {
		wf := require(t, "example-client/nightly-sync.json")
		fetch := eval(t, exprOf(wf, "build-fetch"), nil)
		if fetch.(map[string]any)["method"] != "GET" {
			t.Fatalf("fetch payload: %#v", fetch)
		}
		sum := eval(t, exprOf(wf, "summarize"), map[string]any{
			"json": map[string]any{"results": []any{1.0, 2.0, 3.0}},
		}).(map[string]any)
		if !isThree(sum["count"]) {
			t.Fatalf("count: %#v", sum)
		}
		push := eval(t, exprOf(wf, "build-push"), sum).(map[string]any)
		if push["method"] != "POST" {
			t.Fatalf("push payload: %#v", push)
		}
	})

	t.Run("approval-refund", func(t *testing.T) {
		wf := require(t, "example-client/approval-refund.json")
		review := eval(t, exprOf(wf, "build-review"), map[string]any{
			"order_id": "ord_1", "amount": 42.5, "reason": "dup",
		}).(map[string]any)
		if review["order_id"] != "ord_1" {
			t.Fatalf("review: %#v", review)
		}
		fanIn := map[string]any{
			"approve":      map[string]any{"by": "ops"},
			"build-review": review,
		}
		refund := eval(t, exprOf(wf, "build-refund"), fanIn).(map[string]any)
		if refund["method"] != "POST" {
			t.Fatalf("refund payload: %#v", refund)
		}
	})

	t.Run("ops-dlq-watchdog", func(t *testing.T) {
		wf := require(t, "ops/dlq-watchdog.json")
		alert := eval(t, exprOf(wf, "build-alert"), map[string]any{
			"stream": "DEAD_LETTERS", "depth": 3.0, "threshold": 0, "alert": true,
		}).(map[string]any)
		if alert["webhook_secret"] != "OPS_SLACK_WEBHOOK_URL" {
			t.Fatalf("alert payload: %#v", alert)
		}
		text := alert["payload"].(map[string]any)["text"].(string)
		if !strings.Contains(text, "3") || !strings.Contains(text, "DEAD_LETTERS") {
			t.Fatalf("alert text: %q", text)
		}
		// skip_if parity: empty DLQ skips the alert step.
		matched, err := handlers.Compare(false, "==", false)
		if err != nil || !matched {
			t.Fatalf("skip_if parity: %v %v", matched, err)
		}
	})
}
