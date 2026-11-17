// Command turvez is a read-only router for an agent (or a human) working in
// this repo. It answers "where do I look, and what proves I looked": which
// decision-tree branch applies, which docs matter, what a client's manifest
// declares, and whether the repo's own gates pass.
//
// It deliberately cannot write. There is no `register`, no `run start`, no
// `approve`, no `trigger delete` — those are proposals that a human executes
// (AGENTS.md: "propose the exact command and wait"). A router that could also
// mutate would be a second orchestrator, which is the one thing this repo
// refuses.
//
// The data it serves is the same data the gates read: docs/README.md is the
// index, llms.txt is the task route, skills/turvez/references/decision-tree.md
// is the tree, clients/<c>/client.yaml is the manifest. Nothing here is a copy
// — a copy would drift, and drift is what the gates exist to catch.
//
// Usage:
//
//	turvez investigate "automatizar NF do fornecedor X"   # branch + docs + gates
//	turvez read docs/browser.md                          # a doc, with its gates
//	turvez context clients/example-client                # engine, runtime, access
//	turvez check                                         # the gates, summarised
//	turvez docs                                          # the index, as JSON
//	turvez manifest                                      # everything, for an MCP/CLI wrapper
//
// Every command takes --json.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var out any
	switch cmd {
	case "investigate":
		out, err = investigate(root, args)
	case "read":
		out, err = readDoc(root, args)
	case "context":
		out, err = clientContext(root, args)
	case "check":
		out, err = check(root, args)
	case "docs":
		out, err = docsIndex(root)
	case "manifest":
		out, err = manifest(root)
	case "help", "-h", "--help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command %q (try: investigate, read, context, check, docs)", cmd)
	}
	if err != nil {
		fatal(err)
	}
	if wantJSON(args) {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fatal(err)
		}
		return
	}
	print(out)
}

func usage() {
	fmt.Fprint(os.Stderr, `turvez — read-only router for this repo (it cannot write)

  turvez investigate "<the ask>"   which Q branch applies, docs to read, gates to run
  turvez read <doc>                a doc's purpose + the gate that covers it
  turvez context <client-dir>      engine, runtime, deployment, access, secrets names
  turvez check [--quiet]           run the repo gates, one line each
  turvez docs                      the docs/ index as data
  turvez manifest                  one JSON document: branches + docs + engines

Nothing here registers a workflow, starts a run or approves anything: those are
proposals for a human (AGENTS.md). --json on every command.
`)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "turvez:", err)
	os.Exit(1)
}

func wantJSON(args []string) bool {
	for _, a := range args {
		if a == "--json" {
			return true
		}
	}
	return false
}

// repoRoot walks up from the working directory to the go.mod that names this
// module, so the binary works from a client folder as well as from the root.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if raw, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(raw), "module github.com/calionauta/turvez") {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("not inside the turvez repo (no go.mod naming the module above here)")
		}
		dir = parent
	}
}

// --- investigate -----------------------------------------------------------

// branch is one answer of the decision tree, as the tree states it. The trigger
// words are deliberately coarse: this is a *router*, and a wrong branch that a
// human can see and correct beats a confident answer with no evidence.
type branch struct {
	ID     string   `json:"id"`
	Ask    string   `json:"ask"`
	Answer string   `json:"answer"`
	Doc    string   `json:"doc"`
	Gate   string   `json:"gate"`
	Words  []string `json:"-"`
	// Route is "runtime" for "where does this run" answers; the rest are
	// capability questions that come before a runtime decision.
	Route string `json:"route"`
	// Section is an H2 the doc must still contain. It is the "read when" text,
	// made checkable: if the doc is reorganised, TurvezPoints fails instead of
	// the router pointing at a section that no longer exists.
	Section string `json:"section"`
}

func branches() []branch {
	return []branch{
		{"Q0", "which repo am I in?", "go.mod = public template (no real clients); TURVEZ_PIN = private ops repo", "docs/ops-repo.md", "go test ./clients/", []string{"template", "ops repo", "cliente real", "which repo"}, "runtime", "Which repo am I in?"},
		{"Q1", "which engine?", "Windmill (default) unless a contract criterion says dagnats; Dagu is level 2 only", "docs/client-spine.md#choosing-the-engine", "scripts/test-decisions.sh", []string{"engine", "windmill", "dagnats", "dagu", "orquestrador", "orchestrator"}, "runtime", "Choosing the engine"},
		{"Q1.5", "an agent loop with tools?", "a human watches → Genkit flow; nobody waits → a run in the client's engine", "docs/genkit.md", "scripts/check-agent-template.sh", []string{"agente", "agent", "loop", "llm que escolhe", "tool calling", "genkit"}, "capability", "Where the line runs: flow vs recipe"},
		{"Q1.6", "a library or new dependency?", "stdlib → go.mod → approved capability → propose, confirm, pin", "docs/tooling.md", "scripts/test-decisions.sh", []string{"dependência", "dependencia", "library", "biblioteca", "pacote", "npm", "go get"}, "capability", "The test for any future addition"},
		{"Q1.7", "the data only exists on a page (no API)?", "read (no browser) → capture (mitmproxy/flow) → browser in a Windmill Go node → not an automation", "docs/browser.md", "scripts/test-decisions.sh", []string{"sem api", "no api", "portal", "scraping", "scraper", "página", "pagina", "site", "web"}, "capability", "Q1.7 — answer in this order, stop at the first that works"},
		{"Q1.7.2", "an app, not a page — or 'the logic is in the binary'?", "capture to find the endpoint; signed/spoofed sessions keep the browser; no HTTP to replay = not an automation", "docs/traffic-capture.md", "scripts/test-decisions.sh", []string{"app", "aplicativo", "mobile", "android", "ios", "mitmproxy", "har", "frida", "ghidra", "binwalk", "reverse", "engenharia reversa", "assinatura", "signed"}, "capability", "2. The deep path: REA, Frida, Ghidra, binwalk — we do not do this"},
		{"Q2", "where does it run?", "shared by default; dedicated on regulation, client infra, noisy neighbour or an upgrade window", "docs/deployment-profiles.md", "scripts/verify-isolation.sh", []string{"onde roda", "host", "dedicado", "dedicated", "shared", "isolamento", "kubernetes", "k8s"}, "runtime", "Choosing"},
		{"Q4", "a one-off script?", "level 1 sandbox, zero residue; if it recurs → level 2; if it needs replay/UI → level 3", "docs/script-execution.md", "scripts/test-decisions.sh", []string{"one-off", "script solto", "avulso", "único", "unico", "cron"}, "runtime", "Decision tree (ask in order)"},
		{"Q0.8", "answer questions over our data?", "call the system of record via Nango; documents → route-then-read", "docs/retrieval.md", "scripts/test-decisions.sh", []string{"rag", "buscar", "search", "documento", "document", "resumir", "consulta", "knowledge"}, "capability", "Route, then read (the default shape)"},
		{"Q5", "how do I operate, install or migrate?", "install → first run → extract a client → migrate engines; the operational docs", "docs/README.md", "scripts/test-decisions.sh", []string{"instalar", "install", "primeiro run", "first run", "migrar", "migrate", "extrair", "isolate", "deploy", "onboard"}, "ops", "Operating it"},
	}
}

// hit is one branch plus the words that fired, so the reader can see WHY the
// router chose it and disagree with evidence rather than intuition.
type hit struct {
	branch
	Matched []string `json:"matched"`
}

func normalise(s string) string {
	s = strings.ToLower(s)
	return strings.ReplaceAll(s, "\n", " ")
}

func investigate(root string, args []string) (any, error) {
	var words []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		words = append(words, a)
	}
	ask := strings.TrimSpace(strings.Join(words, " "))
	if ask == "" {
		return nil, errors.New(`missing the ask: turvez investigate "automatizar NF do fornecedor X"`)
	}
	hay := normalise(ask)

	var hits []hit
	for _, b := range branches() {
		var m []string
		for _, w := range b.Words {
			if strings.Contains(hay, normalise(w)) {
				m = append(m, w)
			}
		}
		if len(m) > 0 {
			hits = append(hits, hit{branch: b, Matched: m})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return len(hits[i].Matched) > len(hits[j].Matched) })

	read := ""
	verified := true
	if len(hits) > 0 {
		read = hits[0].Doc
		if !sectionExists(root, hits[0].Doc, hits[0].Section) {
			verified = false
		}
	}

	// The skeleton is what makes this a router rather than a search engine: it
	// names the three reads and the one gate for the branch that fired, in the
	// order they cost you. It still decides nothing — step 0 is the human.
	skeleton := []string{
		"0. confirm the branch with the human (one question, branch-flipping)",
	}
	if read != "" {
		skeleton = append(skeleton,
			"1. turvez read "+read+" --full      # the answer, in prose",
			"2. read the client manifest: clients/<client>/client.yaml",
			"3. run the gate: "+hits[0].Gate)
	}

	result := map[string]any{
		"ask":              ask,
		"repo":             repoKind(root),
		"matches":          hits,
		"read":             read,
		"section_verified": verified,
		"skeleton":         skeleton,
		"note": "coarse word match on purpose — confirm the branch with the human, " +
			"read the doc it names, then run the gate. This command does not decide.",
	}
	if !verified && read != "" {
		result["stale"] = fmt.Sprintf("%s no longer has the section %q that documents this branch: "+
			"the doc was reorganised and the router believes the old shape", read, hits[0].Section)
	}
	if len(hits) == 0 {
		result["fallback"] = branch{"?", "no branch matched", "open skills/turvez/references/decision-tree.md and ask the human one branch-flipping question", "skills/turvez/references/decision-tree.md", "scripts/test-decisions.sh", nil, "capability", ""}
	}
	return result, nil
}

// sectionExists is the answer to "does the 'read when' text still tell the truth
// about where to look?". A gate cannot parse a sentence, but it can prove the
// section the sentence points at is still there — which is the part that breaks
// when a doc is reorganised. Route words stay unverified; honesty about that is
// cheaper than a check that will never run.
func sectionExists(root, doc, section string) bool {
	if section == "" {
		return false
	}
	path := doc
	if !strings.HasPrefix(path, "docs/") {
		path = "docs/" + path
	}
	if i := strings.Index(path, "#"); i >= 0 {
		path = path[:i]
	}
	body, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		if plainHeading(strings.TrimPrefix(line, "## ")) == plainHeading(section) {
			return true
		}
	}
	return false
}

// plainHeading drops the decoration (backticks, bold, em dashes around a list)
// so a heading can be restyled without the router claiming the doc moved.
func plainHeading(h string) string {
	h = strings.NewReplacer("**", "", "`", "", "\u2014", "-", "\u2013", "-").Replace(h)
	h = strings.Join(strings.Fields(h), " ")
	return strings.ToLower(strings.TrimSpace(h))
}

func repoKind(root string) string {
	if _, err := os.Stat(filepath.Join(root, "TURVEZ_PIN")); err == nil {
		return "ops (private: real clients)"
	}
	return "template (public: no real clients)"
}

// --- read ------------------------------------------------------------------

// indexRow is one row of docs/README.md — the single place that says what a doc
// is for and which gate keeps it honest.
type indexRow struct {
	Doc     string `json:"doc"`
	Purpose string `json:"purpose"`
	Gate    string `json:"gate"`
	Section string `json:"section"`
}

var indexRowRe = regexp.MustCompile("^\\|\\s*`([a-z0-9-]+\\.md)`\\s*\\|\\s*(.+?)\\s*\\|\\s*(.+?)\\s*\\|\\s*$")

func docsIndex(root string) (any, error) {
	raw, err := os.ReadFile(filepath.Join(root, "docs", "README.md"))
	if err != nil {
		return nil, fmt.Errorf("docs/README.md is the index; %w", err)
	}
	var rows []indexRow
	section := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		m := indexRowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rows = append(rows, indexRow{Doc: m[1], Purpose: m[2], Gate: m[3], Section: section})
	}
	if len(rows) == 0 {
		return nil, errors.New("docs/README.md has no index rows (expected `| `doc.md` | when | gate |`)")
	}
	return rows, nil
}

func readDoc(root string, args []string) (any, error) {
	var target string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			target = a
			break
		}
	}
	if target == "" {
		return nil, errors.New("missing the doc: turvez read docs/browser.md (or: browser.md)")
	}
	base := filepath.Base(target)
	if !strings.HasSuffix(base, ".md") {
		base += ".md"
	}
	path := filepath.Join(root, "docs", base)
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no such doc: docs/%s (turvez docs lists them)", base)
	}
	rows, err := docsIndex(root)
	if err != nil {
		return nil, err
	}
	var row *indexRow
	for i, r := range rows.([]indexRow) {
		if r.Doc == base {
			row = &rows.([]indexRow)[i]
			break
		}
	}
	full := false
	for _, a := range args {
		if a == "--full" || a == "-f" {
			full = true
		}
	}
	out := map[string]any{
		"doc":      "docs/" + base,
		"title":    firstHeading(string(body)),
		"lines":    strings.Count(string(body), "\n") + 1,
		"headings": headings(string(body)),
		"purpose":  nil,
		"gate":     nil,
	}
	if row != nil {
		out["purpose"] = row.Purpose
		out["gate"] = row.Gate
	}
	if full || !wantJSON(args) {
		out["body"] = string(body)
	}
	return out, nil
}

// headings lists the H2s: enough for an agent to decide which section to read
// without paying for the whole file.
func headings(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "## ")))
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

// --- context ---------------------------------------------------------------

// manifest is the flattened client.yaml the repo's own tests read the same way
// (clients/recipes_test.go:parseManifest): keys are trimmed, inline comments
// stripped, quotes removed. It is flat on purpose — a deep YAML parser would be
// a dependency, and a dependency for a read-only viewer is not worth it.
func parseManifest(raw []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if i := strings.Index(value, "#"); i >= 0 {
			value = strings.TrimSpace(value[:i])
		}
		value = strings.Trim(value, `"'`)
		if key != "" {
			out[key] = value
		}
	}
	return out
}

// secretNameRe finds a secret NAME — never a value. It matches both shapes the
// repo uses: a JSON key (`"webhook_secret": "ACME_URL"`) and a gojq expression
// (`auth_secret: "ACME_TOKEN"`), which is how the integration steps carry it.
// ANCHORED at a word start so `webhook_secret` (a JSON key) and
// `auth_secret:` (inside a gojq expression) both match, while a value that
// merely ends in "secret" does not.
// The optional backslashes cover the gojq case, where the expression is a JSON
// string and the inner quotes arrive escaped (`auth_secret: \"NAME\"`).
var secretNameRe = regexp.MustCompile(`[a-z_]*secret[a-z_]*"?\s*:\s*\\?"([A-Z][A-Z0-9_]{2,})\\?"`)

func clientContext(root string, args []string) (any, error) {
	var target string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			target = a
			break
		}
	}
	if target == "" {
		return nil, errors.New("missing the client: turvez context clients/example-client")
	}
	dir := target
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, target)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "client.yaml"))
	if err != nil {
		return nil, fmt.Errorf("%s has no client.yaml (that file is the manifest; %w)", target, err)
	}
	m := parseManifest(raw)

	// Recipes declared in git, per engine folder. Listed, never loaded: reading
	// a recipe is `read`, running one is a human's call.
	recipeDirs := map[string][]string{}
	if entries, err := os.ReadDir(filepath.Join(dir, "automation")); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			var files []string
			_ = filepath.Walk(filepath.Join(dir, "automation", e.Name()), func(p string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				switch filepath.Ext(p) {
				case ".json", ".yaml", ".yml", ".go":
					rel, _ := filepath.Rel(dir, p)
					files = append(files, rel)
				}
				return nil
			})
			sort.Strings(files)
			recipeDirs[e.Name()] = files
		}
	}

	// Secret NAMES referenced by the recipes: the repo's rule is names in JSON,
	// values in env/age, so surfacing the names is safe and useful. It never
	// reads env/ or secrets/, which may hold ciphertext or values.
	var names []string
	seen := map[string]bool{}
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Ext(p) != ".json" {
			return nil
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for _, mm := range secretNameRe.FindAllStringSubmatch(string(body), -1) {
			if !seen[mm[1]] {
				seen[mm[1]] = true
				names = append(names, mm[1])
			}
		}
		return nil
	})
	sort.Strings(names)
	if names == nil {
		names = []string{}
	}

	return map[string]any{
		"client":       m["client"],
		"engine":       m["engine"],
		"runtime":      m["runtime"],
		"data_class":   m["data_class"],
		"owner":        m["owner"],
		"target":       pick(m, "base_url", "workspace", "nats_url", "api_url", "host"),
		"deployment":   pick(m, "windmill", "nango"),
		"access":       pick(m, "enabled", "plane", "surface", "identity", "policy", "vendor_account"),
		"secret_names": names,
		"automation":   recipeDirs,
		"docs":         []string{"docs/client-spine.md", "docs/secrets.md", "docs/observability.md"},
		"note":         "read-only view of client.yaml; `plane: shared` is a pilot for data_class: normal only",
	}, nil
}

func pick(m map[string]string, keys ...string) map[string]string {
	out := map[string]string{}
	for _, k := range keys {
		if v, ok := m[k]; ok && v != "" {
			out[k] = v
		}
	}
	return out
}

// --- manifest --------------------------------------------------------------

// manifest is the whole router as one JSON document. It exists so a wrapper
// (MCP server, editor plugin, another agent) can consume the router without
// shelling out per question — and so we do not have to ship an MCP server to be
// machine-readable. It is deliberately data only: no verbs, no side effects.
func manifest(root string) (any, error) {
	docs, err := docsIndex(root)
	if err != nil {
		return nil, err
	}
	engines := map[string]any{}
	if raw, err := os.ReadFile(filepath.Join(root, "engines.json")); err == nil {
		_ = json.Unmarshal(raw, &engines)
	}
	clients := []string{}
	if entries, err := os.ReadDir(filepath.Join(root, "clients")); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), "_") {
				clients = append(clients, "clients/"+e.Name())
			}
		}
	}
	sort.Strings(clients)
	return map[string]any{
		"schema":    "turvez/manifest/1",
		"repo":      repoKind(root),
		"engines":   engines,
		"branches":  branches(),
		"docs":      docs,
		"clients":   clients,
		"verbs":     []string{"investigate", "read", "context", "check", "docs", "manifest"},
		"read_only": true,
		"note": "no write verb exists on purpose: register/run/approve are proposals " +
			"for a human (AGENTS.md). `turvez investigate` prints its own refusal.",
	}, nil
}

// --- check -----------------------------------------------------------------

type gate struct {
	Name string `json:"name"`
	Cmd  string `json:"cmd"`
	OK   bool   `json:"ok"`
	Out  string `json:"out,omitempty"`
}

func check(root string, args []string) (any, error) {
	for _, a := range args {
		if a == "--quiet" || a == "-q" {
			break
		}
	}
	gates := []struct{ name, cmd string }{
		{"validate", "make validate"},
		{"go-test", "go test ./..."},
		{"build-turvez", "go build -o bin/turvez ./cmd/turvez"},
	}
	var out []gate
	failed := false
	for _, g := range gates {
		c := exec.Command("sh", "-c", g.cmd)
		c.Dir = root
		body, err := c.CombinedOutput()
		res := gate{Name: g.name, Cmd: g.cmd, OK: err == nil}
		if !res.OK {
			failed = true
			res.Out = tail(string(body), 20)
		}
		out = append(out, res)
	}
	result := map[string]any{"gates": out, "ok": !failed}
	if !wantJSON(args) {
		for _, g := range out {
			mark := "ok  "
			if !g.OK {
				mark = "FAIL"
			}
			fmt.Printf("%s %s  (%s)\n", mark, g.Name, g.Cmd)
			if !g.OK && g.Out != "" {
				fmt.Println(g.Out)
			}
		}
	}
	if failed && !wantJSON(args) {
		os.Exit(1)
	}
	return result, nil
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// --- output ----------------------------------------------------------------

func print(v any) {
	switch t := v.(type) {
	case []indexRow:
		section := ""
		for _, r := range t {
			if r.Section != section {
				section = r.Section
				fmt.Printf("\n%s\n", section)
			}
			fmt.Printf("  %-24s %s\n  %-24s gate: %s\n", r.Doc, r.Purpose, "", r.Gate)
		}
		fmt.Println()
	case map[string]any:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(t)
	default:
		fmt.Println(v)
	}
}
