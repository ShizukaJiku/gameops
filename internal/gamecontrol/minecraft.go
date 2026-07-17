package gamecontrol

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
	"github.com/ShizukaJiku/gameops/internal/rcon"
	"github.com/ShizukaJiku/gameops/maintenance"
)

var listResponseRe = regexp.MustCompile(`There are (\d+) of a max(?: of)? (\d+)`)

// MinecraftController implements GameController for a Minecraft instance.
// Start/Stop delegate to maintenance.Resume/maintenance.Stop rather than
// re-implementing RCON-stop/force-kill logic that already exists there.
type MinecraftController struct {
	cfg config.InstanceConfig

	mu          sync.Mutex
	onlineSince time.Time // zero when offline; set the moment Status observes a transition to online
}

func NewMinecraftController(cfg config.InstanceConfig) *MinecraftController {
	return &MinecraftController{cfg: cfg}
}

// backendReachable reports whether c.cfg.BackendPort is currently accepting
// TCP connections. Mirrors maintenance.backendReachable (unexported there) —
// duplicated per this project's existing per-package convention (see
// maintenance.go's own doc comments on backendReachable/waitUntilBackendDown).
//
// The dial is bounded by whichever comes first: the caller's ctx or a 2s
// timeout local to this call.
func (c *MinecraftController) backendReachable(ctx context.Context) bool {
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", fmt.Sprintf("127.0.0.1:%d", c.cfg.BackendPort))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (c *MinecraftController) Status(ctx context.Context) (Status, error) {
	online := c.backendReachable(ctx)

	c.mu.Lock()
	if online && c.onlineSince.IsZero() {
		c.onlineSince = time.Now()
	} else if !online {
		c.onlineSince = time.Time{}
	}
	since := c.onlineSince
	c.mu.Unlock()

	if !online {
		return Status{Online: false}, nil
	}

	playerCount, maxPlayers, err := c.playerCounts(ctx)
	if err != nil {
		// Backend port is up but RCON didn't answer (still booting, or the
		// RCON listener died without the JVM dying — see project memory on
		// the Hunter Fury bug). Report online with unknown player counts
		// instead of failing the whole status check.
		return Status{Online: true, UptimeSec: int64(time.Since(since).Seconds())}, nil
	}

	return Status{
		Online:      true,
		PlayerCount: playerCount,
		MaxPlayers:  maxPlayers,
		UptimeSec:   int64(time.Since(since).Seconds()),
	}, nil
}

// playerCountsResult carries the outcome of the dial+command round-trip
// performed on the background goroutine started by playerCounts.
type playerCountsResult struct {
	current, max int
	err          error
}

// playerCounts dials RCON and issues the "list" command to obtain current
// and max player counts. rcon.Dial itself takes no context, so the whole
// dial+command round-trip runs on a goroutine that reports its result on a
// buffered (capacity 1) channel; playerCounts selects between that channel
// and ctx.Done(). If ctx wins, playerCounts returns ctx.Err() immediately —
// the goroutine keeps running in the background and writes its result to
// the buffered channel without blocking, so it does not leak. The in-flight
// dial itself is not cancelled; that is out of scope here.
func (c *MinecraftController) playerCounts(ctx context.Context) (current, max int, err error) {
	if c.cfg.Minecraft == nil {
		return 0, 0, fmt.Errorf("gamecontrol: instance %s has no minecraft_config", c.cfg.Name)
	}

	resultCh := make(chan playerCountsResult, 1)
	go func() {
		current, max, err := c.playerCountsBlocking()
		resultCh <- playerCountsResult{current: current, max: max, err: err}
	}()

	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	case res := <-resultCh:
		return res.current, res.max, res.err
	}
}

// playerCountsBlocking performs the actual (non-cancellable) RCON dial and
// "list" command round-trip.
func (c *MinecraftController) playerCountsBlocking() (current, max int, err error) {
	client, err := rcon.Dial(fmt.Sprintf("127.0.0.1:%d", c.cfg.Minecraft.RconPort), c.cfg.Minecraft.RconPassword, 5*time.Second)
	if err != nil {
		return 0, 0, err
	}
	defer client.Close()

	resp, err := client.Command("list")
	if err != nil {
		return 0, 0, err
	}
	m := listResponseRe.FindStringSubmatch(resp)
	if m == nil {
		return 0, 0, fmt.Errorf("gamecontrol: unexpected 'list' response: %q", resp)
	}
	current, err = strconv.Atoi(m[1])
	if err != nil {
		return 0, 0, err
	}
	max, err = strconv.Atoi(m[2])
	if err != nil {
		return 0, 0, err
	}
	return current, max, nil
}

func (c *MinecraftController) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return maintenance.Resume(c.cfg)
}

func (c *MinecraftController) Stop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return maintenance.Stop(c.cfg)
}

func (c *MinecraftController) Restart(ctx context.Context) error {
	if err := c.Stop(ctx); err != nil {
		return fmt.Errorf("gamecontrol: restart: stop failed: %w", err)
	}
	return c.Start(ctx)
}
