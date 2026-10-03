package secret

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGet(t *testing.T) {
	t.Setenv("SECRET_TEST_TOKEN", "abc123")
	v, err := Get("SECRET_TEST_TOKEN")
	if err != nil || v != "abc123" {
		t.Fatalf("Get = %q, %v", v, err)
	}
	if _, err := Get("SECRET_TEST_DEFINITELY_MISSING"); err == nil {
		t.Fatal("expected error for missing secret")
	}
	if _, err := Get("  "); err == nil {
		t.Fatal("expected error for empty secret name")
	}
}

func TestLoadDotEnvFileDoesNotOverride(t *testing.T) {
	t.Setenv("DOTENV_KEPT", "from-env")
	dir := t.TempDir()
	p := filepath.Join(dir, "test.env")
	content := "# comment\nDOTENV_KEPT=from-file\nDOTENV_NEW=\"hello world\"\nEMPTY=\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadDotEnvFile(p); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DOTENV_KEPT"); got != "from-env" {
		t.Fatalf("existing env must win, got %q", got)
	}
	if got := os.Getenv("DOTENV_NEW"); got != "hello world" {
		t.Fatalf("quoted value not parsed, got %q", got)
	}
}
