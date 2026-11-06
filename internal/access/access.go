// Package access is the contract for the third plane of turvez: the client's
// own AI agents calling authenticated tools (docs/access-plane.md).
//
// Control is git; execution is the client's engine. This plane is neither: it
// has per-request identity and the credential belongs to the client, not to the
// consultancy. The package holds the parts a test can pin without a broker
// running:
//
//   - the `access:` block of clients/<client>/client.yaml;
//   - the ceiling in clients/<client>/catalog/policy.yaml;
//   - the only legal shape for a tool name.
//
// It parses the tiny YAML subset the repo already uses for manifests (flat
// scalars, one nesting level per two spaces, inline `[a, b]` lists). A full
// YAML parser would be a dependency for three keys, which is the same trade
// clients/recipes_test.go already made for parseManifest.
package access

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Broker planes. The letter is the profile in docs/access-plane.md.
const (
	// PlaneClient is profile A: the broker runs on the client's infrastructure,
	// under their account. The default, and the only legal plane for
	// data_class: sensitive.
	PlaneClient = "client"
	// PlaneCloud is profile B: the vendor's cloud, but under the client's own
	// account and card. If the account is ours, this is not B.
	PlaneCloud = "cloud"
	// PlaneShared is profile C: one broker on the consultancy host, for pilots
	// with data_class: normal only.
	PlaneShared = "shared"
)

// Surfaces the access plane can expose.
const (
	SurfaceMCP  = "mcp"
	SurfaceNone = "none"
)

// Identity models: what a grant points at.
const (
	// IdentityOrg means one connection per provider; the agent acts as the
	// company.
	IdentityOrg = "org"
	// IdentityPerUser means each staff member connects their own account and
	// the agent acts as that person (`<client>:<user>`).
	IdentityPerUser = "per-user"
)

// Tool kinds and their approval policies.
const (
	KindRead        = "read"
	KindWrite       = "write"
	KindDestructive = "destructive"

	ApprovalNone   = "none"
	ApprovalWrites = "writes"
	ApprovalAll    = "all"
)

// Vendor accounts for profile B: whose account the vendor cloud runs under.
const (
	AccountClient       = "client"
	AccountConsultancy  = "consultancy"
	DataClassNormal     = "normal"
	DataClassSensitive  = "sensitive"
	defaultPolicyPath   = "catalog/policy.yaml"
	maxToolNameSegments = 8
)

var (
	slugRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	opRe       = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)*$`)
	scopeEntry = regexp.MustCompile(`^[A-Za-z0-9_.:/-]+$`)
)

// Manifest is the subset of clients/<client>/client.yaml the access plane
// needs. Other fields are read by clients/recipes_test.go's flat parser.
type Manifest struct {
	Client    string
	Engine    string
	Runtime   string
	DataClass string
	Access    Config
}

// Config is the `access:` block. Absent block and `enabled: false` are both
// "no access plane"; Present distinguishes them so a test can demand that the
// scaffold ships the block at all.
type Config struct {
	Present       bool
	Enabled       bool
	Plane         string
	Surface       string
	Identity      string
	PolicyPath    string
	VendorAccount string
}

// Policy is clients/<client>/catalog/policy.yaml: the ceiling. It is not an
// inventory — what is actually connected lives in the broker, so connecting an
// already-allowed app never needs a commit (docs/access-plane.md).
type Policy struct {
	Providers map[string]Provider
	Limits    Limits
}

// Provider is one entry of the ceiling.
type Provider struct {
	Kinds    []string
	Approval string
}

// Limits bound how far a client can go inside the ceiling, and which OAuth
// scopes the broker may request.
type Limits struct {
	MaxGrants     int
	ScopesCeiling []string
}

// ParseManifest reads client.yaml. A file with no `access:` block is valid:
// the plane is opt-in per client.
func ParseManifest(raw []byte) (Manifest, error) {
	tree, err := parseTree(raw)
	if err != nil {
		return Manifest{}, err
	}
	m := Manifest{
		Client:    scalar(tree["client"]),
		Engine:    scalar(tree["engine"]),
		Runtime:   scalar(tree["runtime"]),
		DataClass: scalar(tree["data_class"]),
	}
	block, ok := tree["access"]
	if !ok {
		return m, nil
	}
	node, ok := block.(map[string]any)
	if !ok {
		return Manifest{}, fmt.Errorf("access: must be a block, got %q", scalar(block))
	}
	cfg := Config{
		Present:       true,
		Enabled:       boolean(node["enabled"]),
		Plane:         scalar(node["plane"]),
		Surface:       scalar(node["surface"]),
		Identity:      scalar(node["identity"]),
		PolicyPath:    scalar(node["policy"]),
		VendorAccount: scalar(node["vendor_account"]),
	}
	// scripts/verify-isolation.sh and clients/recipes_test.go read client.yaml
	// with a flat `key: value` scan, so a nested key with a top-level name would
	// silently answer for the wrong field (`access.host` for `target.host`).
	for _, reserved := range []string{"client", "engine", "runtime", "data_class", "owner", "host"} {
		if _, dup := node[reserved]; dup {
			return Manifest{}, fmt.Errorf("access.%s: collides with the top-level manifest key of the same name; the flat readers (verify-isolation.sh, recipes_test.go) cannot tell them apart", reserved)
		}
	}
	if cfg.PolicyPath == "" {
		cfg.PolicyPath = defaultPolicyPath
	}
	if cfg.Enabled {
		if cfg.Surface == "" {
			cfg.Surface = SurfaceMCP
		}
		if cfg.Identity == "" {
			cfg.Identity = IdentityOrg
		}
	}
	m.Access = cfg
	return m, nil
}

// ParsePolicy reads the ceiling.
func ParsePolicy(raw []byte) (Policy, error) {
	tree, err := parseTree(raw)
	if err != nil {
		return Policy{}, err
	}
	p := Policy{Providers: map[string]Provider{}}
	switch providers := tree["providers"].(type) {
	case nil:
	case map[string]any:
		for name, entry := range providers {
			node, ok := entry.(map[string]any)
			if !ok {
				return Policy{}, fmt.Errorf("providers.%s: must be a block with kinds/approval", name)
			}
			p.Providers[name] = Provider{
				Kinds:    list(node["kinds"]),
				Approval: scalar(node["approval"]),
			}
		}
	default:
		return Policy{}, fmt.Errorf("providers: must be a block of provider entries")
	}
	if limits, ok := tree["limits"].(map[string]any); ok {
		n, err := integer(limits["max_grants"])
		if err != nil {
			return Policy{}, fmt.Errorf("limits.max_grants: %v", err)
		}
		p.Limits.MaxGrants = n
		p.Limits.ScopesCeiling = list(limits["scopes_ceiling"])
	}
	return p, nil
}

// Validate enforces the topology rules of docs/access-plane.md §3. The data
// class comes from the manifest itself: a client cannot be `sensitive` and
// shared at the same time, so the two fields must agree.
func (m Manifest) Validate() error {
	cfg := m.Access
	if !cfg.Enabled {
		return nil
	}
	if err := cfg.Validate(m.DataClass); err != nil {
		return err
	}
	// A dedicated runtime means the whole client lives on their own host. A
	// broker for that client on our shared stack would put their tokens back
	// inside our perimeter, which is the one thing dedicated infra exists to
	// prevent (docs/access-plane.md §3).
	if m.Runtime == "dedicated" && cfg.Plane != PlaneClient {
		return fmt.Errorf("access.plane: %q with runtime: dedicated — a dedicated client's broker is client-hosted too (plane: %s)",
			cfg.Plane, PlaneClient)
	}
	return nil
}

// Validate checks one `access:` block against the client's data class.
func (c Config) Validate(dataClass string) error {
	if !c.Present {
		return fmt.Errorf("access: block missing (copy clients/_template/client.yaml)")
	}
	if !c.Enabled {
		return nil
	}
	if dataClass == "" {
		return fmt.Errorf("access.enabled: true but data_class is not set")
	}
	switch c.Plane {
	case PlaneClient, PlaneCloud, PlaneShared:
	default:
		return fmt.Errorf("access.plane: %q must be %s|%s|%s (docs/access-plane.md §3)",
			c.Plane, PlaneClient, PlaneCloud, PlaneShared)
	}
	switch c.Surface {
	case SurfaceMCP, SurfaceNone:
	default:
		return fmt.Errorf("access.surface: %q must be %s|%s", c.Surface, SurfaceMCP, SurfaceNone)
	}
	switch c.Identity {
	case IdentityOrg, IdentityPerUser:
	default:
		return fmt.Errorf("access.identity: %q must be %s|%s", c.Identity, IdentityOrg, IdentityPerUser)
	}
	// Invariant: a sensitive client is client-hosted, no exception.
	if dataClass == DataClassSensitive && c.Plane != PlaneClient {
		return fmt.Errorf("access.plane: %q for a data_class: sensitive client; only %q is allowed (invariant 7)",
			c.Plane, PlaneClient)
	}
	// Invariant: B is only B while the account and the card are the client's.
	switch c.Plane {
	case PlaneCloud:
		if c.VendorAccount != AccountClient {
			return fmt.Errorf("access.plane: cloud requires access.vendor_account: client — with our account it is %q, not %q (invariant 9)",
				PlaneShared, PlaneCloud)
		}
	case PlaneShared:
		if c.VendorAccount == AccountClient {
			return fmt.Errorf("access.plane: shared with access.vendor_account: client contradicts itself; the shared broker runs under our account")
		}
	case PlaneClient:
		if c.VendorAccount == AccountConsultancy {
			return fmt.Errorf("access.plane: client with access.vendor_account: consultancy contradicts itself")
		}
	}
	if c.PolicyPath == "" {
		return fmt.Errorf("access.policy: required when the plane is enabled")
	}
	if filepath.IsAbs(c.PolicyPath) || strings.Contains(c.PolicyPath, "..") {
		return fmt.Errorf("access.policy: %q must be a path inside the client folder", c.PolicyPath)
	}
	return nil
}

// Validate checks the ceiling itself, independent of any client: a write or
// destructive provider without an approval policy is not a ceiling.
func (p Policy) Validate() error {
	if len(p.Providers) == 0 {
		return nil
	}
	if p.Limits.MaxGrants <= 0 {
		return fmt.Errorf("limits.max_grants: required (a ceiling without a grant cap is not a ceiling)")
	}
	names := make([]string, 0, len(p.Providers))
	for name := range p.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := p.Providers[name]
		if !slugRe.MatchString(name) {
			return fmt.Errorf("providers.%s: must be the lowercase provider slug (e.g. hubspot)", name)
		}
		if len(entry.Kinds) == 0 {
			return fmt.Errorf("providers.%s: kinds is required (read|write|destructive)", name)
		}
		writes := false
		for _, kind := range entry.Kinds {
			switch kind {
			case KindRead:
			case KindWrite, KindDestructive:
				writes = true
			default:
				return fmt.Errorf("providers.%s: unknown kind %q (read|write|destructive)", name, kind)
			}
		}
		switch entry.Approval {
		case ApprovalNone, ApprovalWrites, ApprovalAll:
		default:
			return fmt.Errorf("providers.%s: approval %q must be none|writes|all", name, entry.Approval)
		}
		// Invariant: a write without a declared approval is a build failure.
		if writes && entry.Approval == ApprovalNone {
			return fmt.Errorf("providers.%s: allows write/destructive with approval: none (invariant 2)", name)
		}
	}
	for _, scope := range p.Limits.ScopesCeiling {
		if !scopeEntry.MatchString(scope) {
			return fmt.Errorf("limits.scopes_ceiling: %q is not a scope string or suffix", scope)
		}
	}
	return nil
}

// Coverage enforces invariant 3: every provider the client already uses (a
// recorded connection, or a recipe calling it through the Nango proxy) must be
// inside the ceiling. You cannot grant what you have not declared.
func Coverage(p Policy, providers []string) error {
	var missing []string
	for _, provider := range providers {
		if provider == "" {
			continue
		}
		if _, ok := p.Providers[provider]; !ok {
			missing = append(missing, provider)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("providers used but not in the catalog ceiling: %s — add them to the client's policy or remove the usage (invariant 3)",
		strings.Join(missing, ", "))
}

// ToolName is the only legal shape for a tool the access plane exposes:
// <tenant>.<provider>.<operation>, e.g. acme.hubspot.contacts.list.
//
// The tenant segment is not optional and not inferred: a name that starts with
// the provider is how one tenant's tool ends up in another tenant's catalog.
func ToolName(tenant, provider, op string) (string, error) {
	if !slugRe.MatchString(tenant) {
		return "", fmt.Errorf("tool name: tenant %q must be a slug (%s) — a tool without a tenant prefix is not addressable", tenant, slugRe)
	}
	if !slugRe.MatchString(provider) {
		return "", fmt.Errorf("tool name: provider %q must be the lowercase slug", provider)
	}
	if !opRe.MatchString(op) {
		return "", fmt.Errorf("tool name: operation %q must be dot-separated lowercase (e.g. contacts.list)", op)
	}
	name := tenant + "." + provider + "." + op
	if got := len(strings.Split(name, ".")); got < 3 || got > maxToolNameSegments+2 {
		return "", fmt.Errorf("tool name %q: %d segments", name, got)
	}
	return name, nil
}

// ---- tiny YAML subset -----------------------------------------------------

// parseTree reads the subset of YAML this repo's manifests use: `key: value`
// scalars, blocks nested by indentation, and inline `[a, b]` lists. It is
// deliberately strict: an unparseable line is an error, never a silent skip,
// because a policy that half-parses is worse than one that fails the build.
func parseTree(raw []byte) (map[string]any, error) {
	root := map[string]any{}
	type frame struct {
		indent int
		node   map[string]any
	}
	stack := []frame{{indent: -1, node: root}}
	for i, line := range strings.Split(string(raw), "\n") {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := 0
		for indent < len(line) && line[indent] == ' ' {
			indent++
		}
		if indent < len(line) && line[indent] == '\t' {
			return nil, fmt.Errorf("line %d: indent with spaces, not tabs", lineNo)
		}
		content := strings.TrimRight(stripComment(line[indent:]), " \t")
		if content == "" {
			continue
		}
		for len(stack) > 1 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1].node
		key, value, ok := strings.Cut(content, ":")
		if !ok {
			return nil, fmt.Errorf("line %d: want `key: value`, got %q", lineNo, content)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", lineNo)
		}
		if _, dup := parent[key]; dup {
			return nil, fmt.Errorf("line %d: duplicate key %q", lineNo, key)
		}
		switch {
		case value == "":
			child := map[string]any{}
			parent[key] = child
			stack = append(stack, frame{indent: indent, node: child})
		case value == "{}":
			parent[key] = map[string]any{}
		case strings.HasPrefix(value, "["):
			items, err := parseInlineList(value)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", lineNo, err)
			}
			parent[key] = items
		default:
			parent[key] = unquote(value)
		}
	}
	return root, nil
}

// stripComment cuts a trailing ` # comment`. Values in these files never carry
// a literal " #", so the simple rule beats a quoting-aware scanner.
func stripComment(s string) string {
	if i := strings.Index(s, " #"); i >= 0 {
		return s[:i]
	}
	return s
}

func parseInlineList(value string) ([]string, error) {
	if !strings.HasSuffix(value, "]") {
		return nil, fmt.Errorf("list %q must close with ]", value)
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if inner == "" {
		return nil, nil
	}
	var out []string
	for _, item := range strings.Split(inner, ",") {
		item = unquote(strings.TrimSpace(item))
		if item == "" {
			return nil, fmt.Errorf("list %q has an empty entry", value)
		}
		out = append(out, item)
	}
	return out, nil
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func scalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func list(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	default:
		return nil
	}
}

func boolean(v any) bool {
	return strings.EqualFold(scalar(v), "true")
}

func integer(v any) (int, error) {
	switch t := v.(type) {
	case nil:
		return 0, nil
	case string:
		if t == "" {
			return 0, nil
		}
		n := 0
		for _, r := range t {
			if r < '0' || r > '9' {
				return 0, fmt.Errorf("%q is not a whole number", t)
			}
			n = n*10 + int(r-'0')
		}
		return n, nil
	default:
		return 0, fmt.Errorf("%v is not a whole number", v)
	}
}
