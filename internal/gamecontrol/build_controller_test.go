package gamecontrol

import (
	"testing"

	"github.com/ShizukaJiku/gameops/internal/config"
)

func TestBuildControllerMinecraft(t *testing.T) {
	cfg := config.InstanceConfig{Name: "test", Game: "minecraft", Minecraft: &config.MinecraftAdapterConfig{RconPort: 25575}}
	gc, err := BuildController(cfg)
	if err != nil {
		t.Fatalf("BuildController error: %v", err)
	}
	if _, ok := gc.(*MinecraftController); !ok {
		t.Fatalf("expected *MinecraftController, got %T", gc)
	}
}

func TestBuildControllerMissingMinecraftConfig(t *testing.T) {
	cfg := config.InstanceConfig{Name: "test", Game: "minecraft"}
	if _, err := BuildController(cfg); err == nil {
		t.Fatal("expected error when minecraft_config is missing")
	}
}

func TestBuildControllerUnknownGame(t *testing.T) {
	cfg := config.InstanceConfig{Name: "test", Game: "palworld"}
	if _, err := BuildController(cfg); err == nil {
		t.Fatal("expected error for unknown game")
	}
}
