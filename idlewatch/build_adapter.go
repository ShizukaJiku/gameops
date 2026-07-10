package idlewatch

import "github.com/ShizukaJiku/gameops/internal/config"

// BuildAdapter constructs the Adapter for an instance based on cfg.Adapter.
// Currently only "minecraft" is supported — additional adapters are added
// here as new games are onboarded.
func BuildAdapter(cfg config.InstanceConfig) (Adapter, error) {
	switch cfg.Adapter {
	case "minecraft":
		if cfg.Minecraft == nil {
			return nil, errAdapterConfigMissing(cfg.Name, "minecraft_config")
		}
		return NewMinecraftAdapter(cfg.Minecraft), nil
	default:
		return nil, errUnknownAdapter(cfg.Adapter)
	}
}

func errAdapterConfigMissing(instance, section string) error {
	return &adapterConfigError{instance: instance, section: section}
}

type adapterConfigError struct {
	instance, section string
}

func (e *adapterConfigError) Error() string {
	return "instance " + e.instance + " uses adapter requiring [" + e.section + "] but it is missing"
}

func errUnknownAdapter(name string) error {
	return &unknownAdapterError{name: name}
}

type unknownAdapterError struct{ name string }

func (e *unknownAdapterError) Error() string {
	return "unknown adapter: " + e.name
}
