package idlewatch

import (
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
)

type Adapter interface {
	HandleSleeping(conn net.Conn, wakingSince time.Time) (attemptingLogin bool)
	PlayerCount() (int, error)
	Stop() error
}

// wakeState tracks where an Instance is between a real backend being asleep
// and fully proxying. stateWaking exists so connections that land in the gap
// between "start_command has run" and "backend port accepts connections" —
// which for Forge is routinely tens of seconds — get a real adapter response
// (a "starting up" status/kick) instead of being proxied into a dial failure.
type wakeState int

const (
	stateAsleep wakeState = iota
	stateWaking
	stateAwake
)

type Instance struct {
	cfg     config.InstanceConfig
	adapter Adapter

	startFn      func() // invokes cfg.StartCommand; overridable for tests
	pollInterval time.Duration
	idleTimeout  time.Duration
	readyTimeout time.Duration
	stopTimeout  time.Duration // how long to wait for backend port to go down after Stop()

	mu          sync.Mutex
	state       wakeState
	wakingSince time.Time // set by wake() on entering stateWaking; zero value otherwise
}

func NewInstance(cfg config.InstanceConfig, adapter Adapter) *Instance {
	in := &Instance{
		cfg:          cfg,
		adapter:      adapter,
		pollInterval: time.Duration(cfg.PollIntervalSeconds) * time.Second,
		idleTimeout:  time.Duration(cfg.IdleTimeoutMinutes) * time.Minute,
		readyTimeout: time.Duration(cfg.BackendReadyTimeoutMinutes) * time.Minute,
		stopTimeout:  60 * time.Second,
	}
	in.startFn = func() {
		if err := runStartCommand(cfg.StartCommand); err != nil {
			log.Printf("[%s] start command failed: %v", cfg.Name, err)
		}
	}
	return in
}

// newInstanceForTest lets tests replace the real "run a shell command and
// wait for a real Windows process to boot" behavior with an immediate,
// in-process fake, and use short poll/idle durations instead of the
// minutes-granularity config fields.
func newInstanceForTest(cfg config.InstanceConfig, adapter Adapter, startFn func(), pollInterval, idleTimeout time.Duration) *Instance {
	in := NewInstance(cfg, adapter)
	in.startFn = startFn
	in.pollInterval = pollInterval
	in.idleTimeout = idleTimeout
	in.readyTimeout = 2 * time.Second
	in.stopTimeout = 300 * time.Millisecond
	return in
}

func (in *Instance) Run() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", in.cfg.ListenPort))
	if err != nil {
		return err
	}
	log.Printf("[%s] listening on :%d", in.cfg.Name, in.cfg.ListenPort)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[%s] accept error: %v", in.cfg.Name, err)
			continue
		}
		go in.handleConn(conn)
	}
}

// handleConn routes one connection based on Instance.state at the moment it
// is read. There's a narrow window (microseconds) around a state transition
// where a connection can be routed based on a value that changes immediately
// after — e.g. proxied to a backend that's just been stopped, or sent to
// HandleSleeping just as wake() flips state to waking. This is accepted, not
// re-checked: it self-corrects on the client's next connection attempt, and
// closing it fully would require re-locking mid-function or restructuring
// the accept path for a boundary condition that already degrades gracefully.
//
// While waking (start_command has run, backend port not confirmed up yet),
// connections are NOT proxied — dialing a backend that isn't listening yet
// would just fail the connection with no message. They go to the adapter
// instead, same as asleep, but its return value is ignored: a second login
// attempt during this window must not re-run start_command.
func (in *Instance) handleConn(conn net.Conn) {
	in.mu.Lock()
	state := in.state
	wakingSince := in.wakingSince
	in.mu.Unlock()

	switch state {
	case stateAwake:
		if !in.proxy(conn) {
			// Backend unreachable even though we thought we were awake —
			// something stopped it outside idleWatcher's own Stop() call
			// (e.g. a direct RCON stop). Resync to asleep and hand this
			// same connection to the adapter as if it had arrived while
			// genuinely asleep: proxy() only returns false before any
			// bytes are exchanged with the client, so conn is untouched.
			in.mu.Lock()
			in.state = stateAsleep
			in.wakingSince = time.Time{}
			in.mu.Unlock()
			log.Printf("[%s] backend unreachable while awake — resyncing to asleep", in.cfg.Name)
			if in.adapter.HandleSleeping(conn, time.Time{}) {
				in.wake()
			}
		}
	case stateWaking:
		in.adapter.HandleSleeping(conn, wakingSince)
	default: // stateAsleep
		if in.adapter.HandleSleeping(conn, time.Time{}) {
			in.wake()
		}
	}
}

func (in *Instance) wake() {
	in.mu.Lock()
	if in.state != stateAsleep {
		in.mu.Unlock()
		return
	}
	in.state = stateWaking
	in.wakingSince = time.Now()
	in.mu.Unlock()

	log.Printf("[%s] waking", in.cfg.Name)
	in.startFn()

	if !in.waitForBackend() {
		log.Printf("[%s] backend never became ready, going back to sleep", in.cfg.Name)
		in.mu.Lock()
		in.state = stateAsleep
		in.wakingSince = time.Time{}
		in.mu.Unlock()
		return
	}

	in.mu.Lock()
	in.state = stateAwake
	in.wakingSince = time.Time{}
	in.mu.Unlock()

	go in.idleWatcher()
}

func (in *Instance) waitForBackend() bool {
	deadline := time.Now().Add(in.readyTimeout)
	addr := fmt.Sprintf("127.0.0.1:%d", in.cfg.BackendPort)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			c.Close()
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

// proxy dials the backend and, on success, proxies bytes bidirectionally
// until either side closes, always closing conn itself. It returns false
// only when the backend dial fails — in that case conn is left open and
// unread, so the caller can safely hand it to HandleSleeping instead.
func (in *Instance) proxy(conn net.Conn) bool {
	backend, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", in.cfg.BackendPort))
	if err != nil {
		log.Printf("[%s] backend dial failed: %v", in.cfg.Name, err)
		return false
	}
	defer conn.Close()
	defer backend.Close()

	done := make(chan struct{}, 2)
	go func() { io.Copy(backend, conn); done <- struct{}{} }()
	go func() { io.Copy(conn, backend); done <- struct{}{} }()
	<-done
	return true
}

func (in *Instance) idleWatcher() {
	var idleSince time.Time

	for {
		time.Sleep(in.pollInterval)

		in.mu.Lock()
		state := in.state
		in.mu.Unlock()
		if state != stateAwake {
			return
		}

		count, err := in.adapter.PlayerCount()
		if err != nil {
			log.Printf("[%s] player count check failed, not counting toward idle: %v", in.cfg.Name, err)
			idleSince = time.Time{}
			continue
		}

		if count > 0 {
			idleSince = time.Time{}
			continue
		}

		if idleSince.IsZero() {
			idleSince = time.Now()
			continue
		}

		if time.Since(idleSince) >= in.idleTimeout {
			log.Printf("[%s] idle for %s, stopping", in.cfg.Name, in.idleTimeout)
			if err := in.adapter.Stop(); err != nil {
				log.Printf("[%s] stop command failed: %v", in.cfg.Name, err)
			}
			if !in.waitForBackendDown(in.stopTimeout) {
				log.Printf("[%s] backend still reachable after stop timeout — needs manual check", in.cfg.Name)
			}
			in.mu.Lock()
			in.state = stateAsleep
			in.wakingSince = time.Time{}
			in.mu.Unlock()
			return
		}
	}
}

// waitForBackendDown polls the backend port until it stops accepting
// connections (backend process exited) or timeout elapses. Returns false on
// timeout — gameops cannot force-kill an arbitrary game process (that needs
// adapter/process knowledge it doesn't have), so a timeout just logs for the
// operator to check manually.
func (in *Instance) waitForBackendDown(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", in.cfg.BackendPort)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 1*time.Second)
		if err != nil {
			return true
		}
		c.Close()
		time.Sleep(2 * time.Second)
	}
	return false
}

func runStartCommand(cmd string) error {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return fmt.Errorf("empty start_command")
	}
	return exec.Command(parts[0], parts[1:]...).Start()
}
