package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/calionauta/turvez/internal/access"
	"github.com/calionauta/turvez/internal/access/grants"
	"github.com/calionauta/turvez/internal/access/nango"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	host := os.Getenv("NANGO_HOST")
	key := os.Getenv("NANGO_SECRET_KEY")
	if host == "" || key == "" {
		fmt.Fprintln(os.Stderr, "set NANGO_HOST and NANGO_SECRET_KEY (environment-scoped)")
		os.Exit(1)
	}
	auditPath := os.Getenv("ACCESS_AUDIT_FILE")
	if auditPath == "" {
		auditPath = "/tmp/access-audit.jsonl"
	}
	m := &grants.Manager{Backend: nango.New(host, key), Audit: grants.FileSink{Path: auditPath}}

	switch os.Args[1] {
	case "grant":
		fs := flag.NewFlagSet("grant", flag.ContinueOnError)
		tenant := fs.String("tenant", "", "client slug")
		provider := fs.String("provider", "", "provider slug")
		policy := fs.String("policy", "", "catalog/policy.yaml")
		if err := fs.Parse(os.Args[2:]); err != nil {
			fatal(err)
		}
		if *tenant == "" || *provider == "" {
			usageGrant()
			os.Exit(2)
		}
		if *policy != "" {
			raw, err := os.ReadFile(*policy)
			if err != nil {
				fatal(err)
			}
			p, err := access.ParsePolicy(raw)
			if err != nil {
				fatal(err)
			}
			if err := p.Validate(); err != nil {
				fatal(err)
			}
			if err := access.Coverage(p, []string{*provider}); err != nil {
				fatal(err)
			}
		}
		g, err := m.Create(*tenant, *provider)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("link=%s\n", g.ConnectLink)
		fmt.Printf("expires=%s\n", g.ExpiresAt)
	case "list":
		fs := flag.NewFlagSet("list", flag.ContinueOnError)
		tenant := fs.String("tenant", "", "client slug")
		_ = fs.Parse(os.Args[2:])
		if *tenant == "" {
			usageList()
			os.Exit(2)
		}
		conns, err := m.Active(*tenant)
		if err != nil {
			fatal(err)
		}
		out := make([]any, 0, len(conns))
		for _, c := range conns {
			out = append(out, map[string]any{"connection_id": c.ConnectionID, "provider_config_key": c.ProviderConfigKey, "created": c.Created})
		}
		if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
			fatal(err)
		}
	case "revoke":
		fs := flag.NewFlagSet("revoke", flag.ContinueOnError)
		tenant := fs.String("tenant", "", "client slug")
		conn := fs.String("connection", "", "connection id")
		provider := fs.String("provider", "", "provider config key")
		who := fs.String("who", "", "operator performing the revocation")
		_ = fs.Parse(os.Args[2:])
		if *tenant == "" || *conn == "" || *provider == "" || *who == "" {
			usageRevoke()
			os.Exit(2)
		}
		if err := m.Revoke(*tenant, *conn, *provider, *who); err != nil {
			fatal(err)
		}
		fmt.Println("revoked")
	case "audit":
		fs := flag.NewFlagSet("audit", flag.ContinueOnError)
		tenant := fs.String("tenant", "", "client slug")
		file := fs.String("file", auditPath, "audit JSONL file")
		_ = fs.Parse(os.Args[2:])
		data, err := os.ReadFile(*file)
		if err != nil {
			fatal(err)
		}
		for _, line := range splitLines(data) {
			if line == "" {
				continue
			}
			var e map[string]any
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				fatal(err)
			}
			if *tenant == "" || e["Tenant"] == *tenant {
				fmt.Println(line)
			}
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: access-plane <grant|list|revoke|audit> [flags]")
}

func usageGrant() {
	fmt.Fprintln(os.Stderr, "usage: access-plane grant --tenant <client> --provider <slug> [--policy <catalog/policy.yaml>]")
}

func usageList() {
	fmt.Fprintln(os.Stderr, "usage: access-plane list --tenant <client>")
}

func usageRevoke() {
	fmt.Fprintln(os.Stderr, "usage: access-plane revoke --tenant <client> --connection <id> --provider <slug> --who <operator>")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}

func splitLines(b []byte) []string {
	var out []string
	start := 0
	for i, c := range b {
		if c == '\n' {
			out = append(out, string(b[start:i]))
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, string(b[start:]))
	}
	return out
}
