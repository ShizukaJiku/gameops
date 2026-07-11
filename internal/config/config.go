package config

import "github.com/BurntSushi/toml"

type Config struct {
	Instances []InstanceConfig `toml:"instances"`
}

type InstanceConfig struct {
	Name                       string                  `toml:"name"`
	Game                       string                  `toml:"game"`
	ListenPort                 int                     `toml:"listen_port"`
	BackendPort                int                     `toml:"backend_port"`
	IdleTimeoutMinutes         int                     `toml:"idle_timeout_minutes"`
	PollIntervalSeconds        int                     `toml:"poll_interval_seconds"`
	BackendReadyTimeoutMinutes int                     `toml:"backend_ready_timeout_minutes"`
	StartCommand               string                  `toml:"start_command"`
	Minecraft                  *MinecraftAdapterConfig `toml:"minecraft_config"`
	Backup                     *BackupConfig           `toml:"backup_config"`
	Maintenance                *MaintenanceConfig      `toml:"maintenance_config"`
}

type MinecraftAdapterConfig struct {
	RconPort     int    `toml:"rcon_port"`
	RconPassword string `toml:"rcon_password"`
	// ForgePropertiesPath points at Forge's server.properties, read live on
	// every status/kick response for the real motd — defaults to
	// C:\mc-forge\server.properties when empty (see idlewatch.NewMinecraftAdapter).
	ForgePropertiesPath string `toml:"forge_properties_path"`
	// MotdFallback is used only if ForgePropertiesPath can't be read (file
	// missing, no motd= line, etc).
	MotdFallback string `toml:"motd_fallback"`
}

// BackupConfig configures the `backup run` subcommand for this instance.
// All three fields default when empty/zero — see backup.resolveBackupConfig.
type BackupConfig struct {
	WorldPath  string `toml:"world_path"`
	BackupsDir string `toml:"backups_dir"`
	MaxBackups int    `toml:"max_backups"`
}

// MaintenanceConfig configures the `maintenance stop`/`maintenance resume`
// subcommands for this instance. Both fields default when empty — see
// maintenance.resolveMaintenanceConfig. ProcessName and StopCommand are the
// two points of variation between games (e.g. Minecraft's "java"/"stop"
// versus a future second game's own process name and shutdown command) —
// keeping them config-driven here is what lets a new game be added without
// changing this package's code.
type MaintenanceConfig struct {
	ProcessName string `toml:"process_name"`
	StopCommand string `toml:"stop_command"`
}

// Load reads and parses a gameops TOML config file.
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
