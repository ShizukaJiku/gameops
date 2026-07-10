package idlewatch

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
)

// fakeAdapter is a test double: HandleSleeping always triggers wake,
// PlayerCount and Stop are controlled by the test via fields.
type fakeAdapter struct {
	mu             sync.Mutex
	playerCount    int
	playerCountErr error
	stopCalled     chan struct{}
	wakingCalls    int
}

func newFakeAdapter() *fakeAdapter {
	return &fakeAdapter{stopCalled: make(chan struct{}, 1)}
}

func (f *fakeAdapter) HandleSleeping(conn net.Conn, wakingSince time.Time) bool {
	conn.Close()
	if !wakingSince.IsZero() {
		f.mu.Lock()
		f.wakingCalls++
		f.mu.Unlock()
	}
	return true
}

func (f *fakeAdapter) wakingCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.wakingCalls
}

func (f *fakeAdapter) PlayerCount() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.playerCountErr != nil {
		return 0, f.playerCountErr
	}
	return f.playerCount, nil
}

func (f *fakeAdapter) setPlayerCount(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playerCount = n
}

func (f *fakeAdapter) setPlayerCountErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playerCountErr = err
}

func (f *fakeAdapter) Stop() error {
	f.stopCalled <- struct{}{}
	return nil
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestInstanceWakeProxyAndIdleStop(t *testing.T) {
	listenPort := freePort(t)
	backendPort := freePort(t)

	adapter := newFakeAdapter()
	var startCalled sync.WaitGroup
	startCalled.Add(1)

	var backendLn net.Listener
	cfg := config.InstanceConfig{
		Name:                       "test",
		ListenPort:                 listenPort,
		BackendPort:                backendPort,
		IdleTimeoutMinutes:         0, // treated as immediate for this test via short override below
		PollIntervalSeconds:        0,
		BackendReadyTimeoutMinutes: 1,
		StartCommand:               "true", // no-op on the runner; real start is exercised in manual smoke testing (Task 10)
	}
	// Override with test-friendly durations directly rather than minutes/seconds
	// granularity — Instance reads these via cfg fields converted at Run() time,
	// so this test uses a dedicated constructor path (see newInstanceForTest).
	in := newInstanceForTest(cfg, adapter, func() {
		var err error
		backendLn, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", backendPort))
		if err != nil {
			t.Errorf("backend listen failed: %v", err)
			return
		}
		startCalled.Done()
		go func() {
			for {
				c, err := backendLn.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()
	}, 100*time.Millisecond, 50*time.Millisecond)

	go in.Run()
	time.Sleep(50 * time.Millisecond) // let listener bind

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", listenPort))
	if err != nil {
		t.Fatalf("dial listen port failed: %v", err)
	}
	conn.Close()

	startCalled.Wait()
	defer backendLn.Close()

	adapter.setPlayerCount(0)
	select {
	case <-adapter.stopCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("expected Stop() to be called after idle timeout")
	}
}

// TestInstanceIdleWatcherDoesNotStopOnPlayerCountError proves the fail-safe
// branch in idleWatcher: when PlayerCount() errors, that poll must NOT count
// toward idle (spec hard requirement). We inject a persistent PlayerCount
// error, wait several poll intervals (long enough that Stop() would have
// fired if errors were mistakenly treated as "0 players" given the test's
// short idle timeout), and assert Stop() was never called. Then we clear the
// error and confirm idle-stop still works, proving the fail-safe suppresses
// only the error case rather than breaking idle-stop entirely.
func TestInstanceIdleWatcherDoesNotStopOnPlayerCountError(t *testing.T) {
	listenPort := freePort(t)
	backendPort := freePort(t)

	adapter := newFakeAdapter()
	var startCalled sync.WaitGroup
	startCalled.Add(1)

	var backendLn net.Listener
	cfg := config.InstanceConfig{
		Name:                       "test",
		ListenPort:                 listenPort,
		BackendPort:                backendPort,
		IdleTimeoutMinutes:         0,
		PollIntervalSeconds:        0,
		BackendReadyTimeoutMinutes: 1,
		StartCommand:               "true",
	}
	in := newInstanceForTest(cfg, adapter, func() {
		var err error
		backendLn, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", backendPort))
		if err != nil {
			t.Errorf("backend listen failed: %v", err)
			return
		}
		startCalled.Done()
		go func() {
			for {
				c, err := backendLn.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()
	}, 100*time.Millisecond, 50*time.Millisecond)

	go in.Run()
	time.Sleep(50 * time.Millisecond) // let listener bind

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", listenPort))
	if err != nil {
		t.Fatalf("dial listen port failed: %v", err)
	}
	conn.Close()

	startCalled.Wait()
	defer backendLn.Close()

	adapter.setPlayerCountErr(errors.New("rcon unreachable"))

	// Wait several poll intervals (idleTimeout is 50ms, pollInterval is
	// 100ms) — long enough that if the fail-safe were broken, Stop() would
	// have already fired.
	select {
	case <-adapter.stopCalled:
		t.Fatal("Stop() was called while PlayerCount() was erroring; fail-safe did not suppress idle counting")
	case <-time.After(400 * time.Millisecond):
		// expected: no Stop() call while errors are occurring
	}

	adapter.setPlayerCountErr(nil)
	adapter.setPlayerCount(0)
	select {
	case <-adapter.stopCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("expected Stop() to be called after idle timeout once PlayerCount() errors stopped")
	}
}

// TestInstanceRoutesToAdapterWhileWakingAndDoesNotDoubleWake covers the gap
// found via manual testing: Forge routinely takes tens of seconds to bind
// its port after start_command runs. A connection landing in that window
// used to be routed straight to proxy() (state was already "awake"), which
// dialed a backend that wasn't listening yet and just failed silently. It
// must instead reach the adapter with waking=true, and must NOT re-trigger
// start_command.
func TestInstanceRoutesToAdapterWhileWakingAndDoesNotDoubleWake(t *testing.T) {
	listenPort := freePort(t)
	backendPort := freePort(t)

	adapter := newFakeAdapter()
	var startCount int32

	cfg := config.InstanceConfig{
		Name:                       "test",
		ListenPort:                 listenPort,
		BackendPort:                backendPort,
		BackendReadyTimeoutMinutes: 1,
		StartCommand:               "true",
	}
	startFn := func() {
		atomic.AddInt32(&startCount, 1)
		go func() {
			time.Sleep(150 * time.Millisecond) // simulate Forge's slow boot
			ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", backendPort))
			if err != nil {
				t.Errorf("backend listen failed: %v", err)
				return
			}
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()
	}

	in := newInstanceForTest(cfg, adapter, startFn, 100*time.Millisecond, time.Hour)

	go in.Run()
	time.Sleep(30 * time.Millisecond) // let listener bind

	// First connection: asleep -> triggers wake().
	conn1, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", listenPort))
	if err != nil {
		t.Fatalf("dial listen port failed: %v", err)
	}
	conn1.Close()

	// Backend won't be listening for another ~120ms — a connection now must
	// land in the "waking" state, not get proxied into a dial failure.
	time.Sleep(50 * time.Millisecond)

	conn2, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", listenPort))
	if err != nil {
		t.Fatalf("dial listen port failed: %v", err)
	}
	conn2.Close()

	time.Sleep(30 * time.Millisecond) // let handleConn(conn2) run

	if got := adapter.wakingCallCount(); got < 1 {
		t.Fatalf("expected adapter.HandleSleeping to be called with waking=true at least once, got %d", got)
	}
	if got := atomic.LoadInt32(&startCount); got != 1 {
		t.Fatalf("expected start_command to run exactly once, got %d", got)
	}
}

// TestInstanceResyncsToAsleepWhenBackendDiesWhileAwake covers a real
// production incident: stopping the backend directly (RCON stop, bypassing
// idleWatcher's own Stop() call) leaves Instance.state stuck at stateAwake.
// The next connection used to be proxied straight into a dead backend and
// fail silently. It must instead resync state to stateAsleep and hand that
// same connection to the adapter, which — for a real login attempt —
// triggers a fresh wake().
func TestInstanceResyncsToAsleepWhenBackendDiesWhileAwake(t *testing.T) {
	listenPort := freePort(t)
	backendPort := freePort(t)

	adapter := newFakeAdapter()
	var startCount int32
	var mu sync.Mutex
	var backendLn net.Listener

	cfg := config.InstanceConfig{
		Name:                       "test",
		ListenPort:                 listenPort,
		BackendPort:                backendPort,
		BackendReadyTimeoutMinutes: 1,
		StartCommand:               "true",
	}
	startFn := func() {
		atomic.AddInt32(&startCount, 1)
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", backendPort))
		if err != nil {
			t.Errorf("backend listen failed: %v", err)
			return
		}
		mu.Lock()
		backendLn = ln
		mu.Unlock()
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()
	}

	in := newInstanceForTest(cfg, adapter, startFn, 100*time.Millisecond, time.Hour)

	go in.Run()
	time.Sleep(30 * time.Millisecond) // let listener bind

	// First connection: asleep -> wake() -> awake (backend listening).
	conn1, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", listenPort))
	if err != nil {
		t.Fatalf("dial listen port failed: %v", err)
	}
	conn1.Close()

	deadline := time.Now().Add(2 * time.Second)
	for {
		in.mu.Lock()
		s := in.state
		in.mu.Unlock()
		if s == stateAwake {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("instance never reached awake state")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Simulate an external stop: close the backend listener without going
	// through Instance.Stop()/idleWatcher — Instance.state stays stateAwake.
	mu.Lock()
	backendLn.Close()
	mu.Unlock()
	time.Sleep(30 * time.Millisecond) // let the OS release the port

	// Second connection: proxy() dials a dead backend, must resync to
	// asleep and hand this same connection to the adapter as sleeping,
	// which (per fakeAdapter) triggers wake() again.
	conn2, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", listenPort))
	if err != nil {
		t.Fatalf("dial listen port failed: %v", err)
	}
	conn2.Close()

	time.Sleep(50 * time.Millisecond) // let handleConn(conn2) run

	if got := atomic.LoadInt32(&startCount); got != 2 {
		t.Fatalf("expected start_command to run twice (initial wake + resync rewake), got %d", got)
	}
}
