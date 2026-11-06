package clients

// Guard for docs/access-plane.md: the access plane (the client's own agents
// calling authenticated tools) is declared in client.yaml, capped by a ceiling
// in catalog/policy.yaml, and never carries a credential.
//
// What a checkout can prove without a broker running:
//
//  1. the `access:` block agrees with itself and with data_class — a sensitive
//     client is client-hosted, a cloud plane names the client's own account;
//  2. the ceiling exists, parses, and cannot offer a write with no approval;
//  3. every provider the client already uses through Nango is inside the
//     ceiling — you cannot grant what you have not declared;
//  4. no credential value sits in the ceiling or in the manifest.
//
// The broker-side rules (a grant outside the ceiling is rejected server-side,
// revocation stops the next tool call) land with the broker in P2; they are
// stated in docs/access-plane.md and are not claimable from a checkout alone.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calionauta/turvez/internal/access"
)

// embeddedSecretPatterns are value shapes that must never reach git. Shared by
// TestRecipeStructure (recipes) and the access checks (manifest + ceiling): one
// list, so a new provider shape is caught everywhere at once.
var embeddedSecretPatterns = []string{
	"hooks.slack.com/services/", "xoxb-", "xoxp-", "sk-live-",
	"sk-test-", "ghp_", "-----BEGIN",
}

// connectionProviders reads the provider keys of clients/<client>/nango/
// connections.json: the connections the broker can actually hand out.
func connectionProviders(t *testing.T, dir string) []string {
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
		t.Fatalf("%s: %v", path, err)
	}
	out := make([]string, 0, len(doc.Connections))
	for provider := range doc.Connections {
		out = append(out, provider)
	}
	return out
}

// recipeProviders lists the Provider-Config-Key values every recipe of the
// client already calls through the Nango proxy. They are real usage, so the
// ceiling has to cover them.
func recipeProviders(t *testing.T, dir string) []string {
	t.Helper()
	seen := map[string]bool{}
	root := filepath.Join(dir, "automation")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range providerRe.FindAllStringSubmatch(string(raw), -1) {
			if m[1] != "" {
				seen[m[1]] = true
			}
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	out := make([]string, 0, len(seen))
	for provider := range seen {
		out = append(out, provider)
	}
	return out
}

// TestAccessPlane validates every client that turns the plane on.
func TestAccessPlane(t *testing.T) {
	enabled := 0
	for _, dir := range clientDirs(t) {
		slug := filepath.Base(dir)
		manifestPath := filepath.Join(dir, "client.yaml")
		raw, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // phase-1 folder; TestClientManifests owns that complaint
		}
		m, err := access.ParseManifest(raw)
		if err != nil {
			t.Errorf("%s: %v", manifestPath, err)
			continue
		}
		if !m.Access.Enabled {
			continue
		}
		enabled++
		t.Run(slug, func(t *testing.T) {
			if err := m.Validate(); err != nil {
				t.Fatalf("%s: %v", manifestPath, err)
			}

			policyPath := filepath.Join(dir, m.Access.PolicyPath)
			policyRaw, err := os.ReadFile(policyPath)
			if err != nil {
				t.Fatalf("%s: the plane is enabled but the ceiling is unreadable: %v", policyPath, err)
			}
			for _, p := range embeddedSecretPatterns {
				if strings.Contains(string(policyRaw), p) {
					t.Fatalf("%s: looks like a credential value (%q); the ceiling holds provider names and scopes, never tokens", policyPath, p)
				}
			}
			policy, err := access.ParsePolicy(policyRaw)
			if err != nil {
				t.Fatalf("%s: %v", policyPath, err)
			}
			if err := policy.Validate(); err != nil {
				t.Fatalf("%s: %v", policyPath, err)
			}
			for name := range policy.Providers {
				if strings.Contains(name, slug) {
					t.Errorf("%s: provider %q carries the client name; the ceiling is one entry per provider, tenants are separate (docs/nango.md)", policyPath, name)
				}
			}

			providers := append(connectionProviders(t, dir), recipeProviders(t, dir)...)
			if err := access.Coverage(policy, providers); err != nil {
				t.Errorf("%s: %v", policyPath, err)
			}
			t.Logf("%s: plane %s, %d provider(s) in the ceiling", slug, m.Access.Plane, len(policy.Providers))
		})
	}
	if enabled == 0 && os.Getenv("WORKFLOWS_DIR") == "" {
		// The template ships the plane off, so a template checkout with no
		// enabled client means the contract was never exercised — the fixture
		// exists for that. An ops repo under test may legitimately have no
		// client on the access plane yet.
		t.Fatal("no client enables the access plane; clients/example-client is the worked example and must stay enabled")
	}
}

// TestAccessInvariantsAreRedProofed keeps the manual red-proof automated. Each
// case takes the real fixture, applies one mutation, and requires the error a
// broken checkout would produce — so a rule that gets loosened (or a contract
// test that stops calling it) fails here instead of turning the guarantee back
// into documentation.
func TestAccessInvariantsAreRedProofed(t *testing.T) {
	dir := "example-client"
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "client.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	policyRaw, err := os.ReadFile(filepath.Join(dir, "catalog", "policy.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	// The untouched fixture must pass; otherwise every mutation below would
	// "fail" for the wrong reason.
	base, err := access.ParseManifest(manifestRaw)
	if err != nil || base.Validate() != nil {
		t.Fatalf("fixture does not pass its own rules: %v %v", err, base.Validate())
	}
	basePolicy, err := access.ParsePolicy(policyRaw)
	if err != nil || basePolicy.Validate() != nil {
		t.Fatalf("ceiling does not pass its own rules: %v %v", err, basePolicy.Validate())
	}
	providers := append(connectionProviders(t, dir), recipeProviders(t, dir)...)
	if len(providers) == 0 {
		t.Fatal("the fixture declares no provider usage, so invariant 3 would be checked against nothing")
	}

	cases := []struct {
		name           string
		manifest       func(string) string
		policy         func(string) string
		extraProviders []string
		want           string
	}{
		{
			name:   "invariant 2: write with no approval",
			policy: func(s string) string { return strings.Replace(s, "\n    approval: writes", "\n    approval: none", 1) },
			want:   "invariant 2",
		},
		{
			name:           "invariant 3: provider used but not declared",
			extraProviders: []string{"slack"},
			want:           "not in the catalog ceiling",
		},
		{
			name:     "invariant 7: sensitive client off the client-hosted plane",
			manifest: func(s string) string { return strings.Replace(s, "data_class: normal", "data_class: sensitive", 1) },
			want:     "invariant 7",
		},
		{
			name:     "invariant 9: cloud plane on our account",
			manifest: func(s string) string { return strings.Replace(s, "plane: shared", "plane: cloud", 1) },
			want:     "invariant 9",
		},
		{
			name:     "runtime dedicated with a shared broker",
			manifest: func(s string) string { return strings.Replace(s, "runtime: shared", "runtime: dedicated", 1) },
			want:     "runtime: dedicated",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mRaw, pRaw := string(manifestRaw), string(policyRaw)
			if c.manifest != nil {
				mRaw = c.manifest(mRaw)
			}
			if c.policy != nil {
				pRaw = c.policy(pRaw)
			}
			m, err := access.ParseManifest([]byte(mRaw))
			if err != nil {
				t.Fatalf("mutated manifest does not parse: %v", err)
			}
			if c.manifest != nil && mRaw == string(manifestRaw) {
				t.Fatal("mutation did not change the manifest — the red-proof would pass for the wrong reason")
			}
			if c.policy != nil && pRaw == string(policyRaw) {
				t.Fatal("mutation did not change the ceiling — the red-proof would pass for the wrong reason")
			}
			policy, err := access.ParsePolicy([]byte(pRaw))
			if err != nil {
				t.Fatalf("mutated ceiling does not parse: %v", err)
			}

			err = m.Validate()
			if err == nil {
				err = policy.Validate()
			}
			if err == nil {
				err = access.Coverage(policy, append(providers, c.extraProviders...))
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

// TestAccessTemplate pins the scaffold: a new client inherits a parseable
// manifest with the plane OFF, so turning it on is a decision and never an
// accident of copying a working client.
func TestAccessTemplate(t *testing.T) {
	manifestPath := filepath.Join("_template", "client.yaml")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	m, err := access.ParseManifest(raw)
	if err != nil {
		t.Fatalf("%s: %v", manifestPath, err)
	}
	if !m.Access.Present {
		t.Fatalf("%s: no access: block — the template must show the shape, even with the plane off", manifestPath)
	}
	if m.Access.Enabled {
		t.Fatalf("%s: access.enabled is true — a new client must opt in, not inherit an exposure", manifestPath)
	}

	policyPath := filepath.Join("_template", "catalog", "policy.yaml")
	policyRaw, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("%s: %v", policyPath, err)
	}
	policy, err := access.ParsePolicy(policyRaw)
	if err != nil {
		t.Fatalf("%s: %v", policyPath, err)
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("%s: %v", policyPath, err)
	}
}
