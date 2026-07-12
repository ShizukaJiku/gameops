package maintenance

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
)

// --- test helpers ---

func freeTCPListener(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln, ln.Addr().(*net.TCPAddr).Port
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

// stubForceKill replaces the package-level forceKillFn for the duration of
// the calling test, restoring the real implementation afterward. Every test
// that can reach the force-kill fallback path MUST use this — the real
// forceKill shells out to taskkill and would force-kill real processes on
// whatever machine runs the tests.
func stubForceKill(t *testing.T, fn func(string) error) {
	t.Helper()
	original := forceKillFn
	forceKillFn = fn
	t.Cleanup(func() { forceKillFn = original })
}

func failIfForceKillCalled(t *testing.T) {
	t.Helper()
	stubForceKill(t, func(processName string) error {
		t.Fatalf("force-kill should not have been called (processName=%q)", processName)
		return nil
	})
}

// --- minimal fake RCON server, local to this package's tests (same
// approach as backup_test.go — internal/rcon's own fake server is
// unexported to that package) ---

func readTestPacket(r io.Reader) (id int32, body string, err error) {
	var length int32
	if err = binary.Read(r, binary.LittleEndian, &length); err != nil {
		return 0, "", err
	}
	buf := make([]byte, length)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, "", err
	}
	id = int32(binary.LittleEndian.Uint32(buf[0:4]))
	body = string(bytes.TrimRight(buf[8:], "\x00"))
	return id, body, nil
}

func writeTestPacket(w io.Writer, id int32, typ int32, body string) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, id)
	binary.Write(buf, binary.LittleEndian, typ)
	buf.WriteString(body)
	buf.WriteByte(0)
	buf.WriteByte(0)
	out := new(bytes.Buffer)
	binary.Write(out, binary.LittleEndian, int32(buf.Len()))
	out.Write(buf.Bytes())
	w.Write(out.Bytes())
}

// fakeRconServer accepts any password (single-packet Minecraft-style auth).
// For each command received, it looks up responses; a missing entry closes
// the connection without responding (simulating an RCON failure). onCommand,
// if non-nil, is called synchronously with each command body right after
// it's received — used to trigger a side effect (like closing a backend
// listener) exactly when the "stop" command arrives, simulating a real
// clean shutdown.
func fakeRconServer(t *testing.T, responses map[string]string, onCommand func(cmd string)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				authID, _, err := readTestPacket(conn)
				if err != nil {
					return
				}
				const typeAuthResponse = 2
				const typeResponse = 0
				writeTestPacket(conn, authID, typeAuthResponse, "")

				for {
					id, body, err := readTestPacket(conn)
					if err != nil {
						return
					}
					if onCommand != nil {
						onCommand(body)
					}
					resp, ok := responses[body]
					if !ok {
						return
					}
					writeTestPacket(conn, id, typeResponse, resp)
				}
			}(conn)
		}
	}()

	return ln.Addr().String()
}

func rconPortFromAddr(t *testing.T, addr string) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi port: %v", err)
	}
	return port
}

// --- resolveMaintenanceConfig ---

func TestResolveMaintenanceConfigAppliesAllDefaultsWhenNil(t *testing.T) {
	processName, stopCommand := resolveMaintenanceConfig(nil)
	if processName != defaultProcessName || stopCommand != defaultStopCommand {
		t.Fatalf("expected defaults, got processName=%q stopCommand=%q", processName, stopCommand)
	}
}

func TestResolveMaintenanceConfigAppliesPartialDefaults(t *testing.T) {
	cfg := &config.MaintenanceConfig{ProcessName: "PalServer"}
	processName, stopCommand := resolveMaintenanceConfig(cfg)
	if processName != "PalServer" {
		t.Fatalf("expected custom processName kept, got %q", processName)
	}
	if stopCommand != defaultStopCommand {
		t.Fatalf("expected default stopCommand, got %q", stopCommand)
	}
}

func TestResolveMaintenanceConfigKeepsAllFieldsWhenSet(t *testing.T) {
	cfg := &config.MaintenanceConfig{ProcessName: "PalServer", StopCommand: "Shutdown 5"}
	processName, stopCommand := resolveMaintenanceConfig(cfg)
	if processName != "PalServer" || stopCommand != "Shutdown 5" {
		t.Fatalf("expected both custom values kept, got processName=%q stopCommand=%q", processName, stopCommand)
	}
}

// --- backendReachable / waitUntilBackendDown ---

func TestBackendReachableTrueWhenListening(t *testing.T) {
	_, port := freeTCPListener(t)
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

func TestWaitUntilBackendDownReturnsTrueWhenPortGoesDown(t *testing.T) {
	ln, port := freeTCPListener(t)
	go func() {
		time.Sleep(30 * time.Millisecond)
		ln.Close()
	}()

	if !waitUntilBackendDown(port, 2*time.Second, 10*time.Millisecond) {
		t.Fatal("expected waitUntilBackendDown to return true once the port closed")
	}
}

func TestWaitUntilBackendDownReturnsFalseOnTimeout(t *testing.T) {
	_, port := freeTCPListener(t) // never closed during this test

	if waitUntilBackendDown(port, 50*time.Millisecond, 10*time.Millisecond) {
		t.Fatal("expected waitUntilBackendDown to return false when the port never goes down")
	}
}

// --- Stop / stopWithTimeout ---

func TestStopReturnsNilWhenAlreadyStopped(t *testing.T) {
	failIfForceKillCalled(t)
	port := freeTCPPort(t) // nothing listening

	cfg := config.InstanceConfig{Name: "test", BackendPort: port}
	if err := Stop(cfg); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

func TestStopCleanRconSucceedsWithoutForceKill(t *testing.T) {
	failIfForceKillCalled(t)

	backendLn, backendPort := freeTCPListener(t)
	var mu sync.Mutex
	closed := false
	addr := fakeRconServer(t, map[string]string{"stop": "Stopping the server"}, func(cmd string) {
		if cmd != "stop" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if !closed {
			backendLn.Close()
			closed = true
		}
	})

	cfg := config.InstanceConfig{
		Name:        "test",
		BackendPort: backendPort,
		Minecraft: &config.MinecraftAdapterConfig{
			RconPort:     rconPortFromAddr(t, addr),
			RconPassword: "secret",
		},
	}

	if err := stopWithTimeout(cfg, 2*time.Second, 10*time.Millisecond); err != nil {
		t.Fatalf("stopWithTimeout error: %v", err)
	}
}

func TestStopFallsBackToForceKillWhenRconFails(t *testing.T) {
	var mu sync.Mutex
	var calledWith string
	stubForceKill(t, func(processName string) error {
		mu.Lock()
		calledWith = processName
		mu.Unlock()
		return nil
	})

	_, backendPort := freeTCPListener(t) // stays up: RCON is what's broken here
	unreachableRconPort := freeTCPPort(t)

	cfg := config.InstanceConfig{
		Name:        "test",
		BackendPort: backendPort,
		Minecraft: &config.MinecraftAdapterConfig{
			RconPort:     unreachableRconPort,
			RconPassword: "secret",
		},
		Maintenance: &config.MaintenanceConfig{ProcessName: "PalServer"},
	}

	if err := Stop(cfg); err != nil {
		t.Fatalf("Stop error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if calledWith != "PalServer" {
		t.Fatalf("expected force-kill to be called with processName %q, got %q", "PalServer", calledWith)
	}
}

func TestStopFallsBackToForceKillWhenBackendStaysUpAfterRconStop(t *testing.T) {
	forceKillCalled := false
	stubForceKill(t, func(processName string) error {
		forceKillCalled = true
		return nil
	})

	_, backendPort := freeTCPListener(t) // deliberately never closed
	addr := fakeRconServer(t, map[string]string{"stop": "Stopping the server"}, nil)

	cfg := config.InstanceConfig{
		Name:        "test",
		BackendPort: backendPort,
		Minecraft: &config.MinecraftAdapterConfig{
			RconPort:     rconPortFromAddr(t, addr),
			RconPassword: "secret",
		},
	}

	if err := stopWithTimeout(cfg, 50*time.Millisecond, 10*time.Millisecond); err != nil {
		t.Fatalf("stopWithTimeout error: %v", err)
	}
	if !forceKillCalled {
		t.Fatal("expected force-kill to be called after the backend stayed up past the poll timeout")
	}
}

func TestStopFallsBackToForceKillWhenNoMinecraftConfig(t *testing.T) {
	forceKillCalled := false
	stubForceKill(t, func(processName string) error {
		forceKillCalled = true
		return nil
	})

	_, backendPort := freeTCPListener(t)

	cfg := config.InstanceConfig{Name: "test", BackendPort: backendPort, Minecraft: nil}

	if err := Stop(cfg); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
	if !forceKillCalled {
		t.Fatal("expected force-kill to be called when there's no RCON config to attempt a clean stop with")
	}
}

func TestStopReturnsErrorWhenForceKillFails(t *testing.T) {
	stubForceKill(t, func(processName string) error {
		return fmt.Errorf("simulated taskkill failure")
	})

	_, backendPort := freeTCPListener(t)
	cfg := config.InstanceConfig{Name: "test", BackendPort: backendPort, Minecraft: nil}

	err := Stop(cfg)
	if err == nil {
		t.Fatal("expected Stop to return an error when force-kill fails")
	}
}

// --- Resume ---

func TestResumeRunsStartCommand(t *testing.T) {
	cfg := config.InstanceConfig{Name: "test", StartCommand: "cmd /c exit 0"}
	if err := Resume(cfg); err != nil {
		t.Fatalf("Resume error: %v", err)
	}
}

func TestResumeReturnsErrorForEmptyStartCommand(t *testing.T) {
	cfg := config.InstanceConfig{Name: "test", StartCommand: ""}
	if err := Resume(cfg); err == nil {
		t.Fatal("expected error for empty start_command")
	}
}

// TestResumeWaitsForStartCommandToComplete guards against a real production
// bug (2026-07-11): Resume used to dispatch StartCommand with exec.Cmd.Start
// (non-blocking) and return immediately. As a one-shot CLI invocation (not a
// long-lived daemon like idlewatch), the whole process — and, over SSH, its
// Job Object — could tear down before the dispatched command finished,
// silently dropping the resume. If Resume regresses back to Start, this
// command (which takes ~1s) would return in well under 100ms, failing the
// timing assertion below.
func TestResumeWaitsForStartCommandToComplete(t *testing.T) {
	cfg := config.InstanceConfig{Name: "test", StartCommand: "cmd /c ping -n 2 127.0.0.1 >nul"}
	start := time.Now()
	if err := Resume(cfg); err != nil {
		t.Fatalf("Resume error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("expected Resume to wait for the start command to finish (~1s), returned after %v", elapsed)
	}
}
