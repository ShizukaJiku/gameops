package backup

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
)

func TestResolveBackupConfigAppliesAllDefaultsWhenNil(t *testing.T) {
	worldPath, backupsDir, maxBackups := resolveBackupConfig(nil)
	if worldPath != defaultWorldPath || backupsDir != defaultBackupsDir || maxBackups != defaultMaxBackups {
		t.Fatalf("expected all defaults, got worldPath=%q backupsDir=%q maxBackups=%d", worldPath, backupsDir, maxBackups)
	}
}

func TestResolveBackupConfigAppliesPartialDefaults(t *testing.T) {
	cfg := &config.BackupConfig{WorldPath: `C:\custom\world`}
	worldPath, backupsDir, maxBackups := resolveBackupConfig(cfg)
	if worldPath != `C:\custom\world` {
		t.Fatalf("expected custom worldPath to be kept, got %q", worldPath)
	}
	if backupsDir != defaultBackupsDir {
		t.Fatalf("expected default backupsDir, got %q", backupsDir)
	}
	if maxBackups != defaultMaxBackups {
		t.Fatalf("expected default maxBackups, got %d", maxBackups)
	}
}

func TestResolveBackupConfigKeepsAllFieldsWhenSet(t *testing.T) {
	cfg := &config.BackupConfig{WorldPath: "w", BackupsDir: "b", MaxBackups: 3}
	worldPath, backupsDir, maxBackups := resolveBackupConfig(cfg)
	if worldPath != "w" || backupsDir != "b" || maxBackups != 3 {
		t.Fatalf("expected all custom values kept, got worldPath=%q backupsDir=%q maxBackups=%d", worldPath, backupsDir, maxBackups)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func zipEntryNames(t *testing.T, zipPath string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer r.Close()

	out := make(map[string]string)
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", f.Name, err)
		}
		out[f.Name] = string(content)
	}
	return out
}

func TestZipWorldWritesFilesWithWorldPrefix(t *testing.T) {
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	writeFile(t, filepath.Join(worldDir, "level.dat"), "level-data")
	writeFile(t, filepath.Join(worldDir, "region", "r.0.0.mca"), "region-data")

	dest := filepath.Join(dir, "out.zip")
	if err := zipWorld(worldDir, dest); err != nil {
		t.Fatalf("zipWorld error: %v", err)
	}

	entries := zipEntryNames(t, dest)
	if entries["world/level.dat"] != "level-data" {
		t.Fatalf("expected world/level.dat entry, got entries: %v", entries)
	}
	if entries["world/region/r.0.0.mca"] != "region-data" {
		t.Fatalf("expected world/region/r.0.0.mca entry, got entries: %v", entries)
	}
}

func TestZipWorldSkipsSessionLock(t *testing.T) {
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	writeFile(t, filepath.Join(worldDir, "level.dat"), "level-data")
	writeFile(t, filepath.Join(worldDir, "session.lock"), "lock-bytes")

	dest := filepath.Join(dir, "out.zip")
	if err := zipWorld(worldDir, dest); err != nil {
		t.Fatalf("zipWorld error: %v", err)
	}

	entries := zipEntryNames(t, dest)
	if _, ok := entries["world/session.lock"]; ok {
		t.Fatalf("expected session.lock to be excluded, got entries: %v", entries)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 entry (level.dat), got: %v", entries)
	}
}

func TestZipWorldReturnsErrorForNonexistentRoot(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.zip")
	err := zipWorld(filepath.Join(dir, "does-not-exist"), dest)
	if err == nil {
		t.Fatal("expected error for nonexistent world root, got nil")
	}
}

func TestWriteBackupZipProducesFinalNamedFileAndNoTmpLeftover(t *testing.T) {
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	writeFile(t, filepath.Join(worldDir, "level.dat"), "level-data")
	backupsDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		t.Fatal(err)
	}

	finalPath, err := writeBackupZip(worldDir, backupsDir, "20260101_010101")
	if err != nil {
		t.Fatalf("writeBackupZip error: %v", err)
	}
	wantPath := filepath.Join(backupsDir, "world_20260101_010101.zip")
	if finalPath != wantPath {
		t.Fatalf("expected final path %q, got %q", wantPath, finalPath)
	}
	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf("expected final zip to exist: %v", err)
	}
	if _, err := os.Stat(finalPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected no .tmp file left behind, stat error: %v", err)
	}
}

func TestWriteBackupZipCleansUpTmpFileOnFailure(t *testing.T) {
	dir := t.TempDir()
	backupsDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		t.Fatal(err)
	}
	nonexistentWorld := filepath.Join(dir, "does-not-exist")

	_, err := writeBackupZip(nonexistentWorld, backupsDir, "20260101_010101")
	if err == nil {
		t.Fatal("expected error for nonexistent world root, got nil")
	}

	finalPath := filepath.Join(backupsDir, "world_20260101_010101.zip")
	if _, statErr := os.Stat(finalPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no final .zip to exist after failure, stat error: %v", statErr)
	}
	if _, statErr := os.Stat(finalPath + ".tmp"); !os.IsNotExist(statErr) {
		t.Fatalf("expected no .tmp file left behind after failure, stat error: %v", statErr)
	}
}

func setBackupFileMtime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestRotateKeepsOnlyMaxBackupsMostRecent(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	names := []string{
		"world_20260101_010101.zip", // oldest
		"world_20260102_010101.zip",
		"world_20260103_010101.zip",
		"world_20260104_010101.zip", // newest
	}
	for i, name := range names {
		path := filepath.Join(dir, name)
		writeFile(t, path, "zip-bytes")
		setBackupFileMtime(t, path, base.AddDate(0, 0, i))
	}

	if err := rotate(dir, 2); err != nil {
		t.Fatalf("rotate error: %v", err)
	}

	remaining, err := filepath.Glob(filepath.Join(dir, "world_*.zip"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(remaining)
	want := []string{
		filepath.Join(dir, "world_20260103_010101.zip"),
		filepath.Join(dir, "world_20260104_010101.zip"),
	}
	sort.Strings(want)
	if len(remaining) != len(want) {
		t.Fatalf("expected %v to remain, got %v", want, remaining)
	}
	for i := range want {
		if remaining[i] != want[i] {
			t.Fatalf("expected %v to remain, got %v", want, remaining)
		}
	}
}

func TestRotateNoOpWhenUnderLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "world_20260101_010101.zip")
	writeFile(t, path, "zip-bytes")

	if err := rotate(dir, 12); err != nil {
		t.Fatalf("rotate error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the single backup to remain untouched: %v", err)
	}
}
