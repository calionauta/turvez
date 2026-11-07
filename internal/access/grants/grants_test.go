package grants

import (
	"testing"

	"github.com/calionauta/turvez/internal/access/nango"
)

type fakeBackend struct {
	conns   []nango.Connection
	revoked bool
}

func (f *fakeBackend) CreateGrant(tenant, provider string) (nango.Grant, error) {
	return nango.Grant{ConnectLink: "https://connect.example/" + tenant + "/" + provider, ExpiresAt: "2099-01-01"}, nil
}
func (f *fakeBackend) ListForTenant(tenant string) ([]nango.Connection, error) {
	var out []nango.Connection
	for _, c := range f.conns {
		if c.Tags["turvez_client"] == tenant {
			out = append(out, c)
		}
	}
	return out, nil
}
func (f *fakeBackend) Revoke(connectionID, provider string) error {
	f.revoked = true
	return nil
}

func TestManagerAuditsLifecycle(t *testing.T) {
	fb := &fakeBackend{conns: []nango.Connection{{ConnectionID: "c1", ProviderConfigKey: "github", Tags: map[string]string{"turvez_client": "acme"}}}}
	sink := &MemorySink{}
	m := &Manager{Backend: fb, Audit: sink}

	if _, err := m.Create("acme", "github"); err != nil {
		t.Fatal(err)
	}
	conns, err := m.Active("acme")
	if err != nil || len(conns) != 1 {
		t.Fatalf("active %d err %v", len(conns), err)
	}
	if err := m.Revoke("acme", "c1", "github", "ops"); err != nil {
		t.Fatal(err)
	}
	ev := sink.Events()
	if len(ev) != 3 || ev[2].Kind != EventGrantRevoked || ev[2].Result != "ops" {
		t.Fatalf("events %+v", ev)
	}
}
