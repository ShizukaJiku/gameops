package maintenance

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
	"github.com/ShizukaJiku/gameops/internal/rcon"
)

const (
	defaultProcessName = "java"
	defaultStopCommand = "stop"
	stopPollTimeout     = 60 * time.Second
	stopPollInterval    = 2 * time.Second
)

// resolveMaintenanceConfig applies defaults to a possibly-nil or
// partially-set MaintenanceConfig, same pattern as
// backup.resolveBackupConfig.
func resolveMaintenanceConfig(cfg *config.MaintenanceConfig) (processName, stopCommand string) {
	processName = defaultProcessName
	stopCommand = defaultStopCommand
	if cfg == nil {
		return
	}
	if cfg.ProcessName != "" {
		processName = cfg.ProcessName
	}
	if cfg.StopCommand != "" {
		stopCommand = cfg.StopCommand
	}
	return
}

// backendReachable reports whether a TCP connection to 127.0.0.1:backendPort
// succeeds right now. It doesn't speak any game protocol — it only tells
// "something is listening" apart from "nothing is listening", which is
// enough to know whether an instance is already stopped or has finished
// stopping.
func backendReachable(backendPort int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", backendPort), 2*time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// waitUntilBackendDown polls backendPort until it stops accepting
// connections or timeout elapses, checking every pollInterval. Returns
// false on timeout.
func waitUntilBackendDown(backendPort int, timeout, pollInterval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !backendReachable(backendPort) {
			return true
		}
		time.Sleep(pollInterval)
	}
	return false
}

// trySendStopCommand dials RCON once and sends stopCommand, reporting
// whether the command itself was accepted (not whether the backend has
// actually stopped yet — the caller polls for that separately).
func trySendStopCommand(mc *config.MinecraftAdapterConfig, stopCommand string) error {
	c, err := rcon.Dial(fmt.Sprintf("127.0.0.1:%d", mc.RconPort), mc.RconPassword, 5*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()

	_, err = c.Command(stopCommand)
	return err
}

// forceKillFn is a package-level seam so tests can replace the real
// taskkill invocation with a fake — the real one force-kills every process
// with a matching name on whatever machine runs it, which must never happen
// as a side effect of running the test suite.
var forceKillFn = forceKill

// forceKill shells out to taskkill to force-terminate every process named
// processName. Matches the original script's Stop-Process -Force fallback.
func forceKill(processName string) error {
	return exec.Command("taskkill", "/IM", processName, "/F").Run()
}

// Stop stops cfg's backend: a clean RCON stop_command first (if cfg.Minecraft
// is set and the command is accepted), waited out until the backend port
// goes down; falling back to force-killing process_name if there's no RCON
// config, the command fails, or the backend doesn't go down within the poll
// timeout. A stop that required the hard fallback is not an error — that's
// the documented, expected behavior of the original script this ports (see
// design spec §3.3).
func Stop(cfg config.InstanceConfig) error {
	return stopWithTimeout(cfg, stopPollTimeout, stopPollInterval)
}

// stopWithTimeout is Stop with injectable poll timing, so tests can drive
// the force-kill-on-timeout path without waiting 60 real seconds.
func stopWithTimeout(cfg config.InstanceConfig, pollTimeout, pollInterval time.Duration) error {
	processName, stopCommand := resolveMaintenanceConfig(cfg.Maintenance)

	if !backendReachable(cfg.BackendPort) {
		log.Printf("[%s] already stopped", cfg.Name)
		return nil
	}

	cleanStopWorked := false
	if cfg.Minecraft != nil {
		if err := trySendStopCommand(cfg.Minecraft, stopCommand); err != nil {
			log.Printf("[%s] RCON stop failed (%v), falling back to force-kill", cfg.Name, err)
		} else if waitUntilBackendDown(cfg.BackendPort, pollTimeout, pollInterval) {
			cleanStopWorked = true
		} else {
			log.Printf("[%s] backend still up after %s, falling back to force-kill", cfg.Name, pollTimeout)
		}
	}

	if cleanStopWorked {
		log.Printf("[%s] stopped cleanly via RCON", cfg.Name)
		return nil
	}

	if err := forceKillFn(processName); err != nil {
		return fmt.Errorf("maintenance: force-kill %s: %w", processName, err)
	}
	log.Printf("[%s] force-killed process %s", cfg.Name, processName)
	return nil
}

// Resume restarts cfg's backend by running its configured StartCommand and
// returning immediately, without waiting for it to finish booting — the
// same "fire and forget" semantics idlewatch uses to wake a sleeping
// instance, implemented independently here (see design spec §3.3 for why
// this isn't imported from idlewatch).
func Resume(cfg config.InstanceConfig) error {
	parts := strings.Fields(cfg.StartCommand)
	if len(parts) == 0 {
		return fmt.Errorf("maintenance: empty start_command")
	}
	if err := exec.Command(parts[0], parts[1:]...).Start(); err != nil {
		return fmt.Errorf("maintenance: start command failed: %w", err)
	}
	log.Printf("[%s] resume: start command issued", cfg.Name)
	return nil
}
