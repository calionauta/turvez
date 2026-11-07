package grants

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/calionauta/turvez/internal/access/nango"
)

// Backend is the authority for connections. It is kept narrow on purpose: the
// manager decides policy, the broker stores the grant.
type Backend interface {
	CreateGrant(tenant, provider string) (nango.Grant, error)
	ListForTenant(tenant string) ([]nango.Connection, error)
	Revoke(connectionID, providerConfigKey string) error
}

// Event kinds written to the audit sink.
const (
	EventGrantCreated  = "grant.created"
	EventGrantExpired  = "grant.expired"
	EventGrantRevoked  = "grant.revoked"
	EventAccessChecked = "access.checked"
)

// Event is one auditable fact. Secrets and tokens are never part of it.
type Event struct {
	At       time.Time
	Kind     string
	Tenant   string
	Provider string
	ConnID   string
	Result   string
}

// Sink receives audit events. FileSink writes JSON lines; MemorySink is for tests.
type Sink interface {
	Append(Event) error
}

// Manager wires policy to the backend and the audit sink.
type Manager struct {
	Backend Backend
	Audit   Sink
}

func (m *Manager) Create(tenant, provider string) (nango.Grant, error) {
	g, err := m.Backend.CreateGrant(tenant, provider)
	if err != nil {
		return nango.Grant{}, err
	}
	_ = m.audit(Event{Kind: EventGrantCreated, Tenant: tenant, Provider: provider, Result: "ok"})
	return g, nil
}

func (m *Manager) Active(tenant string) ([]nango.Connection, error) {
	conns, err := m.Backend.ListForTenant(tenant)
	if err != nil {
		return nil, err
	}
	_ = m.audit(Event{Kind: EventAccessChecked, Tenant: tenant, Result: fmt.Sprintf("ok:%d", len(conns))})
	return conns, nil
}

func (m *Manager) Revoke(tenant, connectionID, providerConfigKey, who string) error {
	if err := m.Backend.Revoke(connectionID, providerConfigKey); err != nil {
		return err
	}
	_ = m.audit(Event{Kind: EventGrantRevoked, Tenant: tenant, Provider: providerConfigKey, ConnID: connectionID, Result: who})
	return nil
}

func (m *Manager) audit(e Event) error {
	if m.Audit == nil {
		return nil
	}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	return m.Audit.Append(e)
}

// MemorySink is the test sink.
type MemorySink struct {
	mu     sync.Mutex
	events []Event
}

func (s *MemorySink) Append(e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return nil
}

func (s *MemorySink) Events() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, len(s.events))
	copy(out, s.events)
	return out
}

// FileSink appends JSON lines to path.
type FileSink struct{ Path string }

func (s FileSink) Append(e Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
