package backup

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ShizukaJiku/gameops/internal/config"
)

func TestResolveBackupConfigAppliesAllDefaultsWhenNil(t *testing.T) {
	worldPath, backupsDir, maxBackups := resolveBackupConfig(nil)
	if worldPath != defaultWorldPath || backupsDir != defaultBackupsDir || maxBackups != defaultMaxBackups {
		t.Fatalf("expected all defaults, got worldPath=%q backupsDir=%q maxBackups=%d", worldPath, backupsDir, maxBackups)
	}
}

func TestResolveBackupConfigAppliesPartialDefaults(t *testing.T) {
	cfg := &config.BackupConfig{WorldPath: `C:\custom\world`}
	worldPath, backupsDir, maxBackups := resolveBackupConfig(cfg)
	if worldPath != `C:\custom\world` {
		t.Fatalf("expected custom worldPath to be kept, got %q", worldPath)
	}
	if backupsDir != defaultBackupsDir {
		t.Fatalf("expected default backupsDir, got %q", backupsDir)
	}
	if maxBackups != defaultMaxBackups {
		t.Fatalf("expected default maxBackups, got %d", maxBackups)
	}
}

func TestResolveBackupConfigKeepsAllFieldsWhenSet(t *testing.T) {
	cfg := &config.BackupConfig{WorldPath: "w", BackupsDir: "b", MaxBackups: 3}
	worldPath, backupsDir, maxBackups := resolveBackupConfig(cfg)
	if worldPath != "w" || backupsDir != "b" || maxBackups != 3 {
		t.Fatalf("expected all custom values kept, got worldPath=%q backupsDir=%q maxBackups=%d", worldPath, backupsDir, maxBackups)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func zipEntryNames(t *testing.T, zipPath string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer r.Close()

	out := make(map[string]string)
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", f.Name, err)
		}
		out[f.Name] = string(content)
	}
	return out
}

func TestZipWorldWritesFilesWithWorldPrefix(t *testing.T) {
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	writeFile(t, filepath.Join(worldDir, "level.dat"), "level-data")
	writeFile(t, filepath.Join(worldDir, "region", "r.0.0.mca"), "region-data")

	dest := filepath.Join(dir, "out.zip")
	if err := zipWorld(worldDir, dest); err != nil {
		t.Fatalf("zipWorld error: %v", err)
	}

	entries := zipEntryNames(t, dest)
	if entries["world/level.dat"] != "level-data" {
		t.Fatalf("expected world/level.dat entry, got entries: %v", entries)
	}
	if entries["world/region/r.0.0.mca"] != "region-data" {
		t.Fatalf("expected world/region/r.0.0.mca entry, got entries: %v", entries)
	}
}

func TestZipWorldSkipsSessionLock(t *testing.T) {
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	writeFile(t, filepath.Join(worldDir, "level.dat"), "level-data")
	writeFile(t, filepath.Join(worldDir, "session.lock"), "lock-bytes")

	dest := filepath.Join(dir, "out.zip")
	if err := zipWorld(worldDir, dest); err != nil {
		t.Fatalf("zipWorld error: %v", err)
	}

	entries := zipEntryNames(t, dest)
	if _, ok := entries["world/session.lock"]; ok {
		t.Fatalf("expected session.lock to be excluded, got entries: %v", entries)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 entry (level.dat), got: %v", entries)
	}
}

func TestZipWorldReturnsErrorForNonexistentRoot(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.zip")
	err := zipWorld(filepath.Join(dir, "does-not-exist"), dest)
	if err == nil {
		t.Fatal("expected error for nonexistent world root, got nil")
	}
}

func TestWriteBackupZipProducesFinalNamedFileAndNoTmpLeftover(t *testing.T) {
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	writeFile(t, filepath.Join(worldDir, "level.dat"), "level-data")
	backupsDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		t.Fatal(err)
	}

	finalPath, err := writeBackupZip(worldDir, backupsDir, "20260101_010101")
	if err != nil {
		t.Fatalf("writeBackupZip error: %v", err)
	}
	wantPath := filepath.Join(backupsDir, "world_20260101_010101.zip")
	if finalPath != wantPath {
		t.Fatalf("expected final path %q, got %q", wantPath, finalPath)
	}
	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf("expected final zip to exist: %v", err)
	}
	if _, err := os.Stat(finalPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected no .tmp file left behind, stat error: %v", err)
	}
}

func TestWriteBackupZipCleansUpTmpFileOnFailure(t *testing.T) {
	dir := t.TempDir()
	backupsDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		t.Fatal(err)
	}
	nonexistentWorld := filepath.Join(dir, "does-not-exist")

	_, err := writeBackupZip(nonexistentWorld, backupsDir, "20260101_010101")
	if err == nil {
		t.Fatal("expected error for nonexistent world root, got nil")
	}

	finalPath := filepath.Join(backupsDir, "world_20260101_010101.zip")
	if _, statErr := os.Stat(finalPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no final .zip to exist after failure, stat error: %v", statErr)
	}
	if _, statErr := os.Stat(finalPath + ".tmp"); !os.IsNotExist(statErr) {
		t.Fatalf("expected no .tmp file left behind after failure, stat error: %v", statErr)
	}
}

func setBackupFileMtime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestRotateKeepsOnlyMaxBackupsMostRecent(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	names := []string{
		"world_20260101_010101.zip", // oldest
		"world_20260102_010101.zip",
		"world_20260103_010101.zip",
		"world_20260104_010101.zip", // newest
	}
	for i, name := range names {
		path := filepath.Join(dir, name)
		writeFile(t, path, "zip-bytes")
		setBackupFileMtime(t, path, base.AddDate(0, 0, i))
	}

	if err := rotate(dir, 2); err != nil {
		t.Fatalf("rotate error: %v", err)
	}

	remaining, err := filepath.Glob(filepath.Join(dir, "world_*.zip"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(remaining)
	want := []string{
		filepath.Join(dir, "world_20260103_010101.zip"),
		filepath.Join(dir, "world_20260104_010101.zip"),
	}
	sort.Strings(want)
	if len(remaining) != len(want) {
		t.Fatalf("expected %v to remain, got %v", want, remaining)
	}
	for i := range want {
		if remaining[i] != want[i] {
			t.Fatalf("expected %v to remain, got %v", want, remaining)
		}
	}
}

func TestRotateNoOpWhenUnderLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "world_20260101_010101.zip")
	writeFile(t, path, "zip-bytes")

	if err := rotate(dir, 12); err != nil {
		t.Fatalf("rotate error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the single backup to remain untouched: %v", err)
	}
}

// --- minimal fake RCON server, local to this test file (internal/rcon's
// own fakeRconServer is unexported to that package and not reusable here) ---

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

// fakeRconServer accepts any password (single-packet Minecraft-style auth
// response, no leading empty packet). For each command received, it looks
// up responses; if the command isn't in the map, it force-closes the
// connection without responding, simulating an RCON failure for that one
// command — used to test the save-off-succeeds/save-all-flush-fails
// scenario. Every command received across every connection this server
// accepts (backup.Run dials fresh per RCON operation, same pattern as
// idlewatch.MinecraftAdapter) is appended to a shared, mutex-guarded slice
// so a test can assert exact call order across separate Dial calls.
func fakeRconServer(t *testing.T, responses map[string]string) (addr string, receivedCommands func() []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	var mu sync.Mutex
	var commands []string

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
					mu.Lock()
					commands = append(commands, body)
					mu.Unlock()

					resp, ok := responses[body]
					if !ok {
						return // simulate an RCON failure for this command
					}
					writeTestPacket(conn, id, typeResponse, resp)
				}
			}(conn)
		}
	}()

	receivedCommands = func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(commands))
		copy(out, commands)
		return out
	}
	return ln.Addr().String(), receivedCommands
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

func TestRunCallsSaveOnEvenWhenSaveAllFlushFails(t *testing.T) {
	addr, receivedCommands := fakeRconServer(t, map[string]string{
		"save-off": "Turned off world auto-saving",
		// "save-all flush" deliberately absent — the fake server closes the
		// connection without responding, simulating an RCON failure mid-sequence.
		"save-on": "Turned on world auto-saving",
	})

	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	writeFile(t, filepath.Join(worldDir, "level.dat"), "level-data")

	cfg := config.InstanceConfig{
		Name: "test",
		Backup: &config.BackupConfig{
			WorldPath:  worldDir,
			BackupsDir: filepath.Join(dir, "backups"),
			MaxBackups: 12,
		},
		Minecraft: &config.MinecraftAdapterConfig{
			RconPort:     rconPortFromAddr(t, addr),
			RconPassword: "secret",
		},
	}

	if _, err := Run(cfg); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	cmds := receivedCommands()
	if len(cmds) < 1 || cmds[0] != "save-off" {
		t.Fatalf("expected save-off to be sent first, got %v", cmds)
	}
	saveOnCalled := false
	for _, c := range cmds {
		if c == "save-on" {
			saveOnCalled = true
		}
	}
	if !saveOnCalled {
		t.Fatalf("expected save-on to be called even though save-all flush failed, got commands: %v", cmds)
	}
}

func TestRunSucceedsWithoutSaveOffWhenRconUnreachable(t *testing.T) {
	// Bind and immediately close a listener to get a port nothing is
	// listening on, guaranteeing rcon.Dial fails with a connection error.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	unreachablePort := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	writeFile(t, filepath.Join(worldDir, "level.dat"), "level-data")

	cfg := config.InstanceConfig{
		Name: "test",
		Backup: &config.BackupConfig{
			WorldPath:  worldDir,
			BackupsDir: filepath.Join(dir, "backups"),
			MaxBackups: 12,
		},
		Minecraft: &config.MinecraftAdapterConfig{
			RconPort:     unreachablePort,
			RconPassword: "secret",
		},
	}

	path, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected backup to be created despite RCON being unreachable: %v", statErr)
	}
}

func TestRunReturnsEmptyPathWhenWorldMissing(t *testing.T) {
	dir := t.TempDir()
	cfg := config.InstanceConfig{
		Name: "test",
		Backup: &config.BackupConfig{
			WorldPath:  filepath.Join(dir, "does-not-exist"),
			BackupsDir: filepath.Join(dir, "backups"),
		},
	}

	path, err := Run(cfg)
	if err != nil {
		t.Fatalf("expected no error when world is missing, got: %v", err)
	}
	if path != "" {
		t.Fatalf("expected empty path when world is missing, got %q", path)
	}
}

func TestRunSkipsRconWhenMinecraftConfigNil(t *testing.T) {
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	writeFile(t, filepath.Join(worldDir, "level.dat"), "level-data")

	cfg := config.InstanceConfig{
		Name: "test",
		Backup: &config.BackupConfig{
			WorldPath:  worldDir,
			BackupsDir: filepath.Join(dir, "backups"),
			MaxBackups: 12,
		},
		Minecraft: nil,
	}

	path, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected backup to be created without any RCON config: %v", statErr)
	}
}

func TestRunRotatesOldBackupsAfterMultipleSuccessfulRuns(t *testing.T) {
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	writeFile(t, filepath.Join(worldDir, "level.dat"), "level-data")
	backupsDir := filepath.Join(dir, "backups")

	cfg := config.InstanceConfig{
		Name: "test",
		Backup: &config.BackupConfig{
			WorldPath:  worldDir,
			BackupsDir: backupsDir,
			MaxBackups: 2,
		},
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if _, err := runAt(cfg, base.AddDate(0, 0, i)); err != nil {
			t.Fatalf("runAt error on iteration %d: %v", i, err)
		}
	}

	remaining, err := filepath.Glob(filepath.Join(backupsDir, "world_*.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected exactly 2 backups to remain after rotation, got %d: %v", len(remaining), remaining)
	}
}
