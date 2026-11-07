package access

import (
	"strings"
	"testing"
)

const sampleManifest = `client: acme
engine: dagnats
runtime: shared
data_class: normal    # normal | sensitive
owner: ops@example.com
target:
  nats_url: nats://127.0.0.1:4222
access:
  enabled: true
  plane: shared
  surface: mcp
  identity: org
  policy: catalog/policy.yaml
  vendor_account: consultancy
`

const samplePolicy = `providers:
  hubspot:
    kinds: [read, write]
    approval: writes
  gmail:
    kinds: [read]
    approval: none
limits:
  max_grants: 20
  scopes_ceiling: [".read", ".readonly"]
`

func TestParseManifest(t *testing.T) {
	m, err := ParseManifest([]byte(sampleManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.Client != "acme" || m.Engine != "dagnats" || m.DataClass != "normal" {
		t.Fatalf("manifest: %#v", m)
	}
	if !m.Access.Present || !m.Access.Enabled {
		t.Fatalf("access block not read: %#v", m.Access)
	}
	if m.Access.Plane != PlaneShared || m.Access.Identity != IdentityOrg || m.Access.Surface != SurfaceMCP {
		t.Fatalf("access: %#v", m.Access)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

// A manifest without the block is legal: the plane is opt-in per client, and a
// real ops repo has clients that only run automations.
func TestParseManifestWithoutAccess(t *testing.T) {
	m, err := ParseManifest([]byte("client: acme\nengine: windmill\ndata_class: normal\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Access.Present || m.Access.Enabled {
		t.Fatalf("absent block must read as disabled: %#v", m.Access)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("absent block must not fail: %v", err)
	}
}

func TestManifestTopology(t *testing.T) {
	cases := []struct {
		name  string
		block string
		class string
		want  string
	}{
		{"client-hosted is the default shape", "enabled: true\n  plane: client", DataClassSensitive, ""},
		{"sensitive cannot be shared", "enabled: true\n  plane: shared", DataClassSensitive, "invariant 7"},
		{"sensitive cannot be cloud", "enabled: true\n  plane: cloud\n  vendor_account: client", DataClassSensitive, "invariant 7"},
		{"cloud needs the client's account", "enabled: true\n  plane: cloud", DataClassNormal, "invariant 9"},
		{"cloud with our account is shared", "enabled: true\n  plane: cloud\n  vendor_account: consultancy", DataClassNormal, "invariant 9"},
		{"cloud on the client's account", "enabled: true\n  plane: cloud\n  vendor_account: client", DataClassNormal, ""},
		{"shared with the client's account contradicts", "enabled: true\n  plane: shared\n  vendor_account: client", DataClassNormal, "contradicts"},
		{"unknown plane", "enabled: true\n  plane: vpc", DataClassNormal, "must be client|cloud|shared"},
		{"unknown identity", "enabled: true\n  plane: shared\n  identity: team", DataClassNormal, "identity"},
		{"policy cannot escape the client folder", "enabled: true\n  plane: shared\n  policy: ../../policy.yaml", DataClassNormal, "inside the client folder"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := "client: acme\ndata_class: " + c.class + "\naccess:\n  " + c.block + "\n"
			m, err := ParseManifest([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			err = m.Validate()
			if c.want == "" {
				if err != nil {
					t.Fatalf("unexpected rejection: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestManifestKeyCollision(t *testing.T) {
	raw := "client: acme\nengine: dagnats\ndata_class: normal\naccess:\n  enabled: true\n  plane: shared\n  host: acme.internal\n"
	if _, err := ParseManifest([]byte(raw)); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("want a collision error, got %v", err)
	}
}

func TestDedicatedRuntimeNeedsClientPlane(t *testing.T) {
	for _, runtime := range []string{"shared", "dedicated"} {
		for _, plane := range []string{PlaneClient, PlaneCloud, PlaneShared} {
			raw := "client: acme\ndata_class: normal\nruntime: " + runtime + "\naccess:\n  enabled: true\n  plane: " + plane + "\n  vendor_account: client\n"
			m, err := ParseManifest([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			err = m.Validate()
			// A shared runtime may pick any plane; a dedicated one may not.
			wantErr := runtime == "dedicated" && plane != PlaneClient
			// plane: shared with vendor_account: client is a separate
			// contradiction, exercised in TestManifestTopology.
			if plane == PlaneShared && err != nil && strings.Contains(err.Error(), "contradicts") {
				continue
			}
			if wantErr && (err == nil || !strings.Contains(err.Error(), "runtime: dedicated")) {
				t.Errorf("%s/%s: want the dedicated-runtime error, got %v", runtime, plane, err)
			}
			if !wantErr && err != nil {
				t.Errorf("%s/%s: unexpected rejection: %v", runtime, plane, err)
			}
		}
	}
}

func TestPolicy(t *testing.T) {
	p, err := ParsePolicy([]byte(samplePolicy))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Providers) != 2 || p.Limits.MaxGrants != 20 {
		t.Fatalf("policy: %#v", p)
	}
	if got := p.Providers["hubspot"].Kinds; len(got) != 2 || got[1] != KindWrite {
		t.Fatalf("hubspot kinds: %#v", got)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("valid ceiling rejected: %v", err)
	}
}

func TestPolicyRejections(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			"write with approval: none",
			"providers:\n  hubspot:\n    kinds: [write]\n    approval: none\nlimits:\n  max_grants: 5\n",
			"invariant 2",
		},
		{
			"no grant cap",
			"providers:\n  gmail:\n    kinds: [read]\n    approval: none\n",
			"max_grants",
		},
		{
			"no scope ceiling",
			"providers:\n  gmail:\n    kinds: [read]\n    approval: none\nlimits:\n  max_grants: 5\n",
			"scopes_ceiling",
		},
		{
			"unknown kind",
			"providers:\n  gmail:\n    kinds: [admin]\n    approval: none\nlimits:\n  max_grants: 5\n",
			"unknown kind",
		},
		{
			"provider is not a slug",
			"providers:\n  HubSpot:\n    kinds: [read]\n    approval: none\nlimits:\n  max_grants: 5\n",
			"must be the lowercase provider slug",
		},
		{
			"bad approval",
			"providers:\n  gmail:\n    kinds: [read]\n    approval: maybe\nlimits:\n  max_grants: 5\n",
			"approval",
		},
		{
			"scope with a space",
			"providers:\n  gmail:\n    kinds: [read]\n    approval: none\nlimits:\n  max_grants: 5\n  scopes_ceiling: [\"gmail read\"]\n",
			"not a scope",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := ParsePolicy([]byte(c.raw))
			if err != nil {
				t.Fatal(err)
			}
			err = p.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestCoverage(t *testing.T) {
	p, err := ParsePolicy([]byte(samplePolicy))
	if err != nil {
		t.Fatal(err)
	}
	if err := Coverage(p, []string{"hubspot", "gmail", ""}); err != nil {
		t.Fatalf("covered providers rejected: %v", err)
	}
	err = Coverage(p, []string{"hubspot", "slack"})
	if err == nil || !strings.Contains(err.Error(), "slack") {
		t.Fatalf("uncovered provider accepted: %v", err)
	}
	if strings.Contains(err.Error(), "hubspot") {
		t.Fatalf("covered provider reported as missing: %v", err)
	}
}

// Invariant 1 lives here: the renderer cannot produce a name without a tenant.
func TestToolName(t *testing.T) {
	got, err := ToolName("acme", "hubspot", "contacts.list")
	if err != nil || got != "acme.hubspot.contacts.list" {
		t.Fatalf("got %q, %v", got, err)
	}
	cases := []struct{ tenant, provider, op, want string }{
		{"", "hubspot", "contacts.list", "tenant"},
		{"acme", "hubspot", "", "operation"},
		{"acme", "hubspot", "Contacts List", "operation"},
		{"acme", "HubSpot", "contacts.list", "provider"},
		{"Acme", "hubspot", "contacts.list", "tenant"},
		{"acme.hubspot", "contacts", "list", "tenant"},
	}
	for _, c := range cases {
		if _, err := ToolName(c.tenant, c.provider, c.op); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("ToolName(%q,%q,%q): want %q, got %v", c.tenant, c.provider, c.op, c.want, err)
		}
	}
}

func TestParseTreeRejectsMalformed(t *testing.T) {
	cases := []struct{ name, raw, want string }{
		{"no colon", "client acme\n", "key: value"},
		{"tab indent", "access:\n\tenabled: true\n", "not tabs"},
		{"duplicate key", "client: a\nclient: b\n", "duplicate"},
		{"unclosed list", "kinds: [read, write\n", "must close"},
		{"empty list entry", "kinds: [read, , write]\n", "empty entry"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseTree([]byte(c.raw))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}
