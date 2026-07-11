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
