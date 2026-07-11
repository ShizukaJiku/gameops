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
