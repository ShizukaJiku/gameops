package startup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
)

func TestResolveStartupConfigAppliesDefaultsWhenNil(t *testing.T) {
	logPath, bootPattern, commands := resolveStartupConfig(nil)
	if logPath != "" {
		t.Fatalf("expected empty logPath, got %q", logPath)
	}
	if bootPattern != defaultBootPattern {
		t.Fatalf("expected default boot pattern %q, got %q", defaultBootPattern, bootPattern)
	}
	if commands != nil {
		t.Fatalf("expected nil commands, got %+v", commands)
	}
}

func TestResolveStartupConfigKeepsSetFieldsAndDefaultsBootPatternOnly(t *testing.T) {
	cfg := &config.StartupConfig{LogPath: `C:\mc-forge\logs\latest.log`, Commands: []string{"difficulty hard"}}
	logPath, bootPattern, commands := resolveStartupConfig(cfg)
	if logPath != `C:\mc-forge\logs\latest.log` {
		t.Fatalf("expected custom logPath kept, got %q", logPath)
	}
	if bootPattern != defaultBootPattern {
		t.Fatalf("expected default boot pattern when unset, got %q", bootPattern)
	}
	if len(commands) != 1 || commands[0] != "difficulty hard" {
		t.Fatalf("expected custom commands kept, got %+v", commands)
	}
}

func TestResolveStartupConfigKeepsCustomBootPattern(t *testing.T) {
	cfg := &config.StartupConfig{BootPattern: "custom-pattern"}
	_, bootPattern, _ := resolveStartupConfig(cfg)
	if bootPattern != "custom-pattern" {
		t.Fatalf("expected custom boot pattern kept, got %q", bootPattern)
	}
}

func TestWaitForBootLogReturnsTrueWhenPatternAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "latest.log")
	if err := os.WriteFile(logPath, []byte("[Server thread/INFO]: Done (4.2s)! For help, type \"help\""), 0644); err != nil {
		t.Fatal(err)
	}
	if !waitForBootLog(logPath, "Done (", 2*time.Second, 50*time.Millisecond) {
		t.Fatal("expected waitForBootLog to return true when pattern is already in the file")
	}
}

func TestWaitForBootLogReturnsTrueWhenPatternAppearsLater(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "latest.log")
	if err := os.WriteFile(logPath, []byte("[Server thread/INFO]: Starting minecraft server version 1.20.1"), 0644); err != nil {
		t.Fatal(err)
	}

	done := make(chan bool, 1)
	go func() {
		done <- waitForBootLog(logPath, "Done (", 2*time.Second, 50*time.Millisecond)
	}()

	time.Sleep(150 * time.Millisecond)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n[Server thread/INFO]: Done (4.2s)! For help, type \"help\""); err != nil {
		t.Fatal(err)
	}
	f.Close()

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("expected waitForBootLog to return true once the pattern appears")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("waitForBootLog did not return within 3s of a 2s timeout")
	}
}

func TestWaitForBootLogReturnsFalseOnTimeoutWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "never-created.log")
	if waitForBootLog(logPath, "Done (", 200*time.Millisecond, 50*time.Millisecond) {
		t.Fatal("expected waitForBootLog to return false when the file never appears")
	}
}

func TestWaitForBootLogReturnsFalseOnTimeoutWhenPatternNeverAppears(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "latest.log")
	if err := os.WriteFile(logPath, []byte("[Server thread/INFO]: still booting"), 0644); err != nil {
		t.Fatal(err)
	}
	if waitForBootLog(logPath, "Done (", 200*time.Millisecond, 50*time.Millisecond) {
		t.Fatal("expected waitForBootLog to return false when the pattern never appears")
	}
}
