package idlewatch

import (
	"bufio"
	_ "embed"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
	"github.com/ShizukaJiku/gameops/internal/mcproto"
	"github.com/ShizukaJiku/gameops/internal/rcon"
)

// javaUnicodeEscapeRe matches Java .properties \uXXXX unicode escapes
// (exactly 4 hex digits), as written by Properties.store().
var javaUnicodeEscapeRe = regexp.MustCompile(`\\u([0-9A-Fa-f]{4})`)

// unescapeJavaUnicode decodes Java .properties \uXXXX unicode escapes (the
// form Properties.store() uses for any character outside printable ASCII —
// notably Minecraft's § section-sign color code) into their real rune.
// Scope is intentionally narrow: no other Properties escapes (\:, \=, \\,
// etc.) are handled, since \uXXXX is the only concrete failure mode this
// project has encountered in Forge's server.properties.
func unescapeJavaUnicode(s string) string {
	return javaUnicodeEscapeRe.ReplaceAllStringFunc(s, func(m string) string {
		hex := m[2:] // strip leading \u
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return m
		}
		return string(rune(v))
	})
}

//go:embed assets/server-icon-64.png
var serverIconPNG []byte

const defaultForgePropertiesPath = `C:\mc-forge\server.properties`

var embeddedFavicon = "data:image/png;base64," + base64.StdEncoding.EncodeToString(serverIconPNG)

type MinecraftAdapter struct {
	cfg                 *config.MinecraftAdapterConfig
	forgePropertiesPath string
	iconPath            string
}

func NewMinecraftAdapter(cfg *config.MinecraftAdapterConfig) *MinecraftAdapter {
	path := cfg.ForgePropertiesPath
	if path == "" {
		path = defaultForgePropertiesPath
	}
	return &MinecraftAdapter{
		cfg:                 cfg,
		forgePropertiesPath: path,
		iconPath:            filepath.Join(filepath.Dir(path), "server-icon.png"),
	}
}

// readServerIcon reads Forge's own server-icon.png live off disk — the same
// file Forge itself serves once awake — so an operator replacing that one
// file updates the icon shown in every state (asleep/waking/awake) without
// rebuilding or restarting gameops. Falls back to the embedded PNG if the
// file is missing or unreadable.
func (a *MinecraftAdapter) readServerIcon() string {
	raw, err := os.ReadFile(a.iconPath)
	if err != nil || len(raw) == 0 {
		return embeddedFavicon
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
}

// readForgeMotd reads the live motd= line from Forge's server.properties.
// Falls back to cfg.MotdFallback if the file is missing, unreadable, or has
// no motd= line — read on every call (no caching) so an operator editing
// Forge's motd is reflected without restarting gameops.
func (a *MinecraftAdapter) readForgeMotd() string {
	f, err := os.Open(a.forgePropertiesPath)
	if err != nil {
		return a.cfg.MotdFallback
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "motd=") {
			value := unescapeJavaUnicode(strings.TrimPrefix(line, "motd="))
			if value == "" {
				return a.cfg.MotdFallback
			}
			return value
		}
	}
	return a.cfg.MotdFallback
}

// stateLabel returns the state suffix shown next to the real motd, and
// reused verbatim as the version.name label. Asleep is a fixed label;
// waking cycles through 3 labels every 10s — time-based, not tied to the
// backend's actual boot log (with ~150+ mods, real boot time is dominated
// by mod loading before any "preparing level" log line appears, so tailing
// the log wouldn't show more real progress than a fixed-interval cycle).
func stateLabel(wakingSince time.Time) string {
	if wakingSince.IsZero() {
		return "§7| ⚡ §eDormido"
	}
	switch int(time.Since(wakingSince)/(10*time.Second)) % 3 {
	case 0:
		return "§7| §6Arrancando..."
	case 1:
		return "§7| §6Cargando..."
	default:
		return "§7| §6Preparando..."
	}
}

// HandleSleeping speaks just enough of the Minecraft protocol to answer a
// status ping (server list) or a login attempt while the backend isn't
// proxying yet. The state label from stateLabel (a fixed "asleep" label
// while wakingSince is zero, or a cycling "waking" label once start_command
// has run but the backend isn't accepting connections yet) shows up
// differently depending on which packet is being answered: the status
// response carries it in version.name (forcing a version-mismatch display
// so it's visible without a hover — see mcproto.BuildStatusJSON), leaving
// the description field as the real motd unmodified; the Disconnect packet
// has no version field, so there the label is appended directly to the kick
// message instead. Returns true only for a real login attempt
// (next_state == 2); the caller uses that as the wake trigger, and ignores
// it entirely while waking is already true (a second login attempt
// shouldn't re-run start_command).
func (a *MinecraftAdapter) HandleSleeping(conn net.Conn, wakingSince time.Time) bool {
	defer conn.Close()
	r := bufio.NewReader(conn)

	id, payload, err := mcproto.ReadPacket(r)
	if err != nil || id != 0x00 {
		return false
	}
	nextState, err := mcproto.ParseHandshake(payload)
	if err != nil {
		return false
	}

	label := stateLabel(wakingSince)
	realMotd := a.readForgeMotd()

	if nextState == 2 {
		// The Disconnect packet has no version field to carry the label in,
		// so the kick message is the only place left to compose it into —
		// unlike the status description below, which leaves this to
		// version.name instead of duplicating it.
		reason, err := mcproto.BuildDisconnectJSON(realMotd + " " + label)
		if err != nil {
			return true
		}
		mcproto.WritePacket(conn, 0x00, mcproto.WriteString(reason))
		return true
	}

	// Status ping: consume Status Request, reply, optionally answer Ping.
	if _, _, err := mcproto.ReadPacket(r); err != nil {
		return false
	}
	statusJSON, err := mcproto.BuildStatusJSON(realMotd, a.readServerIcon(), label)
	if err != nil {
		return false
	}
	if err := mcproto.WritePacket(conn, 0x00, mcproto.WriteString(statusJSON)); err != nil {
		return false
	}

	pingID, pingPayload, err := mcproto.ReadPacket(r)
	if err == nil && pingID == 0x01 {
		mcproto.WritePacket(conn, 0x01, pingPayload)
	}
	return false
}

var listResponseRe = regexp.MustCompile(`There are (\d+) of a max`)

func (a *MinecraftAdapter) PlayerCount() (int, error) {
	c, err := rcon.Dial(fmt.Sprintf("127.0.0.1:%d", a.cfg.RconPort), a.cfg.RconPassword, 5*time.Second)
	if err != nil {
		return 0, err
	}
	defer c.Close()

	resp, err := c.Command("list")
	if err != nil {
		return 0, err
	}
	m := listResponseRe.FindStringSubmatch(resp)
	if m == nil {
		return 0, fmt.Errorf("rcon: unexpected 'list' response: %q", resp)
	}
	return strconv.Atoi(m[1])
}

func (a *MinecraftAdapter) Stop() error {
	c, err := rcon.Dial(fmt.Sprintf("127.0.0.1:%d", a.cfg.RconPort), a.cfg.RconPassword, 5*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()

	_, err = c.Command("stop")
	return err
}
