package idlewatch

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
	"github.com/ShizukaJiku/gameops/internal/mcproto"
)

func writeHandshake(t *testing.T, conn net.Conn, nextState int32) {
	t.Helper()
	buf := new(bytes.Buffer)
	buf.Write(mcproto.WriteVarInt(763))
	buf.Write(mcproto.WriteString("localhost"))
	buf.Write([]byte{0x63, 0xDD}) // port, arbitrary
	buf.Write(mcproto.WriteVarInt(nextState))
	if err := mcproto.WritePacket(conn, 0x00, buf.Bytes()); err != nil {
		// Called from a goroutine in both tests below — Fatal/FailNow are
		// unsafe there (they call runtime.Goexit, which only unwinds the
		// calling goroutine and can leave the test hanging on a channel
		// that never receives). Error is safe from any goroutine.
		t.Errorf("writeHandshake: WritePacket error: %v", err)
	}
}

func TestHandleSleepingLoginTriggersWake(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{ForgePropertiesPath: filepath.Join(t.TempDir(), "does-not-exist.properties")})

	go writeHandshake(t, client, 2) // next_state = login

	result := make(chan bool, 1)
	go func() { result <- a.HandleSleeping(server, time.Time{}) }()

	r := bufio.NewReader(client)
	id, payload, err := mcproto.ReadPacket(r)
	if err != nil {
		t.Fatalf("ReadPacket error: %v", err)
	}
	if id != 0x00 {
		t.Fatalf("expected disconnect packet id 0, got %d", id)
	}
	text, _, err := mcproto.ReadString(payload)
	if err != nil {
		t.Fatalf("ReadString error: %v", err)
	}
	if text == "" {
		t.Fatal("expected non-empty disconnect JSON")
	}

	if !<-result {
		t.Fatal("expected HandleSleeping to return true (attemptingLogin) for login state")
	}
}

func TestHandleSleepingStatusDoesNotTriggerWake(t *testing.T) {
	server, client := net.Pipe()

	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{ForgePropertiesPath: filepath.Join(t.TempDir(), "does-not-exist.properties")})

	go func() {
		writeHandshake(t, client, 1)           // next_state = status
		mcproto.WritePacket(client, 0x00, nil) // Status Request, empty body
	}()

	result := make(chan bool, 1)
	go func() { result <- a.HandleSleeping(server, time.Time{}) }()

	r := bufio.NewReader(client)
	id, _, err := mcproto.ReadPacket(r)
	if err != nil {
		t.Fatalf("ReadPacket error: %v", err)
	}
	if id != 0x00 {
		t.Fatalf("expected status response packet id 0, got %d", id)
	}

	client.Close() // Close to signal EOF to HandleSleeping

	if <-result {
		t.Fatal("expected HandleSleeping to return false (no wake) for status state")
	}
}

func statusMotd(t *testing.T, payload []byte) string {
	t.Helper()
	text, _, err := mcproto.ReadString(payload)
	if err != nil {
		t.Fatalf("ReadString error: %v", err)
	}
	var parsed struct {
		Description struct {
			Text string `json:"text"`
		} `json:"description"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("status JSON unmarshal error: %v", err)
	}
	return parsed.Description.Text
}

func TestNewMinecraftAdapterFaviconFallsBackToEmbeddedPNGWhenFileMissing(t *testing.T) {
	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{ForgePropertiesPath: filepath.Join(t.TempDir(), "server.properties")})
	icon := a.readServerIcon()
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(icon, prefix) {
		t.Fatalf("expected favicon to start with %q", prefix)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(icon, prefix))
	if err != nil {
		t.Fatalf("favicon base64 decode error: %v", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("favicon is not a valid PNG: %v", err)
	}
	if cfg.Width != 64 || cfg.Height != 64 {
		t.Fatalf("expected 64x64 PNG, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestReadServerIconReadsLiveFileOverEmbedded(t *testing.T) {
	dir := t.TempDir()
	propsPath := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(propsPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	iconPath := filepath.Join(dir, "server-icon.png")
	if err := os.WriteFile(iconPath, []byte("not-a-real-png-but-distinct-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{ForgePropertiesPath: propsPath})
	icon := a.readServerIcon()
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not-a-real-png-but-distinct-bytes"))
	if icon != want {
		t.Fatalf("expected live icon file to be used, got %q", icon)
	}
}

func TestReadForgeMotdReadsRealValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(path, []byte("server-port=25566\nmotd=Mythic Ages Reborn\nonline-mode=true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{ForgePropertiesPath: path, MotdFallback: "fallback"})
	if got := a.readForgeMotd(); got != "Mythic Ages Reborn" {
		t.Fatalf("expected %q, got %q", "Mythic Ages Reborn", got)
	}
}

func TestReadForgeMotdFallsBackWhenFileMissing(t *testing.T) {
	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{
		ForgePropertiesPath: filepath.Join(t.TempDir(), "does-not-exist.properties"),
		MotdFallback:        "fallback-motd",
	})
	if got := a.readForgeMotd(); got != "fallback-motd" {
		t.Fatalf("expected fallback %q, got %q", "fallback-motd", got)
	}
}

func TestUnescapeJavaUnicodeSectionSign(t *testing.T) {
	// Java's Properties.store() escapes non-ASCII as \uXXXX. Forge/Minecraft's
	// § (section sign, U+00A7, used for chat color codes) commonly gets
	// persisted this way. Input here is the literal 7 ASCII characters
	// backslash, u, 0, 0, A, 7, c — exactly what Properties.store()
	// writes to the file, NOT the real § rune. We expect unescapeJavaUnicode to decode
	// this to the real rune.
	input := `§c`
	want := "§c" // real section-sign rune, Go source understands § natively
	if got := unescapeJavaUnicode(input); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestUnescapeJavaUnicodeLeavesPlainTextAlone(t *testing.T) {
	input := "Mythic Ages Reborn"
	if got := unescapeJavaUnicode(input); got != input {
		t.Fatalf("expected unchanged %q, got %q", input, got)
	}
}

func TestReadForgeMotdUnescapesUnicodeSectionSign(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.properties")
	// § here is the literal escape sequence Java's Properties.store()
	// writes to disk for the real § rune. We expect readForgeMotd to call
	// unescapeJavaUnicode and decode it to the real rune.
	content := "server-port=25566\n" + `motd=Test§cColor` + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{ForgePropertiesPath: path, MotdFallback: "fallback"})
	want := "Test§cColor"
	if got := a.readForgeMotd(); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestReadForgeMotdFallsBackWhenValueEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(path, []byte("server-port=25566\nmotd=\nonline-mode=true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{ForgePropertiesPath: path, MotdFallback: "fallback-motd"})
	if got := a.readForgeMotd(); got != "fallback-motd" {
		t.Fatalf("expected fallback %q, got %q", "fallback-motd", got)
	}
}

func TestReadForgeMotdFallsBackWhenLineMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(path, []byte("server-port=25566\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{ForgePropertiesPath: path, MotdFallback: "fallback-motd"})
	if got := a.readForgeMotd(); got != "fallback-motd" {
		t.Fatalf("expected fallback %q, got %q", "fallback-motd", got)
	}
}

func TestStateLabelAsleep(t *testing.T) {
	if got := stateLabel(time.Time{}); got != "§7| ⚡ §eDormido" {
		t.Fatalf("expected asleep label, got %q", got)
	}
}

func TestStateLabelWakingCyclesThroughThreeStages(t *testing.T) {
	now := time.Now()
	cases := []struct {
		elapsed time.Duration
		want    string
	}{
		{0, "§7| §6Arrancando..."},
		{9 * time.Second, "§7| §6Arrancando..."},
		{10 * time.Second, "§7| §6Cargando..."},
		{19 * time.Second, "§7| §6Cargando..."},
		{20 * time.Second, "§7| §6Preparando..."},
		{29 * time.Second, "§7| §6Preparando..."},
		{30 * time.Second, "§7| §6Arrancando..."}, // cycles back
	}
	for _, c := range cases {
		wakingSince := now.Add(-c.elapsed)
		if got := stateLabel(wakingSince); got != c.want {
			t.Errorf("elapsed=%v: expected %q, got %q", c.elapsed, c.want, got)
		}
	}
}

// TestHandleSleepingStatusDescriptionIsJustRealMotdWhenAsleep and its waking
// counterpart below prove the status ping's description field carries ONLY
// the real Forge motd, with no state-label suffix — that signal lives
// exclusively in version.name (see TestHandleSleepingStatusIncludesFaviconAndVersionLabel),
// so duplicating it into the description would be redundant clutter in the
// server list.
func TestHandleSleepingStatusDescriptionIsJustRealMotdWhenAsleep(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(path, []byte("motd=Mythic Ages Reborn\n"), 0644); err != nil {
		t.Fatal(err)
	}
	server, client := net.Pipe()

	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{ForgePropertiesPath: path})

	go func() {
		writeHandshake(t, client, 1)
		mcproto.WritePacket(client, 0x00, nil)
	}()

	go a.HandleSleeping(server, time.Time{})

	r := bufio.NewReader(client)
	_, payload, err := mcproto.ReadPacket(r)
	if err != nil {
		t.Fatalf("ReadPacket error: %v", err)
	}
	client.Close()

	want := "Mythic Ages Reborn"
	if got := statusMotd(t, payload); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestHandleSleepingStatusDescriptionIsJustRealMotdWhenWaking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(path, []byte("motd=Mythic Ages Reborn\n"), 0644); err != nil {
		t.Fatal(err)
	}
	server, client := net.Pipe()

	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{ForgePropertiesPath: path})

	go func() {
		writeHandshake(t, client, 1)
		mcproto.WritePacket(client, 0x00, nil)
	}()

	go a.HandleSleeping(server, time.Now())

	r := bufio.NewReader(client)
	_, payload, err := mcproto.ReadPacket(r)
	if err != nil {
		t.Fatalf("ReadPacket error: %v", err)
	}
	client.Close()

	want := "Mythic Ages Reborn"
	if got := statusMotd(t, payload); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// TestHandleSleepingDisconnectStillIncludesLabel proves the kick message
// (login attempt while asleep/waking) still carries the state-label suffix —
// unlike the status description, the Disconnect packet has no version field
// to show the label in instead, so this is the only channel available there.
func TestHandleSleepingDisconnectStillIncludesLabel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(path, []byte("motd=Mythic Ages Reborn\n"), 0644); err != nil {
		t.Fatal(err)
	}
	server, client := net.Pipe()
	defer client.Close()

	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{ForgePropertiesPath: path})

	go writeHandshake(t, client, 2)
	go a.HandleSleeping(server, time.Time{})

	r := bufio.NewReader(client)
	_, payload, err := mcproto.ReadPacket(r)
	if err != nil {
		t.Fatalf("ReadPacket error: %v", err)
	}
	text, _, err := mcproto.ReadString(payload)
	if err != nil {
		t.Fatalf("ReadString error: %v", err)
	}
	want := "Mythic Ages Reborn §7| ⚡ §eDormido"
	if !strings.Contains(text, want) {
		t.Fatalf("expected disconnect reason to contain %q, got %q", want, text)
	}
}

func TestHandleSleepingStatusIncludesFaviconAndVersionLabel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(path, []byte("motd=Mythic Ages Reborn\n"), 0644); err != nil {
		t.Fatal(err)
	}
	server, client := net.Pipe()

	a := NewMinecraftAdapter(&config.MinecraftAdapterConfig{ForgePropertiesPath: path})

	go func() {
		writeHandshake(t, client, 1)
		mcproto.WritePacket(client, 0x00, nil)
	}()

	go a.HandleSleeping(server, time.Time{})

	r := bufio.NewReader(client)
	_, payload, err := mcproto.ReadPacket(r)
	if err != nil {
		t.Fatalf("ReadPacket error: %v", err)
	}
	client.Close()

	text, _, err := mcproto.ReadString(payload)
	if err != nil {
		t.Fatalf("ReadString error: %v", err)
	}
	var parsed struct {
		Favicon string `json:"favicon"`
		Version struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		} `json:"version"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("status JSON unmarshal error: %v", err)
	}
	if !strings.HasPrefix(parsed.Favicon, "data:image/png;base64,") {
		t.Fatalf("expected favicon data URI, got %q", parsed.Favicon)
	}
	if parsed.Version.Name != "§7| ⚡ §eDormido" {
		t.Fatalf("expected version.name to carry the asleep label, got %q", parsed.Version.Name)
	}
	if parsed.Version.Protocol >= 0 {
		t.Fatalf("expected a deliberately invalid protocol number to force the version-mismatch display, got %d", parsed.Version.Protocol)
	}
}
