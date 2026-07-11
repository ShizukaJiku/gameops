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

func TestLoadParsesGamesSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gameops.toml")
	content := `
[games.minecraft]
idle_timeout_minutes = 15
poll_interval_seconds = 30
start_command = "schtasks /run /tn mc-forge"

[games.minecraft.minecraft_config]
rcon_port = 25575
motd_fallback = "Server asleep"

[games.minecraft.backup_config]
max_backups = 10

[games.minecraft.maintenance_config]
process_name = "java"
stop_command = "stop"

[[instances]]
name = "servermc1"
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
	game, ok := cfg.Games["minecraft"]
	if !ok {
		t.Fatal("expected games[\"minecraft\"] to exist")
	}
	if game.IdleTimeoutMinutes != 15 || game.PollIntervalSeconds != 30 || game.StartCommand != "schtasks /run /tn mc-forge" {
		t.Fatalf("unexpected game top-level defaults: %+v", game)
	}
	if game.Minecraft == nil || game.Minecraft.RconPort != 25575 || game.Minecraft.MotdFallback != "Server asleep" {
		t.Fatalf("unexpected game minecraft_config: %+v", game.Minecraft)
	}
	if game.Backup == nil || game.Backup.MaxBackups != 10 {
		t.Fatalf("unexpected game backup_config: %+v", game.Backup)
	}
	if game.Maintenance == nil || game.Maintenance.ProcessName != "java" || game.Maintenance.StopCommand != "stop" {
		t.Fatalf("unexpected game maintenance_config: %+v", game.Maintenance)
	}
}

func TestLoadMergesGameDefaultsIntoInstance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gameops.toml")
	content := `
[games.minecraft]
idle_timeout_minutes = 15
poll_interval_seconds = 30
start_command = "schtasks /run /tn mc-forge"

[games.minecraft.minecraft_config]
rcon_port = 25575

[games.minecraft.maintenance_config]
process_name = "java"
stop_command = "stop"

[[instances]]
name = "servermc1"
game = "minecraft"
listen_port = 25565
backend_port = 25566
idle_timeout_minutes = 20

[instances.minecraft_config]
rcon_password = "secret"

[[instances]]
name = "servermc2"
game = "minecraft"
listen_port = 25567
backend_port = 25568
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(cfg.Instances))
	}

	mc1 := cfg.Instances[0]
	if mc1.IdleTimeoutMinutes != 20 {
		t.Fatalf("expected servermc1's own idle_timeout_minutes (20) to win over the game default (15), got %d", mc1.IdleTimeoutMinutes)
	}
	if mc1.PollIntervalSeconds != 30 {
		t.Fatalf("expected servermc1 to inherit poll_interval_seconds from [games.minecraft], got %d", mc1.PollIntervalSeconds)
	}
	if mc1.Minecraft == nil || mc1.Minecraft.RconPassword != "secret" || mc1.Minecraft.RconPort != 25575 {
		t.Fatalf("expected servermc1 to keep its own rcon_password and inherit rcon_port, got %+v", mc1.Minecraft)
	}
	if mc1.Maintenance == nil || mc1.Maintenance.ProcessName != "java" {
		t.Fatalf("expected servermc1 to fully inherit maintenance_config from the game, got %+v", mc1.Maintenance)
	}

	mc2 := cfg.Instances[1]
	if mc2.IdleTimeoutMinutes != 15 || mc2.PollIntervalSeconds != 30 {
		t.Fatalf("expected servermc2 to fully inherit top-level defaults, got %+v", mc2)
	}
}

func TestLoadWithStartupConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gameops.toml")
	content := `
[[instances]]
name = "minecraft"
game = "minecraft"
listen_port = 25565
backend_port = 25566

[instances.startup_config]
log_path = "C:\\mc-forge\\logs\\latest.log"
boot_pattern = "Done ("
commands = ["difficulty hard", "gamerule playersSleepingPercentage 10"]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	s := cfg.Instances[0].Startup
	if s == nil {
		t.Fatal("expected Startup config to be non-nil")
	}
	if s.LogPath != `C:\mc-forge\logs\latest.log` || s.BootPattern != "Done (" {
		t.Fatalf("unexpected startup config: %+v", s)
	}
	if len(s.Commands) != 2 || s.Commands[0] != "difficulty hard" || s.Commands[1] != "gamerule playersSleepingPercentage 10" {
		t.Fatalf("unexpected startup commands: %+v", s.Commands)
	}
}

func TestLoadWithoutStartupConfigLeavesStartupNil(t *testing.T) {
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
	if cfg.Instances[0].Startup != nil {
		t.Fatalf("expected nil Startup config when [instances.startup_config] is absent, got %+v", cfg.Instances[0].Startup)
	}
}

func TestLoadParsesGamesSectionWithStartupConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gameops.toml")
	content := `
[games.minecraft]
[games.minecraft.startup_config]
boot_pattern = "Done ("
commands = ["difficulty hard"]

[[instances]]
name = "servermc1"
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
	game, ok := cfg.Games["minecraft"]
	if !ok {
		t.Fatal("expected games[\"minecraft\"] to exist")
	}
	if game.Startup == nil || game.Startup.BootPattern != "Done (" || len(game.Startup.Commands) != 1 {
		t.Fatalf("unexpected game startup_config: %+v", game.Startup)
	}
}
