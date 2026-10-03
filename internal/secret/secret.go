// Package secret resolves credentials by name at runtime.
//
// Workflow JSON carries secret NAMES (e.g. "auth_secret":
// "ACME_HUBSPOT_TOKEN"); values come only from the process environment,
// optionally seeded from an age-encrypted dotenv file. See secrets/README.md.
package secret

import (
	"fmt"
	"os"
	"strings"
)

// Get returns the value of env var name, or an error naming the missing var.
// Handlers must fail (retryable) or FailPermanent on this error — never
// fall back to a default credential.
func Get(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("secret: empty secret name")
	}
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return "", fmt.Errorf("secret: env var %s is not set", name)
	}
	return v, nil
}

// Must is Get for wiring that should crash the worker at startup
// (e.g. the ops webhook token) rather than fail per-task.
func Must(name string) string {
	v, err := Get(name)
	if err != nil {
		panic(err.Error())
	}
	return v
}

// Optional returns the value or "" when unset. For non-credential knobs
// only (base URLs, timeouts) — never for tokens.
func Optional(name, def string) string {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v
	}
	return def
}

// LoadDotEnvFile parses KEY=VALUE lines (ignoring blanks and # comments,
// stripping optional surrounding quotes) without overriding existing vars.
func LoadDotEnvFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("secret: read dotenv %s: %w", path, err)
	}
	return mergeDotEnvBytes(raw)
}
