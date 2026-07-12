package worldregen

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
