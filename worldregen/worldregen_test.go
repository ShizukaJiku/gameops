package worldregen

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
)

func TestResolveWorldRegenConfigAppliesDefaultsWhenNil(t *testing.T) {
	worldPath, propsPath, seedKey, extraResetFiles, seedTemplateFiles := resolveWorldRegenConfig(nil)
	if worldPath != defaultWorldPath {
		t.Fatalf("expected default worldPath %q, got %q", defaultWorldPath, worldPath)
	}
	if propsPath != "" {
		t.Fatalf("expected empty propsPath, got %q", propsPath)
	}
	if seedKey != defaultSeedKey {
		t.Fatalf("expected default seedKey %q, got %q", defaultSeedKey, seedKey)
	}
	if extraResetFiles != nil {
		t.Fatalf("expected nil extraResetFiles, got %+v", extraResetFiles)
	}
	if seedTemplateFiles != nil {
		t.Fatalf("expected nil seedTemplateFiles, got %+v", seedTemplateFiles)
	}
}

func TestResolveWorldRegenConfigKeepsSetFieldsAndDefaultsSeedKeyOnly(t *testing.T) {
	cfg := &config.WorldRegenConfig{
		WorldPath:       `C:\custom\world`,
		ExtraResetFiles: []string{`C:\custom\lives.json`},
	}
	worldPath, _, seedKey, extraResetFiles, _ := resolveWorldRegenConfig(cfg)
	if worldPath != `C:\custom\world` {
		t.Fatalf("expected custom worldPath kept, got %q", worldPath)
	}
	if seedKey != defaultSeedKey {
		t.Fatalf("expected default seedKey when unset, got %q", seedKey)
	}
	if len(extraResetFiles) != 1 || extraResetFiles[0] != `C:\custom\lives.json` {
		t.Fatalf("expected custom extraResetFiles kept, got %+v", extraResetFiles)
	}
}

func TestResolveWorldRegenConfigKeepsCustomSeedKey(t *testing.T) {
	cfg := &config.WorldRegenConfig{SeedKey: "custom-seed-key"}
	_, _, seedKey, _, _ := resolveWorldRegenConfig(cfg)
	if seedKey != "custom-seed-key" {
		t.Fatalf("expected custom seedKey kept, got %q", seedKey)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestBackendReachableTrueWhenListening(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if !backendReachable(port) {
		t.Fatal("expected backendReachable to be true for a listening port")
	}
}

func TestBackendReachableFalseWhenNothingListening(t *testing.T) {
	port := freeTCPPort(t)
	if backendReachable(port) {
		t.Fatal("expected backendReachable to be false for a port nothing listens on")
	}
}

func TestBlankSeedLineIsNoOpWhenPropsPathEmpty(t *testing.T) {
	if err := blankSeedLine("", "level-seed"); err != nil {
		t.Fatalf("expected nil error for empty propsPath, got %v", err)
	}
}

func TestBlankSeedLineBlanksMatchingLine(t *testing.T) {
	dir := t.TempDir()
	propsPath := filepath.Join(dir, "server.properties")
	content := "motd=hello\nlevel-seed=-12345\ndifficulty=hard\n"
	if err := os.WriteFile(propsPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := blankSeedLine(propsPath, "level-seed"); err != nil {
		t.Fatalf("blankSeedLine error: %v", err)
	}

	got, err := os.ReadFile(propsPath)
	if err != nil {
		t.Fatal(err)
	}
	gotStr := string(got)
	if strings.Contains(gotStr, "level-seed=-12345") {
		t.Fatalf("expected level-seed line to be blanked, got:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "level-seed=\n") {
		t.Fatalf("expected a blank 'level-seed=' line, got:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "motd=hello") || !strings.Contains(gotStr, "difficulty=hard") {
		t.Fatalf("expected other lines untouched, got:\n%s", gotStr)
	}
}

func TestBlankSeedLineReturnsErrorWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	propsPath := filepath.Join(dir, "does-not-exist.properties")
	if err := blankSeedLine(propsPath, "level-seed"); err == nil {
		t.Fatal("expected an error when propsPath doesn't exist")
	}
}

func TestRenameWorldIfExistsRenamesToTimestampedPath(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world")
	if err := os.MkdirAll(worldPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldPath, "level.dat"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	if err := renameWorldIfExists(worldPath, now); err != nil {
		t.Fatalf("renameWorldIfExists error: %v", err)
	}

	expectedBackup := worldPath + "_prev_20260711_120000"
	if _, err := os.Stat(expectedBackup); err != nil {
		t.Fatalf("expected backup dir %s to exist: %v", expectedBackup, err)
	}
	if _, err := os.Stat(worldPath); !os.IsNotExist(err) {
		t.Fatalf("expected original worldPath to no longer exist, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(expectedBackup, "level.dat")); err != nil {
		t.Fatalf("expected level.dat to have moved with the rename: %v", err)
	}
}

func TestRenameWorldIfExistsIsNoOpWhenWorldMissing(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world")
	if err := renameWorldIfExists(worldPath, time.Now()); err != nil {
		t.Fatalf("expected nil error when world doesn't exist, got %v", err)
	}
}

func TestResetExtraFileBacksUpAndResetsToEmptyJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "limitedlives_data.json")
	if err := os.WriteFile(path, []byte(`{"player1":3}`), 0644); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	if err := resetExtraFile(path, now); err != nil {
		t.Fatalf("resetExtraFile error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{}" {
		t.Fatalf("expected original file reset to '{}', got %q", got)
	}

	expectedBackup := filepath.Join(dir, "limitedlives_data_prev_20260711_120000.json")
	backupContent, err := os.ReadFile(expectedBackup)
	if err != nil {
		t.Fatalf("expected backup file %s to exist: %v", expectedBackup, err)
	}
	if string(backupContent) != `{"player1":3}` {
		t.Fatalf("expected backup to contain the original content, got %q", backupContent)
	}
}

func TestResetExtraFileIsNoOpWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")
	if err := resetExtraFile(path, time.Now()); err != nil {
		t.Fatalf("expected nil error when file doesn't exist, got %v", err)
	}
}

func TestCopySeedTemplateCopiesIntoWorldDataDir(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "template.dat")
	if err := os.WriteFile(srcPath, []byte("nbt-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	worldPath := filepath.Join(dir, "world")

	tf := config.SeedTemplateFile{Src: srcPath, Dest: "data/betterzombieai_mapvars.dat"}
	if err := copySeedTemplate(worldPath, tf); err != nil {
		t.Fatalf("copySeedTemplate error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(worldPath, "data", "betterzombieai_mapvars.dat"))
	if err != nil {
		t.Fatalf("expected copied file to exist: %v", err)
	}
	if string(got) != "nbt-bytes" {
		t.Fatalf("expected copied content 'nbt-bytes', got %q", got)
	}
}

func TestCopySeedTemplateIsNoOpWhenSrcMissing(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world")
	tf := config.SeedTemplateFile{Src: filepath.Join(dir, "does-not-exist.dat"), Dest: "data/foo.dat"}
	if err := copySeedTemplate(worldPath, tf); err != nil {
		t.Fatalf("expected nil error when Src doesn't exist, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(worldPath, "data", "foo.dat")); !os.IsNotExist(err) {
		t.Fatal("expected no file to have been created when Src doesn't exist")
	}
}

func TestRegenReturnsErrorWhenBackendRunning(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	cfg := config.InstanceConfig{Name: "test", BackendPort: port}
	if err := Regen(cfg, false); err == nil {
		t.Fatal("expected an error when the backend is still running")
	}
}

func TestRegenAtFullFlowRenamesWorldResetsExtraFilesAndSeedsTemplates(t *testing.T) {
	dir := t.TempDir()
	worldPath := filepath.Join(dir, "world")
	if err := os.MkdirAll(worldPath, 0755); err != nil {
		t.Fatal(err)
	}
	livesPath := filepath.Join(dir, "limitedlives_data.json")
	if err := os.WriteFile(livesPath, []byte(`{"player1":3}`), 0644); err != nil {
		t.Fatal(err)
	}
	templateSrc := filepath.Join(dir, "template.dat")
	if err := os.WriteFile(templateSrc, []byte("nbt-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	propsPath := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(propsPath, []byte("level-seed=-999\nmotd=hi\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.InstanceConfig{
		Name:        "test",
		BackendPort: freeTCPPort(t), // nothing listening -> backend considered stopped
		WorldRegen: &config.WorldRegenConfig{
			WorldPath:            worldPath,
			ServerPropertiesPath: propsPath,
			SeedKey:              "level-seed",
			ExtraResetFiles:      []string{livesPath},
			SeedTemplateFiles:    []config.SeedTemplateFile{{Src: templateSrc, Dest: "data/betterzombieai_mapvars.dat"}},
		},
	}

	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	if err := regenAt(cfg, true, now); err != nil {
		t.Fatalf("regenAt error: %v", err)
	}

	if _, err := os.Stat(worldPath + "_prev_20260711_120000"); err != nil {
		t.Fatalf("expected old world to be renamed away: %v", err)
	}
	seededFile := filepath.Join(worldPath, "data", "betterzombieai_mapvars.dat")
	got, err := os.ReadFile(seededFile)
	if err != nil {
		t.Fatalf("expected template seeded into fresh world dir: %v", err)
	}
	if string(got) != "nbt-bytes" {
		t.Fatalf("unexpected seeded content: %q", got)
	}
	livesContent, err := os.ReadFile(livesPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(livesContent) != "{}" {
		t.Fatalf("expected limitedlives reset to '{}', got %q", livesContent)
	}
	props, err := os.ReadFile(propsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(props), "level-seed=-999") {
		t.Fatalf("expected seed to be blanked, got: %s", props)
	}
}

func TestRegenAtSkipsSeedBlankingWhenNewSeedFalse(t *testing.T) {
	dir := t.TempDir()
	propsPath := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(propsPath, []byte("level-seed=-999\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.InstanceConfig{
		Name:        "test",
		BackendPort: freeTCPPort(t),
		WorldRegen: &config.WorldRegenConfig{
			WorldPath:            filepath.Join(dir, "world"),
			ServerPropertiesPath: propsPath,
			SeedKey:              "level-seed",
		},
	}

	if err := regenAt(cfg, false, time.Now()); err != nil {
		t.Fatalf("regenAt error: %v", err)
	}

	props, err := os.ReadFile(propsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(props), "level-seed=-999") {
		t.Fatalf("expected seed to be left untouched when newSeed=false, got: %s", props)
	}
}

func TestRegenAtAbortsAfterExtraResetFileFailsAndNeverRunsSeedTemplates(t *testing.T) {
	dir := t.TempDir()

	// Create a file to reset
	resetPath := filepath.Join(dir, "test-reset.json")
	if err := os.WriteFile(resetPath, []byte(`{"data":"value"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Compute the exact backup path that resetExtraFile will try to use
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	ext := filepath.Ext(resetPath)
	base := strings.TrimSuffix(resetPath, ext)
	backupPath := base + "_prev_" + now.Format("20060102_150405") + ext

	// Create a directory at the backup path location to force os.WriteFile to fail
	// (can't write a file to a path that's already a directory)
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a seed template source file that would normally be copied
	templateSrc := filepath.Join(dir, "template.dat")
	if err := os.WriteFile(templateSrc, []byte("template-content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set up world path (should remain empty if pipeline aborts before seeding)
	worldPath := filepath.Join(dir, "world")

	cfg := config.InstanceConfig{
		Name:        "test",
		BackendPort: freeTCPPort(t), // nothing listening
		WorldRegen: &config.WorldRegenConfig{
			WorldPath:       worldPath,
			ExtraResetFiles: []string{resetPath},
			SeedTemplateFiles: []config.SeedTemplateFile{{
				Src:  templateSrc,
				Dest: "data/template.dat",
			}},
		},
	}

	// regenAt should fail on the extraResetFiles step
	if err := regenAt(cfg, false, now); err == nil {
		t.Fatal("expected regenAt to return an error when extra reset file fails")
	}

	// Verify that the seed template file was NEVER copied.
	// This is the direct proof that the pipeline aborted before reaching
	// the seed template copy loop, not continuing after the failure.
	expectedCopiedFile := filepath.Join(worldPath, "data", "template.dat")
	if _, err := os.Stat(expectedCopiedFile); !os.IsNotExist(err) {
		t.Fatalf("expected seed template file to not exist (proving pipeline aborted), but it does or had an unexpected stat error: %v", err)
	}
}
