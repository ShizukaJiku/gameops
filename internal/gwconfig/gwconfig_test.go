package gwconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gameops-gateway.toml")
	content := `
listen_addr = ":443"
domain = "admin.example.com"
admin_password_hash = "$2a$10$examplehash"
session_secret = "a-long-random-string"

[[hosts]]
name = "shizu-server"
addr = "127.0.0.1:8090"
token = "shared-secret"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.ListenAddr != ":443" || cfg.Domain != "admin.example.com" {
		t.Fatalf("unexpected top-level config: %+v", cfg)
	}
	if cfg.AdminPasswordHash != "$2a$10$examplehash" || cfg.SessionSecret != "a-long-random-string" {
		t.Fatalf("unexpected auth config: %+v", cfg)
	}
	if len(cfg.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(cfg.Hosts))
	}
	h := cfg.Hosts[0]
	if h.Name != "shizu-server" || h.Addr != "127.0.0.1:8090" || h.Token != "shared-secret" {
		t.Fatalf("unexpected host entry: %+v", h)
	}
}

func TestLoadMissingFileReturnsError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "does-not-exist.toml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
