package worldregen

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
)

const (
	defaultWorldPath = `C:\mc-forge\world`
	defaultSeedKey   = "level-seed"
)

// resolveWorldRegenConfig applies defaults to a possibly-nil or
// partially-set WorldRegenConfig, same pattern as backup.resolveBackupConfig.
// ServerPropertiesPath, ExtraResetFiles, and SeedTemplateFiles have no
// built-in default — empty/nil simply skips that step in Regen, it's not an
// error.
func resolveWorldRegenConfig(cfg *config.WorldRegenConfig) (worldPath, propsPath, seedKey string, extraResetFiles []string, seedTemplateFiles []config.SeedTemplateFile) {
	worldPath = defaultWorldPath
	seedKey = defaultSeedKey
	if cfg == nil {
		return
	}
	if cfg.WorldPath != "" {
		worldPath = cfg.WorldPath
	}
	propsPath = cfg.ServerPropertiesPath
	if cfg.SeedKey != "" {
		seedKey = cfg.SeedKey
	}
	extraResetFiles = cfg.ExtraResetFiles
	seedTemplateFiles = cfg.SeedTemplateFiles
	return
}

// backendReachable reports whether a TCP connection to 127.0.0.1:backendPort
// succeeds right now — same minimal check as maintenance.backendReachable,
// duplicated here per the project's established non-extraction stance (see
// design spec §3.3).
func backendReachable(backendPort int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", backendPort), 2*time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// blankSeedLine rewrites propsPath, replacing the line that starts with
// "seedKey=" with "seedKey=" (empty value) — Minecraft treats an empty
// level-seed as "pick a new random one at world creation", matching the
// original script's PowerShell -replace behavior. A no-op if propsPath is
// empty (nothing configured to edit); an error if propsPath is set but
// unreadable.
func blankSeedLine(propsPath, seedKey string) error {
	if propsPath == "" {
		return nil
	}
	data, err := os.ReadFile(propsPath)
	if err != nil {
		return fmt.Errorf("worldregen: read %s: %w", propsPath, err)
	}
	prefix := seedKey + "="
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if strings.HasPrefix(trimmed, prefix) {
			trailing := ""
			if strings.HasSuffix(line, "\r") {
				trailing = "\r"
			}
			lines[i] = prefix + trailing
		}
	}
	if err := os.WriteFile(propsPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("worldregen: write %s: %w", propsPath, err)
	}
	return nil
}
