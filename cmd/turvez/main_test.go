package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The router's whole promise is "read-only, and it tells the truth about this
// repo". These tests pin the two halves: the index is complete, and the router
// never grows a write verb.

func repoRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test")
		}
		dir = parent
	}
}

// TestDocsIndexCoversEveryDoc is the same contract scripts/test-decisions.sh
// enforces, kept here too so `go test` alone catches a doc nobody indexed.
func TestDocsIndexCoversEveryDoc(t *testing.T) {
	root := repoRootForTest(t)
	got, err := docsIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	rows := got.([]indexRow)
	indexed := map[string]bool{}
	for _, r := range rows {
		indexed[r.Doc] = true
	}
	files, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		base := filepath.Base(f)
		if base == "README.md" {
			continue
		}
		if !indexed[base] {
			t.Errorf("docs/%s has no row in docs/README.md (an unindexed doc is unread)", base)
		}
	}
	for _, r := range rows {
		if _, err := os.Stat(filepath.Join(root, "docs", r.Doc)); err != nil {
			t.Errorf("docs/README.md indexes docs/%s, which does not exist", r.Doc)
		}
		if r.Gate == "" {
			t.Errorf("docs/%s has an empty gate column", r.Doc)
		}
	}
}

// TestReadDocReportsGate: a doc is only worth reading if the reader knows what
// proves it, so `read` must never return a doc without one.
func TestReadDocReportsGate(t *testing.T) {
	root := repoRootForTest(t)
	out, err := readDoc(root, []string{"browser.md", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["gate"] == nil || m["gate"] == "" {
		t.Fatal("docs/browser.md read without a gate: the index row was not found")
	}
	if m["purpose"] == nil || m["purpose"] == "" {
		t.Fatal("docs/browser.md read without a purpose")
	}
	if _, ok := m["body"]; ok {
		t.Fatal("--json without --full must not return the body (an agent pays for what it asked)")
	}
	full, err := readDoc(root, []string{"browser.md", "--json", "--full"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := full.(map[string]any)["body"]; !ok {
		t.Fatal("--full must return the body")
	}
}

// TestInvestigateRoutesQ17 keeps the routing honest for the ask that motivated
// the doc: a target with no API must land on Q1.7, not on a runtime question.
func TestInvestigateRoutesQ17(t *testing.T) {
	root := repoRootForTest(t)
	for _, ask := range []string{
		"portal do fornecedor, sem api",
		"scraping de tabela de preços num site",
		"aplicativo mobile sem api, precisa de mitmproxy",
	} {
		out, err := investigate(root, []string{ask})
		if err != nil {
			t.Fatal(err)
		}
		matches := out.(map[string]any)["matches"].([]hit)
		if len(matches) == 0 {
			t.Fatalf("%q matched no branch", ask)
		}
		first := matches[0].ID
		if !strings.HasPrefix(first, "Q1.7") {
			t.Errorf("%q routed to %s, want Q1.7*", ask, first)
		}
	}
}

// TestInvestigateSaysItDoesNotDecide: the note is the difference between a
// router and an oracle. A model reading this output must see the human gate.
func TestInvestigateSaysItDoesNotDecide(t *testing.T) {
	root := repoRootForTest(t)
	out, err := investigate(root, []string{"preciso de um agente que usa tools"})
	if err != nil {
		t.Fatal(err)
	}
	note, _ := out.(map[string]any)["note"].(string)
	if !strings.Contains(note, "does not decide") {
		t.Fatalf("investigate note lost its refusal: %q", note)
	}
}

// TestClientContextOnlyReadsNames: the manifest view must surface secret NAMES
// (fine in git) and never a VALUE. A value would be a leak in the tool output.
func TestClientContextOnlyReadsNames(t *testing.T) {
	root := repoRootForTest(t)
	out, err := clientContext(root, []string{"clients/example-client"})
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	names, _ := m["secret_names"].([]string)
	if len(names) == 0 {
		t.Fatal("example-client declares secrets in its recipes; none were found")
	}
	for _, n := range names {
		if n != strings.ToUpper(n) || strings.Contains(n, " ") {
			t.Errorf("%q is not a secret NAME (values never leave env/age)", n)
		}
	}
	raw, _ := json.Marshal(m)
	for _, bad := range []string{"password", "sk_live", "Bearer "} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("client context leaked %q", bad)
		}
	}
	if m["engine"] != "dagnats" {
		t.Errorf("engine = %v, want dagnats (the example is dagnats on purpose)", m["engine"])
	}
}

// TestNoWriteVerb is the invariant an agent must not be able to talk itself out
// of: there is no command that mutates. Adding one fails here first.
func TestNoWriteVerb(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, verb := range []string{"register", "run start", "approve", "trigger delete", "cancel"} {
		if strings.Contains(body, `case "`+verb+`"`) {
			t.Errorf("cmd/turvez grew a write verb %q: proposals belong to a human (AGENTS.md)", verb)
		}
	}
	// And it must not import an executor: the router answers, it does not run.
	// Only the import block counts — the doc comments name dagnats on purpose.
	imports := importBlock(t, body)
	for _, bad := range []string{"internal/handlers", "internal/opshttp", "dagnats", "internal/access"} {
		if imports[bad] {
			t.Errorf("cmd/turvez imports %q: a router that can execute is a second orchestrator", bad)
		}
	}
	if len(imports) == 0 {
		t.Fatal("could not read the import block; the invariant test is not testing anything")
	}
}

// importBlock returns the imported paths of a Go source file, parsed from the
// import group only, so prose in a comment cannot trigger the invariant.
func importBlock(t *testing.T, src string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	inBlock := false
	for _, line := range strings.Split(src, "\n") {
		tl := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(tl, "import ("):
			inBlock = true
			continue
		case inBlock && tl == ")":
			inBlock = false
			continue
		case inBlock:
			if p := strings.Trim(tl, "\"`"); p != "" {
				out[p] = true
			}
		}
	}
	return out
}

// TestTurvezPoints is the answer to "does the 'read when' text still tell the
// truth?": every branch must name a doc that exists AND an H2 that doc still
// has. A doc can be reorganised; when it is, this fails instead of the router
// silently pointing at a section somebody deleted.
func TestTurvezPoints(t *testing.T) {
	root := repoRootForTest(t)
	for _, b := range branches() {
		if b.Section == "" {
			t.Errorf("%s has no Section: the router cannot prove its doc still says what it claims", b.ID)
			continue
		}
		if !sectionExists(root, b.Doc, b.Section) {
			t.Errorf("%s points at %s §%q, which is not a heading there", b.ID, b.Doc, b.Section)
		}
		if b.Gate == "" {
			t.Errorf("%s has no gate: an answer nobody can check is an opinion", b.ID)
		}
		// The gate must be a real thing a reader can run.
		gate := strings.Fields(b.Gate)
		if len(gate) == 0 {
			continue
		}
		if !regexp.MustCompile(`^(scripts/|make |go |dagnats )`).MatchString(b.Gate) {
			t.Errorf("%s gate %q is not a command in this repo", b.ID, b.Gate)
		}
	}
}

// TestInvestigateIsHonestAboutStaleness: when a doc loses its section, the
// answer must SAY so rather than report a clean routing.
func TestInvestigateIsHonestAboutStaleness(t *testing.T) {
	root := repoRootForTest(t)
	out, err := investigate(root, []string{"app mobile android"})
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["read"] == "" {
		t.Fatal("no doc routed for an app-with-no-API ask")
	}
	if _, stale := m["stale"]; stale && m["section_verified"] == true {
		t.Fatal("reported both stale and verified")
	}
	if sk, _ := m["skeleton"].([]string); len(sk) < 4 {
		t.Fatalf("skeleton too short (%d): it must name confirm, doc, manifest, gate", len(sk))
	}
}

// TestManifestIsData: a wrapper may depend on this shape, so it is pinned — and
// it must never grow a verb that mutates.
func TestManifestIsData(t *testing.T) {
	root := repoRootForTest(t)
	out, err := manifest(root)
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["schema"] != "turvez/manifest/1" {
		t.Errorf("schema = %v", m["schema"])
	}
	if m["read_only"] != true {
		t.Error("manifest claims to be writable")
	}
	verbs, _ := m["verbs"].([]string)
	for _, v := range verbs {
		switch v {
		case "register", "run", "approve", "delete":
			t.Errorf("manifest advertises write verb %q", v)
		}
	}
	if len(m["branches"].([]branch)) == 0 || len(m["docs"].([]indexRow)) == 0 {
		t.Error("manifest without data")
	}
	// Round-trips through JSON: a wrapper parses it, not Go structs.
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back["schema"] != "turvez/manifest/1" {
		t.Error("manifest does not round-trip")
	}
}
