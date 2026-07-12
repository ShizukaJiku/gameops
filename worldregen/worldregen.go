package worldregen

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
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

// renameWorldIfExists renames worldPath to "worldPath_prev_<timestamp>" if
// it exists. A no-op (not an error) if worldPath doesn't exist yet — Forge
// creates a fresh world on first boot regardless.
func renameWorldIfExists(worldPath string, now time.Time) error {
	if _, err := os.Stat(worldPath); os.IsNotExist(err) {
		return nil
	}
	backupPath := worldPath + "_prev_" + now.Format("20060102_150405")
	if err := os.Rename(worldPath, backupPath); err != nil {
		return fmt.Errorf("worldregen: rename %s: %w", worldPath, err)
	}
	return nil
}

// resetExtraFile backs up path (if it exists) to
// "<name-without-extension>_prev_<timestamp><extension>" alongside it, then
// overwrites the original with "{}". A no-op if path doesn't exist.
func resetExtraFile(path string, now time.Time) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	backupPath := base + "_prev_" + now.Format("20060102_150405") + ext

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("worldregen: read %s: %w", path, err)
	}
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("worldregen: backup %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		return fmt.Errorf("worldregen: reset %s: %w", path, err)
	}
	return nil
}

// copySeedTemplate copies tf.Src to filepath.Join(worldPath, tf.Dest),
// creating the destination's parent directory if needed. A no-op if tf.Src
// doesn't exist. The file's content is never inspected — it's opaque bytes,
// same as the original script's raw Copy-Item.
func copySeedTemplate(worldPath string, tf config.SeedTemplateFile) error {
	if _, err := os.Stat(tf.Src); os.IsNotExist(err) {
		return nil
	}
	dest := filepath.Join(worldPath, tf.Dest)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("worldregen: mkdir for %s: %w", dest, err)
	}
	src, err := os.Open(tf.Src)
	if err != nil {
		return fmt.Errorf("worldregen: open %s: %w", tf.Src, err)
	}
	defer src.Close()
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("worldregen: create %s: %w", dest, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, src); err != nil {
		return fmt.Errorf("worldregen: copy to %s: %w", dest, err)
	}
	return nil
}

// Regen regenerates cfg's world: verifies the backend isn't running,
// optionally blanks the configured seed (newSeed=true), renames the
// existing world directory out of the way, resets configured extra
// progress files, and seeds configured template files into the fresh world
// directory. Returns an error if the backend is still running, or if any
// I/O step fails — unlike startup.Apply, there is no "log and continue"
// here: a failure partway through must stop the whole operation rather
// than leave a half-prepared world (see design spec §3.3).
func Regen(cfg config.InstanceConfig, newSeed bool) error {
	return regenAt(cfg, newSeed, time.Now())
}

// regenAt is Regen with an injectable clock, so tests can assert on the
// exact timestamped backup paths without depending on wall-clock time.
func regenAt(cfg config.InstanceConfig, newSeed bool, now time.Time) error {
	if backendReachable(cfg.BackendPort) {
		return fmt.Errorf("worldregen: backend still running on port %d, stop it first", cfg.BackendPort)
	}

	worldPath, propsPath, seedKey, extraResetFiles, seedTemplateFiles := resolveWorldRegenConfig(cfg.WorldRegen)

	if newSeed {
		if err := blankSeedLine(propsPath, seedKey); err != nil {
			return err
		}
	}

	if err := renameWorldIfExists(worldPath, now); err != nil {
		return err
	}

	for _, f := range extraResetFiles {
		if err := resetExtraFile(f, now); err != nil {
			return err
		}
	}

	for _, tf := range seedTemplateFiles {
		if err := copySeedTemplate(worldPath, tf); err != nil {
			return err
		}
	}

	return nil
}
