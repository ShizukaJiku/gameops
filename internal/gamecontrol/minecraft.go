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
func (c *MinecraftController) backendReachable() bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", c.cfg.BackendPort), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (c *MinecraftController) Status(ctx context.Context) (Status, error) {
	online := c.backendReachable()

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

	playerCount, maxPlayers, err := c.playerCounts()
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

func (c *MinecraftController) playerCounts() (current, max int, err error) {
	if c.cfg.Minecraft == nil {
		return 0, 0, fmt.Errorf("gamecontrol: instance %s has no minecraft_config", c.cfg.Name)
	}
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
	return maintenance.Resume(c.cfg)
}

func (c *MinecraftController) Stop(ctx context.Context) error {
	return maintenance.Stop(c.cfg)
}

func (c *MinecraftController) Restart(ctx context.Context) error {
	if err := c.Stop(ctx); err != nil {
		return fmt.Errorf("gamecontrol: restart: stop failed: %w", err)
	}
	return c.Start(ctx)
}
