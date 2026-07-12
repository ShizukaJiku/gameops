package config

import "github.com/BurntSushi/toml"

type Config struct {
	Instances []InstanceConfig        `toml:"instances"`
	Games     map[string]GameDefaults `toml:"games"`
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
	Startup                    *StartupConfig          `toml:"startup_config"`
	WorldRegen                 *WorldRegenConfig       `toml:"world_regen_config"`
}

// GameDefaults holds config values shared by every instance of a given game
// (e.g. "minecraft"), set under [games.<name>] and merged into each
// instance that references that game via InstanceConfig.Game. Only fields
// that make sense to share across instances of the same game are here —
// per-instance specifics like ListenPort, BackendPort, and
// ForgePropertiesPath live only on InstanceConfig / its sub-configs.
type GameDefaults struct {
	IdleTimeoutMinutes  int                     `toml:"idle_timeout_minutes"`
	PollIntervalSeconds int                     `toml:"poll_interval_seconds"`
	StartCommand        string                  `toml:"start_command"`
	Minecraft           *MinecraftAdapterConfig `toml:"minecraft_config"`
	Backup              *BackupConfig           `toml:"backup_config"`
	Maintenance         *MaintenanceConfig      `toml:"maintenance_config"`
	Startup             *StartupConfig          `toml:"startup_config"`
	WorldRegen          *WorldRegenConfig       `toml:"world_regen_config"`
}

type MinecraftAdapterConfig struct {
	RconPort int `toml:"rcon_port"`
	// RconPassword can technically be set under [games.<name>] and inherited
	// by every instance of that game, but sharing one RCON password across
	// instances means a leak on one compromises all of them — prefer setting
	// it per-instance even though the merge doesn't enforce that.
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

// StartupConfig configures the `startup apply` subcommand for this
// instance. LogPath is the only field with no built-in default: when empty,
// startup.Apply skips waiting on a boot-log pattern entirely and goes
// straight to the RCON-ready check (fully game-agnostic default). BootPattern
// defaults to "Done (" when empty (Forge's own boot-complete log line) — see
// startup.resolveStartupConfig. Commands with zero entries means nothing to
// apply (no-op), not an error.
type StartupConfig struct {
	LogPath     string   `toml:"log_path"`
	BootPattern string   `toml:"boot_pattern"`
	Commands    []string `toml:"commands"`
}

// SeedTemplateFile copies Src (an opaque file — no parsing, just bytes) into
// the fresh world directory at Dest, a path relative to WorldRegenConfig's
// WorldPath (e.g. "data/betterzombieai_mapvars.dat"). Used to pre-seed
// mod-specific state that would otherwise reset itself on first load of a
// new world (see betterzombieai_mapvars_template.dat in ARCHITECTURE.md).
type SeedTemplateFile struct {
	Src  string `toml:"src"`
	Dest string `toml:"dest"`
}

// WorldRegenConfig configures the `world regen` subcommand for this
// instance. Only WorldPath and SeedKey have built-in defaults — see
// worldregen.resolveWorldRegenConfig. ServerPropertiesPath empty means the
// seed is never touched even with --new-seed (nothing to edit). Empty
// ExtraResetFiles/SeedTemplateFiles means nothing to reset/seed — both are
// no-ops, not errors.
type WorldRegenConfig struct {
	WorldPath            string             `toml:"world_path"`
	ServerPropertiesPath string             `toml:"server_properties_path"`
	SeedKey              string             `toml:"seed_key"`
	ExtraResetFiles      []string           `toml:"extra_reset_files"`
	SeedTemplateFiles    []SeedTemplateFile `toml:"seed_template_files"`
}

// Load reads and parses a gameops TOML config file, then merges each
// instance's [games.<name>] defaults (if any) into it — see
// mergeInstanceConfig. Callers always get back fully-resolved instances;
// the game-defaults layer is invisible past this point.
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	for i := range cfg.Instances {
		cfg.Instances[i] = mergeInstanceConfig(cfg.Instances[i], cfg.Games)
	}
	return &cfg, nil
}
