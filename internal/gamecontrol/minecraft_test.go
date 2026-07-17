package gamecontrol

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
)

// --- minimal fake RCON server, duplicated per-package per this project's
// existing convention (see maintenance_test.go, internal/rcon/rcon_test.go) ---

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
				const typeAuthResponse = 2
				const typeResponse = 0
				authID, _, err := readTestPacket(conn)
				if err != nil {
					return
				}
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

func TestMinecraftControllerStatusOfflineWhenBackendUnreachable(t *testing.T) {
	port := freeTCPPort(t)
	c := NewMinecraftController(config.InstanceConfig{Name: "test", BackendPort: port})

	status, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.Online {
		t.Fatal("expected Online=false when backend port is unreachable")
	}
}

func TestMinecraftControllerStatusOnlineParsesPlayerCounts(t *testing.T) {
	_, backendPort := freeTCPListener(t)
	addr := fakeRconServer(t, map[string]string{
		"list": "There are 3 of a max of 20 players online: A, B, C",
	}, nil)

	c := NewMinecraftController(config.InstanceConfig{
		Name:        "test",
		BackendPort: backendPort,
		Minecraft: &config.MinecraftAdapterConfig{
			RconPort:     rconPortFromAddr(t, addr),
			RconPassword: "secret",
		},
	})

	status, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !status.Online {
		t.Fatal("expected Online=true when backend port is reachable")
	}
	if status.PlayerCount != 3 || status.MaxPlayers != 20 {
		t.Fatalf("expected PlayerCount=3 MaxPlayers=20, got PlayerCount=%d MaxPlayers=%d", status.PlayerCount, status.MaxPlayers)
	}
}

func TestMinecraftControllerStatusTracksUptimeSinceObservedOnline(t *testing.T) {
	_, backendPort := freeTCPListener(t)
	addr := fakeRconServer(t, map[string]string{
		"list": "There are 0 of a max of 20 players online: ",
	}, nil)

	c := NewMinecraftController(config.InstanceConfig{
		Name:        "test",
		BackendPort: backendPort,
		Minecraft: &config.MinecraftAdapterConfig{
			RconPort:     rconPortFromAddr(t, addr),
			RconPassword: "secret",
		},
	})

	first, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("first Status error: %v", err)
	}
	if first.UptimeSec != 0 {
		t.Fatalf("expected UptimeSec=0 on first observed-online call, got %d", first.UptimeSec)
	}

	time.Sleep(1100 * time.Millisecond)

	second, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("second Status error: %v", err)
	}
	if second.UptimeSec < 1 {
		t.Fatalf("expected UptimeSec>=1 after 1.1s, got %d", second.UptimeSec)
	}
}

func TestMinecraftControllerStopUsesRconThenBackendDown(t *testing.T) {
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

	c := NewMinecraftController(config.InstanceConfig{
		Name:        "test",
		BackendPort: backendPort,
		Minecraft: &config.MinecraftAdapterConfig{
			RconPort:     rconPortFromAddr(t, addr),
			RconPassword: "secret",
		},
	})

	if err := c.Stop(context.Background()); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

func TestMinecraftControllerStartRunsStartCommand(t *testing.T) {
	c := NewMinecraftController(config.InstanceConfig{Name: "test", StartCommand: "cmd /c exit 0"})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start error: %v", err)
	}
}

// TestMinecraftControllerStatusCanceledContextReturnsOfflinePromptly proves
// backendReachable's dial honors ctx cancellation: even though the backend
// port is genuinely reachable, an already-canceled ctx makes the dial fail
// immediately (same as an unreachable port from the caller's point of view),
// so Status returns promptly with Online=false, err=nil rather than hanging
// or panicking.
func TestMinecraftControllerStatusCanceledContextReturnsOfflinePromptly(t *testing.T) {
	_, backendPort := freeTCPListener(t)

	c := NewMinecraftController(config.InstanceConfig{Name: "test", BackendPort: backendPort})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	status, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.Online {
		t.Fatal("expected Online=false when ctx is already canceled, even though backend port is reachable")
	}
}

// TestMinecraftControllerPlayerCountsCanceledContext proves playerCounts
// selects ctx.Done() over the goroutine's result when ctx is already
// canceled, against a real fake RCON server.
func TestMinecraftControllerPlayerCountsCanceledContext(t *testing.T) {
	addr := fakeRconServer(t, map[string]string{
		"list": "There are 3 of a max of 20 players online: A, B, C",
	}, nil)

	c := NewMinecraftController(config.InstanceConfig{
		Name: "test",
		Minecraft: &config.MinecraftAdapterConfig{
			RconPort:     rconPortFromAddr(t, addr),
			RconPassword: "secret",
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := c.playerCounts(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected errors.Is(err, context.Canceled), got %v", err)
	}
}

// TestMinecraftControllerStartCanceledContext proves the ctx-guard at the
// top of Start fires before maintenance.Resume is invoked: StartCommand is
// deliberately empty, which would make maintenance.Resume fail loudly with
// a distinguishable error if it were actually called. Getting
// context.Canceled instead proves the guard fired first.
func TestMinecraftControllerStartCanceledContext(t *testing.T) {
	c := NewMinecraftController(config.InstanceConfig{Name: "test", StartCommand: ""})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Start(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected errors.Is(err, context.Canceled), got %v", err)
	}
}

// TestMinecraftControllerStopCanceledContext proves the ctx-guard at the
// top of Stop fires before maintenance.Stop is invoked: BackendPort/Minecraft
// config are deliberately left unset, which would make maintenance.Stop fail
// loudly with a distinguishable error if it were actually called. Getting
// context.Canceled instead proves the guard fired first.
func TestMinecraftControllerStopCanceledContext(t *testing.T) {
	c := NewMinecraftController(config.InstanceConfig{Name: "test"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Stop(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected errors.Is(err, context.Canceled), got %v", err)
	}
}
