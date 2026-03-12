package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	// 32 bytes = 64 hex chars
	writeFile(t, dir, "master_key", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	writeFile(t, dir, "admin_token", "my-admin-token")

	t.Setenv("MASTER_KEY_FILE", filepath.Join(dir, "master_key"))
	t.Setenv("ADMIN_TOKEN_FILE", filepath.Join(dir, "admin_token"))
	t.Setenv("DB_PATH", filepath.Join(dir, "test.db"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.MasterKey) != 32 {
		t.Errorf("expected 32-byte master key, got %d", len(cfg.MasterKey))
	}
	if cfg.AdminToken != "my-admin-token" {
		t.Errorf("expected admin token 'my-admin-token', got %q", cfg.AdminToken)
	}
	if cfg.BindAddr != "127.0.0.1" {
		t.Errorf("expected default bind addr 127.0.0.1, got %q", cfg.BindAddr)
	}
	if cfg.AdminPort != "9100" {
		t.Errorf("expected default admin port 9100, got %q", cfg.AdminPort)
	}
	if cfg.DataPort != "9101" {
		t.Errorf("expected default data port 9101, got %q", cfg.DataPort)
	}
}

func TestLoadMissingMasterKeyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "admin_token", "token")

	t.Setenv("MASTER_KEY_FILE", filepath.Join(dir, "nonexistent"))
	t.Setenv("ADMIN_TOKEN_FILE", filepath.Join(dir, "admin_token"))

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing master key file")
	}
}

func TestLoadInvalidHexMasterKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "master_key", "not-valid-hex")
	writeFile(t, dir, "admin_token", "token")

	t.Setenv("MASTER_KEY_FILE", filepath.Join(dir, "master_key"))
	t.Setenv("ADMIN_TOKEN_FILE", filepath.Join(dir, "admin_token"))

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid hex master key")
	}
}

func TestLoadWrongLengthMasterKey(t *testing.T) {
	dir := t.TempDir()
	// 16 bytes = 32 hex chars (too short)
	writeFile(t, dir, "master_key", "0123456789abcdef0123456789abcdef")
	writeFile(t, dir, "admin_token", "token")

	t.Setenv("MASTER_KEY_FILE", filepath.Join(dir, "master_key"))
	t.Setenv("ADMIN_TOKEN_FILE", filepath.Join(dir, "admin_token"))

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for wrong-length master key")
	}
}

func TestLoadEmptyAdminToken(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "master_key", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	writeFile(t, dir, "admin_token", "")

	t.Setenv("MASTER_KEY_FILE", filepath.Join(dir, "master_key"))
	t.Setenv("ADMIN_TOKEN_FILE", filepath.Join(dir, "admin_token"))

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for empty admin token")
	}
}

func TestLoadWhitespaceTrimming(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "master_key", "  0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n")
	writeFile(t, dir, "admin_token", "  my-token  \n")

	t.Setenv("MASTER_KEY_FILE", filepath.Join(dir, "master_key"))
	t.Setenv("ADMIN_TOKEN_FILE", filepath.Join(dir, "admin_token"))
	t.Setenv("DB_PATH", filepath.Join(dir, "test.db"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AdminToken != "my-token" {
		t.Errorf("expected trimmed token 'my-token', got %q", cfg.AdminToken)
	}
}

func TestEnvOr(t *testing.T) {
	// Returns fallback when env not set
	if v := envOr("TEST_ENVVAR_NONEXISTENT_123", "fallback"); v != "fallback" {
		t.Errorf("expected 'fallback', got %q", v)
	}

	// Returns env value when set
	t.Setenv("TEST_ENVVAR_SET_123", "from-env")
	if v := envOr("TEST_ENVVAR_SET_123", "fallback"); v != "from-env" {
		t.Errorf("expected 'from-env', got %q", v)
	}
}

func TestLoadCustomPorts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "master_key", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	writeFile(t, dir, "admin_token", "token")

	t.Setenv("MASTER_KEY_FILE", filepath.Join(dir, "master_key"))
	t.Setenv("ADMIN_TOKEN_FILE", filepath.Join(dir, "admin_token"))
	t.Setenv("DB_PATH", filepath.Join(dir, "test.db"))
	t.Setenv("BIND_ADDR", "0.0.0.0")
	t.Setenv("ADMIN_PORT", "8100")
	t.Setenv("DATA_PORT", "8101")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BindAddr != "0.0.0.0" {
		t.Errorf("expected bind addr 0.0.0.0, got %q", cfg.BindAddr)
	}
	if cfg.AdminPort != "8100" {
		t.Errorf("expected admin port 8100, got %q", cfg.AdminPort)
	}
	if cfg.DataPort != "8101" {
		t.Errorf("expected data port 8101, got %q", cfg.DataPort)
	}
}
