package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gameops.toml")
	content := `
[[instances]]
name = "minecraft"
game = "minecraft"
listen_port = 25565
backend_port = 25566
idle_timeout_minutes = 15
poll_interval_seconds = 30
backend_ready_timeout_minutes = 5
start_command = "schtasks /run /tn mc-forge"

[instances.minecraft_config]
rcon_port = 25575
rcon_password = "secret"
motd_fallback = "sleeping"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(cfg.Instances))
	}
	in := cfg.Instances[0]
	if in.Name != "minecraft" || in.ListenPort != 25565 || in.BackendPort != 25566 {
		t.Fatalf("unexpected instance: %+v", in)
	}
	if in.Minecraft == nil || in.Minecraft.RconPassword != "secret" {
		t.Fatalf("unexpected minecraft config: %+v", in.Minecraft)
	}
}

func TestLoadWithBackupConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gameops.toml")
	content := `
[[instances]]
name = "minecraft"
game = "minecraft"
listen_port = 25565
backend_port = 25566

[instances.minecraft_config]
rcon_port = 25575
rcon_password = "secret"

[instances.backup_config]
world_path = "C:\\test\\world"
backups_dir = "C:\\test\\backups"
max_backups = 6
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(cfg.Instances))
	}
	b := cfg.Instances[0].Backup
	if b == nil {
		t.Fatal("expected Backup config to be non-nil")
	}
	if b.WorldPath != `C:\test\world` || b.BackupsDir != `C:\test\backups` || b.MaxBackups != 6 {
		t.Fatalf("unexpected backup config: %+v", b)
	}
}

func TestLoadWithoutBackupConfigLeavesBackupNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gameops.toml")
	content := `
[[instances]]
name = "minecraft"
game = "minecraft"
listen_port = 25565
backend_port = 25566

[instances.minecraft_config]
rcon_port = 25575
rcon_password = "secret"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Instances[0].Backup != nil {
		t.Fatalf("expected nil Backup config when [instances.backup_config] is absent, got %+v", cfg.Instances[0].Backup)
	}
}

func TestLoadWithMaintenanceConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gameops.toml")
	content := `
[[instances]]
name = "minecraft"
game = "minecraft"
listen_port = 25565
backend_port = 25566

[instances.maintenance_config]
process_name = "java"
stop_command = "stop"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(cfg.Instances))
	}
	m := cfg.Instances[0].Maintenance
	if m == nil {
		t.Fatal("expected Maintenance config to be non-nil")
	}
	if m.ProcessName != "java" || m.StopCommand != "stop" {
		t.Fatalf("unexpected maintenance config: %+v", m)
	}
}

func TestLoadWithoutMaintenanceConfigLeavesMaintenanceNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gameops.toml")
	content := `
[[instances]]
name = "minecraft"
game = "minecraft"
listen_port = 25565
backend_port = 25566
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Instances[0].Maintenance != nil {
		t.Fatalf("expected nil Maintenance config when [instances.maintenance_config] is absent, got %+v", cfg.Instances[0].Maintenance)
	}
}
