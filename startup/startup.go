package startup

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
	"github.com/ShizukaJiku/gameops/internal/rcon"
)

const (
	defaultBootPattern = "Done ("

	bootLogPollTimeout  = 180 * time.Second
	bootLogPollInterval = 3 * time.Second

	rconReadyMaxAttempts  = 60
	rconReadyPollInterval = 3 * time.Second
	commandMaxAttempts    = 3
	commandRetryInterval  = 3 * time.Second
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

// waitForRconReady polls addr up to maxAttempts times, waiting pollInterval
// between attempts, dialing RCON and sending a real "seed" probe command
// each time (dial+auth alone is not sufficient — the original script this
// ports sends a real command and only treats a successful response as
// "ready", so this does the same). Returns false once maxAttempts is
// exhausted without a successful probe.
func waitForRconReady(port int, password string, maxAttempts int, pollInterval time.Duration) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for i := 0; i < maxAttempts; i++ {
		if _, err := sendRconCommand(addr, password, "seed"); err == nil {
			return true
		}
		time.Sleep(pollInterval)
	}
	return false
}

// sendRconCommand dials RCON fresh, sends cmd, and closes the connection —
// matching the original script's Invoke-Rcon, which opens a new connection
// per command rather than reusing one across the whole run.
func sendRconCommand(addr, password, cmd string) (string, error) {
	c, err := rcon.Dial(addr, password, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	return c.Command(cmd)
}

// applyCommand sends cmd via RCON, retrying up to maxAttempts times with
// retryInterval between attempts. It never returns an error: success and
// failure are both logged, and a command that fails every attempt is
// skipped so the caller can move on to the next one — matching the original
// script's log-and-continue behavior for individual startup commands.
func applyCommand(instanceName string, port int, password string, cmd string, maxAttempts int, retryInterval time.Duration) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err := sendRconCommand(addr, password, cmd)
		if err == nil {
			log.Printf("[%s] OK [%s] -> %s", instanceName, cmd, resp)
			return
		}
		if attempt == maxAttempts-1 {
			log.Printf("[%s] FAIL [%s] -> %v", instanceName, cmd, err)
			return
		}
		time.Sleep(retryInterval)
	}
}

// Apply runs the full startup sequence for cfg: optionally wait for a
// boot-log pattern, wait for RCON to respond to a real probe command, then
// apply every configured startup command in order (each with its own
// retries — see applyCommand). A nil StartupConfig or one with zero
// commands is a no-op. Apply returns an error ONLY for the two timeouts
// (boot-log pattern never found, RCON never ready) — individual command
// failures are logged and skipped, never surfaced as an Apply error, per
// the original script's log-and-continue behavior.
func Apply(cfg config.InstanceConfig) error {
	return applyWithTiming(cfg, bootLogPollTimeout, bootLogPollInterval, rconReadyMaxAttempts, rconReadyPollInterval, commandMaxAttempts, commandRetryInterval)
}

// applyWithTiming is Apply with injectable timing, so tests can drive every
// timeout path without waiting on the real multi-minute durations.
func applyWithTiming(cfg config.InstanceConfig, bootTimeout, bootInterval time.Duration, rconMaxAttempts int, rconInterval time.Duration, cmdMaxAttempts int, cmdInterval time.Duration) error {
	logPath, bootPattern, commands := resolveStartupConfig(cfg.Startup)
	if len(commands) == 0 {
		log.Printf("[%s] no startup commands configured, nothing to apply", cfg.Name)
		return nil
	}
	if cfg.Minecraft == nil {
		return fmt.Errorf("startup: instance %s has startup commands configured but no minecraft_config (RCON source)", cfg.Name)
	}

	if logPath != "" {
		log.Printf("[%s] waiting for boot log pattern %q (max %s)...", cfg.Name, bootPattern, bootTimeout)
		if !waitForBootLog(logPath, bootPattern, bootTimeout, bootInterval) {
			return fmt.Errorf("startup: timed out waiting for boot log pattern %q", bootPattern)
		}
	}

	log.Printf("[%s] waiting for RCON to respond...", cfg.Name)
	if !waitForRconReady(cfg.Minecraft.RconPort, cfg.Minecraft.RconPassword, rconMaxAttempts, rconInterval) {
		return fmt.Errorf("startup: RCON never responded after %d attempts", rconMaxAttempts)
	}

	log.Printf("[%s] RCON ready, applying %d startup commands...", cfg.Name, len(commands))
	for _, cmd := range commands {
		applyCommand(cfg.Name, cfg.Minecraft.RconPort, cfg.Minecraft.RconPassword, cmd, cmdMaxAttempts, cmdInterval)
	}
	return nil
}
