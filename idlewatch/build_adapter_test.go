package idlewatch

import (
	"testing"

	"github.com/ShizukaJiku/gameops/internal/config"
)

func TestBuildAdapterMinecraft(t *testing.T) {
	cfg := config.InstanceConfig{
		Name:      "test",
		Game:   "minecraft",
		Minecraft: &config.MinecraftAdapterConfig{RconPort: 25575},
	}
	a, err := BuildAdapter(cfg)
	if err != nil {
		t.Fatalf("BuildAdapter error: %v", err)
	}
	if _, ok := a.(*MinecraftAdapter); !ok {
		t.Fatalf("expected *MinecraftAdapter, got %T", a)
	}
}

func TestBuildAdapterMissingMinecraftConfig(t *testing.T) {
	cfg := config.InstanceConfig{Name: "test", Game: "minecraft"}
	if _, err := BuildAdapter(cfg); err == nil {
		t.Fatal("expected error when minecraft_config is missing")
	}
}

func TestBuildAdapterUnknownAdapter(t *testing.T) {
	cfg := config.InstanceConfig{Name: "test", Game: "bogus"}
	if _, err := BuildAdapter(cfg); err == nil {
		t.Fatal("expected error for unknown adapter")
	}
}
