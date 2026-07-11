package config

import "testing"

func TestMergeMinecraftConfigBothNilReturnsNil(t *testing.T) {
	if got := mergeMinecraftConfig(nil, nil); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestMergeMinecraftConfigInstanceOnlyKeepsInstanceValues(t *testing.T) {
	instance := &MinecraftAdapterConfig{RconPort: 25575, RconPassword: "secret"}
	got := mergeMinecraftConfig(instance, nil)
	if got.RconPort != 25575 || got.RconPassword != "secret" {
		t.Fatalf("unexpected merge: %+v", got)
	}
}

func TestMergeMinecraftConfigGameOnlyFillsFromGame(t *testing.T) {
	game := &MinecraftAdapterConfig{RconPort: 25575, MotdFallback: "sleeping"}
	got := mergeMinecraftConfig(nil, game)
	if got.RconPort != 25575 || got.MotdFallback != "sleeping" {
		t.Fatalf("unexpected merge: %+v", got)
	}
}

func TestMergeMinecraftConfigInstanceWinsOverGamePerField(t *testing.T) {
	instance := &MinecraftAdapterConfig{RconPassword: "instance-secret"}
	game := &MinecraftAdapterConfig{RconPort: 25575, RconPassword: "game-secret", MotdFallback: "sleeping"}
	got := mergeMinecraftConfig(instance, game)
	if got.RconPassword != "instance-secret" {
		t.Fatalf("expected instance RconPassword to win, got %q", got.RconPassword)
	}
	if got.RconPort != 25575 {
		t.Fatalf("expected RconPort inherited from game, got %d", got.RconPort)
	}
	if got.MotdFallback != "sleeping" {
		t.Fatalf("expected MotdFallback inherited from game, got %q", got.MotdFallback)
	}
}

func TestMergeBackupConfigBothNilReturnsNil(t *testing.T) {
	if got := mergeBackupConfig(nil, nil); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestMergeBackupConfigInstanceWinsOverGamePerField(t *testing.T) {
	instance := &BackupConfig{WorldPath: `C:\instance\world`}
	game := &BackupConfig{WorldPath: `C:\game\world`, BackupsDir: `C:\game\backups`, MaxBackups: 10}
	got := mergeBackupConfig(instance, game)
	if got.WorldPath != `C:\instance\world` {
		t.Fatalf("expected instance WorldPath to win, got %q", got.WorldPath)
	}
	if got.BackupsDir != `C:\game\backups` || got.MaxBackups != 10 {
		t.Fatalf("expected BackupsDir/MaxBackups inherited from game, got %+v", got)
	}
}

func TestMergeMaintenanceConfigBothNilReturnsNil(t *testing.T) {
	if got := mergeMaintenanceConfig(nil, nil); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestMergeMaintenanceConfigInstanceWinsOverGamePerField(t *testing.T) {
	instance := &MaintenanceConfig{ProcessName: "PalServer"}
	game := &MaintenanceConfig{ProcessName: "java", StopCommand: "stop"}
	got := mergeMaintenanceConfig(instance, game)
	if got.ProcessName != "PalServer" {
		t.Fatalf("expected instance ProcessName to win, got %q", got.ProcessName)
	}
	if got.StopCommand != "stop" {
		t.Fatalf("expected StopCommand inherited from game, got %q", got.StopCommand)
	}
}

func TestMergeInstanceConfigNoMatchingGameReturnsInstanceUnchanged(t *testing.T) {
	inst := InstanceConfig{Name: "servermc1", Game: "minecraft", IdleTimeoutMinutes: 5}
	got := mergeInstanceConfig(inst, map[string]GameDefaults{})
	if got.IdleTimeoutMinutes != 5 {
		t.Fatalf("expected instance value preserved when no matching game, got %+v", got)
	}
}

func TestMergeInstanceConfigTopLevelFieldsInstanceWinsPerField(t *testing.T) {
	inst := InstanceConfig{Name: "servermc1", Game: "minecraft", IdleTimeoutMinutes: 5}
	games := map[string]GameDefaults{
		"minecraft": {IdleTimeoutMinutes: 15, PollIntervalSeconds: 30, StartCommand: "schtasks /run /tn mc-forge"},
	}
	got := mergeInstanceConfig(inst, games)
	if got.IdleTimeoutMinutes != 5 {
		t.Fatalf("expected instance IdleTimeoutMinutes to win, got %d", got.IdleTimeoutMinutes)
	}
	if got.PollIntervalSeconds != 30 {
		t.Fatalf("expected PollIntervalSeconds inherited from game, got %d", got.PollIntervalSeconds)
	}
	if got.StartCommand != "schtasks /run /tn mc-forge" {
		t.Fatalf("expected StartCommand inherited from game, got %q", got.StartCommand)
	}
}

func TestMergeInstanceConfigMergesSubConfigs(t *testing.T) {
	inst := InstanceConfig{
		Name:      "servermc1",
		Game:      "minecraft",
		Minecraft: &MinecraftAdapterConfig{RconPassword: "instance-secret"},
	}
	games := map[string]GameDefaults{
		"minecraft": {
			Minecraft:   &MinecraftAdapterConfig{RconPort: 25575},
			Backup:      &BackupConfig{MaxBackups: 10},
			Maintenance: &MaintenanceConfig{ProcessName: "java", StopCommand: "stop"},
		},
	}
	got := mergeInstanceConfig(inst, games)
	if got.Minecraft.RconPassword != "instance-secret" || got.Minecraft.RconPort != 25575 {
		t.Fatalf("unexpected merged minecraft config: %+v", got.Minecraft)
	}
	if got.Backup == nil || got.Backup.MaxBackups != 10 {
		t.Fatalf("unexpected merged backup config: %+v", got.Backup)
	}
	if got.Maintenance == nil || got.Maintenance.ProcessName != "java" {
		t.Fatalf("unexpected merged maintenance config: %+v", got.Maintenance)
	}
}

func TestMergeStartupConfigBothNilReturnsNil(t *testing.T) {
	if got := mergeStartupConfig(nil, nil); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestMergeStartupConfigInstanceWinsOverGamePerField(t *testing.T) {
	instance := &StartupConfig{LogPath: `C:\instance\latest.log`}
	game := &StartupConfig{LogPath: `C:\game\latest.log`, BootPattern: "Done (", Commands: []string{"difficulty hard"}}
	got := mergeStartupConfig(instance, game)
	if got.LogPath != `C:\instance\latest.log` {
		t.Fatalf("expected instance LogPath to win, got %q", got.LogPath)
	}
	if got.BootPattern != "Done (" {
		t.Fatalf("expected BootPattern inherited from game, got %q", got.BootPattern)
	}
	if len(got.Commands) != 1 || got.Commands[0] != "difficulty hard" {
		t.Fatalf("expected Commands inherited from game, got %+v", got.Commands)
	}
}

func TestMergeInstanceConfigMergesStartupConfig(t *testing.T) {
	inst := InstanceConfig{
		Name: "servermc1",
		Game: "minecraft",
	}
	games := map[string]GameDefaults{
		"minecraft": {
			Startup: &StartupConfig{BootPattern: "Done (", Commands: []string{"difficulty hard"}},
		},
	}
	got := mergeInstanceConfig(inst, games)
	if got.Startup == nil || got.Startup.BootPattern != "Done (" || len(got.Startup.Commands) != 1 {
		t.Fatalf("unexpected merged startup config: %+v", got.Startup)
	}
}
