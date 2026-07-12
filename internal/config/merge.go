package config

// mergeMinecraftConfig combines an instance's MinecraftAdapterConfig with
// its game's defaults, field by field: an instance value wins whenever it
// is non-empty/non-zero, otherwise the game's value is used. Returns nil
// only when both inputs are nil (meaning: no minecraft_config at all,
// downstream packages already handle that via their own resolve* defaults).
func mergeMinecraftConfig(instance, game *MinecraftAdapterConfig) *MinecraftAdapterConfig {
	if instance == nil && game == nil {
		return nil
	}
	merged := &MinecraftAdapterConfig{}
	if instance != nil {
		*merged = *instance
	}
	if game != nil {
		if merged.RconPort == 0 {
			merged.RconPort = game.RconPort
		}
		if merged.RconPassword == "" {
			merged.RconPassword = game.RconPassword
		}
		if merged.ForgePropertiesPath == "" {
			merged.ForgePropertiesPath = game.ForgePropertiesPath
		}
		if merged.MotdFallback == "" {
			merged.MotdFallback = game.MotdFallback
		}
	}
	return merged
}

// mergeBackupConfig combines an instance's BackupConfig with its game's
// defaults, same per-field precedence as mergeMinecraftConfig.
func mergeBackupConfig(instance, game *BackupConfig) *BackupConfig {
	if instance == nil && game == nil {
		return nil
	}
	merged := &BackupConfig{}
	if instance != nil {
		*merged = *instance
	}
	if game != nil {
		if merged.WorldPath == "" {
			merged.WorldPath = game.WorldPath
		}
		if merged.BackupsDir == "" {
			merged.BackupsDir = game.BackupsDir
		}
		if merged.MaxBackups == 0 {
			merged.MaxBackups = game.MaxBackups
		}
	}
	return merged
}

// mergeMaintenanceConfig combines an instance's MaintenanceConfig with its
// game's defaults, same per-field precedence as mergeMinecraftConfig.
func mergeMaintenanceConfig(instance, game *MaintenanceConfig) *MaintenanceConfig {
	if instance == nil && game == nil {
		return nil
	}
	merged := &MaintenanceConfig{}
	if instance != nil {
		*merged = *instance
	}
	if game != nil {
		if merged.ProcessName == "" {
			merged.ProcessName = game.ProcessName
		}
		if merged.StopCommand == "" {
			merged.StopCommand = game.StopCommand
		}
	}
	return merged
}

// mergeStartupConfig combines an instance's StartupConfig with its game's
// defaults, same per-field precedence as mergeMinecraftConfig. Commands is a
// slice — "empty" means len == 0, treated the same way as a zero int: the
// instance's slice wins only if it has at least one entry, otherwise the
// game's slice (if any) is used.
func mergeStartupConfig(instance, game *StartupConfig) *StartupConfig {
	if instance == nil && game == nil {
		return nil
	}
	merged := &StartupConfig{}
	if instance != nil {
		*merged = *instance
	}
	if game != nil {
		if merged.LogPath == "" {
			merged.LogPath = game.LogPath
		}
		if merged.BootPattern == "" {
			merged.BootPattern = game.BootPattern
		}
		if len(merged.Commands) == 0 {
			merged.Commands = game.Commands
		}
	}
	return merged
}

// mergeWorldRegenConfig combines an instance's WorldRegenConfig with its
// game's defaults, same per-field precedence as mergeMinecraftConfig.
// ExtraResetFiles and SeedTemplateFiles are slices — "empty" means len == 0,
// same rule as StartupConfig.Commands.
func mergeWorldRegenConfig(instance, game *WorldRegenConfig) *WorldRegenConfig {
	if instance == nil && game == nil {
		return nil
	}
	merged := &WorldRegenConfig{}
	if instance != nil {
		*merged = *instance
	}
	if game != nil {
		if merged.WorldPath == "" {
			merged.WorldPath = game.WorldPath
		}
		if merged.ServerPropertiesPath == "" {
			merged.ServerPropertiesPath = game.ServerPropertiesPath
		}
		if merged.SeedKey == "" {
			merged.SeedKey = game.SeedKey
		}
		if len(merged.ExtraResetFiles) == 0 {
			merged.ExtraResetFiles = game.ExtraResetFiles
		}
		if len(merged.SeedTemplateFiles) == 0 {
			merged.SeedTemplateFiles = game.SeedTemplateFiles
		}
	}
	return merged
}

// mergeInstanceConfig merges inst's [games.<inst.Game>] defaults (if any)
// into inst, per-field, instance value winning whenever non-empty/non-zero.
// If inst.Game has no matching entry in games, inst is returned unchanged.
func mergeInstanceConfig(inst InstanceConfig, games map[string]GameDefaults) InstanceConfig {
	game, ok := games[inst.Game]
	if !ok {
		return inst
	}
	if inst.IdleTimeoutMinutes == 0 {
		inst.IdleTimeoutMinutes = game.IdleTimeoutMinutes
	}
	if inst.PollIntervalSeconds == 0 {
		inst.PollIntervalSeconds = game.PollIntervalSeconds
	}
	if inst.StartCommand == "" {
		inst.StartCommand = game.StartCommand
	}
	inst.Minecraft = mergeMinecraftConfig(inst.Minecraft, game.Minecraft)
	inst.Backup = mergeBackupConfig(inst.Backup, game.Backup)
	inst.Maintenance = mergeMaintenanceConfig(inst.Maintenance, game.Maintenance)
	inst.Startup = mergeStartupConfig(inst.Startup, game.Startup)
	inst.WorldRegen = mergeWorldRegenConfig(inst.WorldRegen, game.WorldRegen)
	return inst
}
