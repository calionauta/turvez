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

import (
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

func clientDirs(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, root := range []string{"../clients", "../workflows"} {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || e.Name() == "ops" {
				continue
			}
			out = append(out, filepath.Join(root, e.Name()))
		}
	}
	return out
}

// clientFromPath returns the slug in clients/<slug>/... or workflows/<slug>/...
func clientFromPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, p := range parts {
		if (p == "clients" || p == "workflows") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

var (
	// The reference usually sits inside a JSON string (jq expression), so the
	// quotes around the header name are backslash-escaped.
	connIDRe    = regexp.MustCompile(`(?i)Connection-Id\\?"?\s*[:=]\s*\\?"?([A-Za-z0-9:_-]+)`)
	providerRe  = regexp.MustCompile(`(?i)Provider-Config-Key\\?"?\s*[:=]\s*\\?"?([A-Za-z0-9:_-]+)`)
	slugShapeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

// TestNangoConnectionIsTheClientSlug rejects a hardcoded Connection-Id that is
// not the client the file lives under. A shared id is how one client's flow
// starts reading another client's data.
func TestNangoConnectionIsTheClientSlug(t *testing.T) {
	checked := 0
	for _, dir := range clientDirs(t) {
		slug := filepath.Base(dir)
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
				want := slug
				// <client>:<subaccount> is the documented multi-account form.
				if got != want && !strings.HasPrefix(got, want+":") {
					t.Errorf("%s: Connection-Id %q must be %q or %q:<subaccount> (docs/nango.md)",
						path, got, want, want)
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
	files, _ := filepath.Glob("../env/*.env.example")
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
