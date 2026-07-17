package gamecontrol

import (
	"fmt"

	"github.com/ShizukaJiku/gameops/internal/config"
)

// BuildController constructs the GameController for an instance based on
// cfg.Game — the same dispatch shape as idlewatch.BuildAdapter. Additional
// games are added here as new cases, each implementing GameController
// against their own protocol.
func BuildController(cfg config.InstanceConfig) (GameController, error) {
	switch cfg.Game {
	case "minecraft":
		if cfg.Minecraft == nil {
			return nil, fmt.Errorf("gamecontrol: instance %s uses game minecraft but has no minecraft_config", cfg.Name)
		}
		return NewMinecraftController(cfg), nil
	default:
		return nil, fmt.Errorf("gamecontrol: unknown game %q for instance %s", cfg.Game, cfg.Name)
	}
}
