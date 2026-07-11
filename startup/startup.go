package startup

import (
	"os"
	"strings"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
)

const (
	defaultBootPattern = "Done ("

	bootLogPollTimeout  = 180 * time.Second
	bootLogPollInterval = 3 * time.Second
)

// resolveStartupConfig applies defaults to a possibly-nil or partially-set
// StartupConfig, same pattern as backup.resolveBackupConfig. LogPath and
// Commands have no built-in default (empty means "skip the boot-log wait" /
// "nothing to apply", respectively) — only BootPattern defaults when unset.
func resolveStartupConfig(cfg *config.StartupConfig) (logPath, bootPattern string, commands []string) {
	bootPattern = defaultBootPattern
	if cfg == nil {
		return "", bootPattern, nil
	}
	logPath = cfg.LogPath
	if cfg.BootPattern != "" {
		bootPattern = cfg.BootPattern
	}
	commands = cfg.Commands
	return
}

// waitForBootLog polls logPath until its contents contain pattern or timeout
// elapses, checking every pollInterval. A missing file is treated as "not
// found yet", not an error — it may not exist until the backend process
// creates it. Returns false on timeout.
func waitForBootLog(logPath, pattern string, timeout, pollInterval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(logPath)
		if err == nil && strings.Contains(string(data), pattern) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(pollInterval)
	}
}
