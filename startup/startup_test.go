package startup

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// --- minimal fake RCON server, local to this package's tests (same
// approach as maintenance_test.go/backup_test.go — internal/rcon's own fake
// server is unexported to those packages) ---

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
// the connection without responding (simulating an RCON failure).
func fakeRconServer(t *testing.T, responses map[string]string) string {
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

func TestWaitForRconReadyReturnsTrueWhenSeedSucceeds(t *testing.T) {
	addr := fakeRconServer(t, map[string]string{"seed": "Seed: [123456789]"})
	port := rconPortFromAddr(t, addr)
	if !waitForRconReady(port, "secret", 5, 20*time.Millisecond) {
		t.Fatal("expected waitForRconReady to return true when the server accepts the seed probe")
	}
}

func TestWaitForRconReadyReturnsFalseWhenNothingListening(t *testing.T) {
	port := freeTCPPort(t)
	if waitForRconReady(port, "secret", 3, 20*time.Millisecond) {
		t.Fatal("expected waitForRconReady to return false when nothing is listening")
	}
}

func TestWaitForRconReadySucceedsAfterInitialFailures(t *testing.T) {
	port := freeTCPPort(t) // nothing listening yet — first attempts must fail

	go func() {
		time.Sleep(60 * time.Millisecond)
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return // port may already be in TIME_WAIT in a slow CI; the retry loop will simply keep failing and the test will time out with a clear message
		}
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		authID, _, err := readTestPacket(conn)
		if err != nil {
			return
		}
		writeTestPacket(conn, authID, 2, "")
		id, _, err := readTestPacket(conn)
		if err != nil {
			return
		}
		writeTestPacket(conn, id, 0, "Seed: [1]")
	}()

	if !waitForRconReady(port, "secret", 20, 20*time.Millisecond) {
		t.Fatal("expected waitForRconReady to eventually succeed once the server starts listening")
	}
}

func TestApplyCommandLogsOkOnFirstTrySuccess(t *testing.T) {
	addr := fakeRconServer(t, map[string]string{"difficulty hard": "Set the difficulty to Hard"})
	port := rconPortFromAddr(t, addr)

	var buf strings.Builder
	origOutput := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(origOutput)

	applyCommand("test", port, "secret", "difficulty hard", 3, 10*time.Millisecond)

	if !strings.Contains(buf.String(), "OK") || !strings.Contains(buf.String(), "difficulty hard") {
		t.Fatalf("expected an OK log line mentioning the command, got: %s", buf.String())
	}
}

func TestApplyCommandLogsFailAfterAllRetriesFail(t *testing.T) {
	port := freeTCPPort(t) // nothing listening — every attempt fails

	var buf strings.Builder
	origOutput := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(origOutput)

	applyCommand("test", port, "secret", "difficulty hard", 3, 5*time.Millisecond)

	if !strings.Contains(buf.String(), "FAIL") || !strings.Contains(buf.String(), "difficulty hard") {
		t.Fatalf("expected a FAIL log line mentioning the command, got: %s", buf.String())
	}
}
