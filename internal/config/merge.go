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
