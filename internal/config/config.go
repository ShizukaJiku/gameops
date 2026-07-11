package config

import "github.com/BurntSushi/toml"

type Config struct {
	Instances []InstanceConfig `toml:"instances"`
}

type InstanceConfig struct {
	Name                       string                  `toml:"name"`
	Adapter                    string                  `toml:"adapter"`
	ListenPort                 int                     `toml:"listen_port"`
	BackendPort                int                     `toml:"backend_port"`
	IdleTimeoutMinutes         int                     `toml:"idle_timeout_minutes"`
	PollIntervalSeconds        int                     `toml:"poll_interval_seconds"`
	BackendReadyTimeoutMinutes int                     `toml:"backend_ready_timeout_minutes"`
	StartCommand               string                  `toml:"start_command"`
	Minecraft                  *MinecraftAdapterConfig `toml:"minecraft_config"`
	Backup                     *BackupConfig           `toml:"backup_config"`
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

// Load reads and parses a gameops TOML config file.
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
