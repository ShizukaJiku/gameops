package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
	"github.com/ShizukaJiku/gameops/internal/rcon"
)

const (
	defaultWorldPath  = `C:\mc-forge\world`
	defaultBackupsDir = `C:\mc-forge\backups`
	defaultMaxBackups = 12
)

// resolveBackupConfig applies defaults to a possibly-nil or partially-set
// BackupConfig, the same pattern idlewatch.NewMinecraftAdapter uses for
// ForgePropertiesPath.
func resolveBackupConfig(cfg *config.BackupConfig) (worldPath, backupsDir string, maxBackups int) {
	worldPath = defaultWorldPath
	backupsDir = defaultBackupsDir
	maxBackups = defaultMaxBackups
	if cfg == nil {
		return
	}
	if cfg.WorldPath != "" {
		worldPath = cfg.WorldPath
	}
	if cfg.BackupsDir != "" {
		backupsDir = cfg.BackupsDir
	}
	if cfg.MaxBackups != 0 {
		maxBackups = cfg.MaxBackups
	}
	return
}

// zipWorld walks worldPath and writes every file (except session.lock, which
// Forge holds open exclusively while running) into a zip archive at
// destPath, with paths inside the archive rooted at "world/" rather than at
// worldPath's own directory name — so extracting the zip always produces a
// clean "world/" folder regardless of what the source directory happens to
// be called on disk.
func zipWorld(worldPath, destPath string) error {
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)

	walkErr := filepath.WalkDir(worldPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == "session.lock" {
			return nil
		}

		rel, err := filepath.Rel(worldPath, path)
		if err != nil {
			return err
		}
		zipPath := filepath.ToSlash(filepath.Join("world", rel))

		zf, err := zw.Create(zipPath)
		if err != nil {
			return err
		}

		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		_, err = io.Copy(zf, src)
		return err
	})
	if walkErr != nil {
		zw.Close() // best-effort — destPath is about to be removed by the caller anyway
		return walkErr
	}

	return zw.Close()
}

// writeBackupZip zips worldPath into a fresh temp file inside backupsDir
// and, on success, renames it to its final world_<timestamp>.zip name.
// Never leaves a stray .tmp file behind on failure, and never produces a
// final .zip file unless the zip completed successfully.
func writeBackupZip(worldPath, backupsDir, timestamp string) (finalPath string, err error) {
	finalPath = filepath.Join(backupsDir, fmt.Sprintf("world_%s.zip", timestamp))
	tmpPath := finalPath + ".tmp"

	if err := zipWorld(worldPath, tmpPath); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return finalPath, nil
}

// rotate keeps only the maxBackups most recently modified world_*.zip files
// in dir, deleting the rest. Files with a ".tmp" suffix are never matched by
// the "world_*.zip" glob, so a leftover temp file from an interrupted run is
// never a rotation candidate.
func rotate(dir string, maxBackups int) error {
	matches, err := filepath.Glob(filepath.Join(dir, "world_*.zip"))
	if err != nil {
		return err
	}

	type backupFile struct {
		path    string
		modTime time.Time
	}
	files := make([]backupFile, 0, len(matches))
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		files = append(files, backupFile{path: m, modTime: info.ModTime()})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	if len(files) <= maxBackups {
		return nil
	}

	for _, f := range files[maxBackups:] {
		if err := os.Remove(f.path); err != nil {
			return err
		}
		log.Printf("old backup deleted: %s", f.path)
	}
	return nil
}

// trySaveOff attempts save-off + save-all flush over RCON. It returns true
// only if save-off itself succeeded — the caller uses that as the signal to
// call trySaveOn later, regardless of whether save-all flush also
// succeeded. This is the fix for a bug in the original PowerShell script:
// there, save-on was only restored if BOTH commands succeeded, so a
// save-off that worked followed by a failing save-all flush left autosave
// permanently disabled until the next server restart.
func trySaveOff(mc *config.MinecraftAdapterConfig) bool {
	c, err := rcon.Dial(fmt.Sprintf("127.0.0.1:%d", mc.RconPort), mc.RconPassword, 5*time.Second)
	if err != nil {
		log.Printf("backup: RCON unavailable (%v), backing up without save-off", err)
		return false
	}
	defer c.Close()

	if _, err := c.Command("save-off"); err != nil {
		log.Printf("backup: save-off failed (%v), backing up without save-off", err)
		return false
	}

	if _, err := c.Command("save-all flush"); err != nil {
		log.Printf("backup: save-all flush failed (%v), continuing anyway", err)
	}

	return true
}

// trySaveOn restores autosave. Called via defer whenever trySaveOff
// reported save-off succeeded, regardless of what happened afterward.
func trySaveOn(mc *config.MinecraftAdapterConfig) {
	c, err := rcon.Dial(fmt.Sprintf("127.0.0.1:%d", mc.RconPort), mc.RconPassword, 5*time.Second)
	if err != nil {
		log.Printf("backup: RCON unavailable for save-on (%v) — autosave may still be disabled, check manually", err)
		return
	}
	defer c.Close()
	if _, err := c.Command("save-on"); err != nil {
		log.Printf("backup: save-on failed (%v) — autosave may still be disabled, check manually", err)
	}
}

// Run performs one backup of cfg's configured world: attempts save-off/
// save-all flush via RCON (never an error if RCON is unreachable — only a
// failure to actually write the backup zip is), zips the world directory to
// backupsDir, then rotates old backups down to maxBackups. Returns the path
// of the created backup, or "" if there was nothing to back up (world
// directory doesn't exist).
func Run(cfg config.InstanceConfig) (string, error) {
	return runAt(cfg, time.Now())
}

// runAt is Run with an injectable timestamp, so tests can call it multiple
// times in a tight loop with distinct timestamps without colliding on the
// same second-granularity backup filename.
func runAt(cfg config.InstanceConfig, now time.Time) (string, error) {
	worldPath, backupsDir, maxBackups := resolveBackupConfig(cfg.Backup)

	if _, err := os.Stat(worldPath); os.IsNotExist(err) {
		log.Printf("no world directory at %s, nothing to back up", worldPath)
		return "", nil
	}

	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		return "", fmt.Errorf("backup: create backups dir: %w", err)
	}

	if cfg.Minecraft != nil && trySaveOff(cfg.Minecraft) {
		defer trySaveOn(cfg.Minecraft)
	}

	finalPath, err := writeBackupZip(worldPath, backupsDir, now.Format("20060102_150405"))
	if err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	log.Printf("backup created: %s", finalPath)

	if err := rotate(backupsDir, maxBackups); err != nil {
		return finalPath, fmt.Errorf("backup: rotation: %w", err)
	}

	return finalPath, nil
}
