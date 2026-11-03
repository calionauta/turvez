package clients

// Guard for docs/nango.md: provider credentials live in Nango, and a connection
// is addressed by the client slug, never by a shared name.
//
// Three ways a copy-pasted integration leaks across tenants, all caught here:
//  1. a provider token in env/<client>.env.example — it would be rotated by hand
//     and break on refresh;
//  2. a Connection-Id that is not the client the code lives under — client A's
//     flow reading client B's mailbox;
//  3. a Provider-Config-Key carrying a client prefix, so the same provider is
//     registered N times and a renewal fixes only one of them.
//
// Both checks follow WORKFLOWS_DIR, so reviewing a private ops repo runs them
// over that repo's clients and env examples instead of only the template's.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// saasProviders are the vendors Nango owns a connection for. The list is
// explicit on purpose: a blanket "*_TOKEN" rule would reject a client's own
// internal API token, which is a legitimate static secret.
var saasProviders = []string{
	"hubspot", "slack", "google", "gmail", "gcalendar", "gdrive", "gsheets",
	"salesforce", "notion", "jira", "confluence", "asana", "monday", "trello",
	"clickup", "pipedrive", "zendesk", "intercom", "freshdesk", "mailchimp",
	"shopify", "stripe", "quickbooks", "xero", "dropbox", "airtable",
	"calendly", "docusign", "twilio", "discord", "linkedin", "zoom",
}

// credSuffix marks a name as a credential rather than an endpoint or a flag.
var credSuffix = []string{"_TOKEN", "_API_KEY", "_SECRET", "_CLIENT_SECRET", "_OAUTH", "_REFRESH_TOKEN"}

// endpointSuffix is a static secret we own: an incoming webhook URL, a signing
// key. Nango cannot refresh or revoke it, so it stays in env/age.
var endpointSuffix = []string{"_WEBHOOK_URL", "_WEBHOOK_SECRET", "_SIGNING_KEY"}

// clientDirs lists the client folders of the checkout under test. It reuses
// clientRoots() (recipes_test.go) so a WORKFLOWS_DIR override validates the
// private ops repo, not just the public template sitting next to the tests.
func clientDirs(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	var out []string
	for _, root := range clientRoots() {
		entries, err := filepath.Glob(filepath.Join(root, "*"))
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range entries {
			fi, err := os.Stat(d)
			if err != nil || !fi.IsDir() {
				continue
			}
			base := filepath.Base(d)
			if strings.HasPrefix(base, "_") || base == "ops" {
				continue
			}
			abs, err := filepath.Abs(d)
			if err != nil {
				t.Fatal(err)
			}
			if seen[abs] {
				continue
			}
			seen[abs] = true
			out = append(out, d)
		}
	}
	return out
}

// envExampleRoots lists the directories that may hold env/*.env.example.
// When WORKFLOWS_DIR points at an ops repo, the examples live beside its
// clients/ directory; a guard that only read ../env would miss every real
// client and pass a scaffold that still ships a provider token.
func envExampleRoots() []string {
	seen := map[string]bool{}
	var out []string
	add := func(dir string) {
		fi, err := os.Stat(dir)
		if err != nil || !fi.IsDir() {
			return
		}
		abs, err := filepath.Abs(dir)
		if err != nil || seen[abs] {
			return
		}
		seen[abs] = true
		out = append(out, dir)
	}
	add(filepath.Join("..", "env"))
	if os.Getenv("WORKFLOWS_DIR") != "" {
		opsRoot := filepath.Dir(filepath.Clean(resolveRoot(workflowsRoot())))
		add(filepath.Join(opsRoot, "env"))
	}
	if ops := os.Getenv("TURVEZ_OPS_DIR"); ops != "" {
		add(filepath.Join(ops, "env"))
	}
	return out
}

var (
	// The reference usually sits inside a JSON string (jq expression), so the
	// quotes around the header name are backslash-escaped.
	connIDRe    = regexp.MustCompile(`(?i)Connection-Id\\?"?\s*[:=]\s*\\?"?([A-Za-z0-9:_-]+)`)
	providerRe  = regexp.MustCompile(`(?i)Provider-Config-Key\\?"?\s*[:=]\s*\\?"?([A-Za-z0-9:_-]+)`)
	slugShapeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

// recordedConnectionIDs reads clients/<client>/nango/connections.json: the ids
// Nango assigned to connections that were created through Connect UI. It is the
// one legitimate way a literal id differs from the client slug — Connect UI
// generates the id (a UUID) and Nango cannot rename a connection, so recording
// it is better than pretending the slug rule holds there. The file holds ids,
// not secrets, so it is committed.
//
// It lives in nango/, not at the client root: a client-root *.json is treated
// as a legacy flat-layout recipe by scripts/validate.sh, and a metadata file
// would be validated as a workflow.
func recordedConnectionIDs(t *testing.T, dir string) map[string]bool {
	t.Helper()
	path := filepath.Join(dir, "nango", "connections.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var doc struct {
		Connections map[string]string `json:"connections"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v (want {\"connections\": {\"hubspot\": \"<id>\"}})", path, err)
	}
	out := make(map[string]bool, len(doc.Connections))
	for provider, id := range doc.Connections {
		if id == "" {
			t.Errorf("%s: %q has an empty id; drop the entry instead", path, provider)
			continue
		}
		out[id] = true
	}
	return out
}

// connectionIDReason returns why id is not acceptable for a file that belongs to
// slug, or "" when it is. slugs is every client in the checkout, so an id that
// names another client is reported as a cross-tenant read instead of a typo.
func connectionIDReason(slug, id string, slugs []string, recorded map[string]bool) string {
	// <client>:<subaccount> is the documented multi-account form.
	if id == slug || strings.HasPrefix(id, slug+":") || recorded[id] {
		return ""
	}
	for _, other := range slugs {
		if other != slug && (id == other || strings.HasPrefix(id, other+":")) {
			return fmt.Sprintf("belongs to client %q; a flow must read its own client's connection (docs/nango.md)", other)
		}
	}
	return fmt.Sprintf("must be %q, %q:<subaccount>, or an id recorded in clients/%s/nango/connections.json (docs/nango.md)",
		slug, slug, slug)
}

// TestConnectionIDRules pins the three-way rule without needing a checkout: the
// slug form, the recorded UUID Connect UI hands out, and the two rejections.
func TestConnectionIDRules(t *testing.T) {
	slugs := []string{"acme", "beta"}
	recorded := map[string]bool{"3f2b6c1e-0000-4000-8000-000000000000": true}
	cases := []struct {
		id   string
		want string
	}{
		{"acme", ""},
		{"acme:billing", ""},
		{"3f2b6c1e-0000-4000-8000-000000000000", ""},
		{"beta", `belongs to client "beta"`},
		{"beta:billing", `belongs to client "beta"`},
		{"shared", "must be"},
		{"", "must be"},
	}
	for _, c := range cases {
		got := connectionIDReason("acme", c.id, slugs, recorded)
		switch {
		case c.want == "" && got != "":
			t.Errorf("id %q: unexpected rejection: %s", c.id, got)
		case c.want != "" && !strings.Contains(got, c.want):
			t.Errorf("id %q: reason %q does not contain %q", c.id, got, c.want)
		}
	}
}

// TestNangoConnectionIsTheClientSlug rejects a hardcoded Connection-Id that is
// not the client the file lives under. A shared id is how one client's flow
// starts reading another client's data.
//
// Allowed for a file under clients/<client>/:
//   - <client>, or <client>:<subaccount> (the multi-account form);
//   - an id listed in that client's nango/connections.json, which is where the UUID
//     Connect UI generates is recorded.
//
// Rejected: another client's slug (a cross-tenant read), and anything else.
func TestNangoConnectionIsTheClientSlug(t *testing.T) {
	dirs := clientDirs(t)
	slugs := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		slugs[filepath.Base(dir)] = true
	}
	slugsList := make([]string, 0, len(slugs))
	for s := range slugs {
		slugsList = append(slugsList, s)
	}

	checked := 0
	for _, dir := range dirs {
		slug := filepath.Base(dir)
		recorded := recordedConnectionIDs(t, dir)
		for id := range recorded {
			if !slugs[id] || id == slug {
				continue
			}
			t.Errorf("%s/nango/connections.json: id %q is another client's slug; ids are per-client (docs/nango.md)", dir, id)
		}
		walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			switch filepath.Ext(path) {
			case ".json", ".yaml", ".yml", ".go":
			default:
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(raw)
			for _, m := range connIDRe.FindAllStringSubmatch(text, -1) {
				checked++
				got := m[1]
				if reason := connectionIDReason(slug, got, slugsList, recorded); reason != "" {
					t.Errorf("%s: Connection-Id %q %s", path, got, reason)
				}
			}
			for _, m := range providerRe.FindAllStringSubmatch(text, -1) {
				got := m[1]
				if !slugShapeRe.MatchString(got) {
					t.Errorf("%s: Provider-Config-Key %q must be the lowercase provider slug (e.g. hubspot), one integration per provider (docs/nango.md)",
						path, got)
				}
				if strings.Contains(got, slug) {
					t.Errorf("%s: Provider-Config-Key %q carries the client name; register one integration per provider and separate tenants by Connection-Id (docs/nango.md)",
						path, got)
				}
			}
			return nil
		})
		if walkErr != nil {
			t.Fatal(walkErr)
		}
	}
	t.Logf("checked %d Connection-Id reference(s)", checked)
}

// TestProviderCredentialsLiveInNango rejects a SaaS credential in an env
// example: it must be a Nango connection instead.
func TestProviderCredentialsLiveInNango(t *testing.T) {
	var files []string
	for _, dir := range envExampleRoots() {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.env.example"))
		files = append(files, matches...)
	}
	if len(files) == 0 {
		t.Skip("no env examples in this checkout (ops repo keeps them too)")
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			name := strings.TrimSpace(strings.SplitN(line, "=", 2)[0])
			if strings.HasPrefix(name, "NANGO_") {
				continue
			}
			upper := strings.ToUpper(name)
			for _, suffix := range endpointSuffix {
				if strings.HasSuffix(upper, suffix) {
					name = "" // a webhook URL / signing key we own: static on purpose
					break
				}
			}
			if name == "" {
				continue
			}
			isCred := false
			for _, suffix := range credSuffix {
				if strings.HasSuffix(upper, suffix) {
					isCred = true
					break
				}
			}
			if !isCred {
				continue
			}
			for _, provider := range saasProviders {
				if !strings.Contains(strings.ToLower(name), provider) {
					continue
				}
				t.Errorf("%s: %s is a %s credential; register the client in Nango and keep only NANGO_HOST + NANGO_SECRET_KEY (docs/nango.md). See: ./scripts/nango-connect.sh <client> --provider %s",
					f, name, provider, provider)
			}
		}
	}
}
