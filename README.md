# gameops

Single Go binary with subcommands for operating self-hosted game servers — auto stop/start on inactivity, backups, world regeneration, and startup automation. Built to replace a pile of ad-hoc PowerShell scripts with one ordered, testable tool that anyone can point at their own server.

## Subcommands

| Subcommand | Status |
|---|---|
| `idle-watch` | Implemented — proxies a game server, waking it on a real login attempt and stopping it after a configurable idle period. |
| `backup` | Roadmap |
| `maintenance` | Roadmap |
| `startup apply` | Implemented — waits for a backend to finish booting (optional log check), waits for RCON to respond, then applies a configured list of RCON commands with per-command retries. |
| `world regen` | Implemented — regenerates one named instance's world (backend must already be stopped): optionally blanks the seed, renames the old world out of the way, resets configured progress files, and seeds configured template files into the fresh world. |

## idle-watch

Sits in front of a backend game server (currently: Minecraft/Forge). While the backend is asleep it speaks just enough of the game's own protocol to answer server-list pings and login attempts with a "starting up" message, without needing the backend running. A real login attempt starts the backend; once it's reachable, idle-watch proxies raw bytes through. After a configurable number of minutes with no players online (checked via RCON), it stops the backend and goes back to sleep.

### Config

```toml
[[instances]]
name = "minecraft"
game = "minecraft"
listen_port = 25565
backend_port = 25566
idle_timeout_minutes = 15
poll_interval_seconds = 30
backend_ready_timeout_minutes = 5
start_command = "schtasks /run /tn mc-forge"

[instances.minecraft_config]
rcon_port = 25575
rcon_password = "secret"
forge_properties_path = "C:\\mc-forge\\server.properties"
motd_fallback = "Server asleep - connect to wake it up (~1-2 min)"
```

### Game defaults

Instances of the same game can share defaults instead of repeating every
field, via `[games.<name>]`:

```toml
[games.minecraft]
idle_timeout_minutes = 15
poll_interval_seconds = 30
start_command = "schtasks /run /tn mc-forge"

[games.minecraft.minecraft_config]
rcon_port = 25575
motd_fallback = "Server asleep - connect to wake it up (~1-2 min)"

[games.minecraft.backup_config]
max_backups = 10

[games.minecraft.maintenance_config]
process_name = "java"
stop_command = "stop"

[[instances]]
name = "servermc1"
game = "minecraft"
listen_port = 25565
backend_port = 25566

[instances.minecraft_config]
rcon_password = "secret"                                    # per-instance, not shared
forge_properties_path = "C:\\mc-forge\\server.properties"   # per-instance, not shared
```

Each field on an instance wins if set; otherwise it falls back to its
game's default, then to a built-in default. `rcon_password` *can* be set
under `[games.<name>]` and inherited, but sharing one password across
instances means a leak on one compromises all of them — prefer setting it
per-instance.

Run: `gameops.exe idle-watch -config gameops.toml`

## startup apply

Applies a configured list of RCON commands to a backend right after it
finishes booting — the kind of one-time setup (scoreboards, difficulty,
gamerules, permission toggles) a fresh or regenerated world needs. Optionally
waits for a boot-log pattern first (skip this by leaving `log_path` empty —
fully game-agnostic default), then waits for RCON to respond to a real probe
command, then sends each configured command with up to 3 retries. A command
that fails every retry is logged and skipped — it never aborts the run. Only
a boot-log timeout or an RCON-never-ready timeout make the command fail
overall.

### Config

```toml
[instances.startup_config]
log_path = "C:\\mc-forge\\logs\\latest.log"
boot_pattern = "Done ("
commands = [
  "scoreboard objectives add health health",
  "scoreboard objectives setdisplay list health",
  "gamerule playersSleepingPercentage 10",
  "difficulty hard",
]
```

Also inheritable via `[games.<name>].startup_config`, same as every other
per-instance config (see Game defaults above).

Run: `gameops.exe startup apply -config gameops.toml`

## world regen

Regenerates a single instance's world from scratch. Unlike every other
subcommand, `-instance <name>` is **required** — given how destructive and
irreversible this operation is, there's no "operate on everything configured"
default. The instance's backend must already be stopped (e.g. via
`gameops maintenance stop`) — `world regen` checks this itself and refuses to
run otherwise, but does not stop it for you.

With `-new-seed`, blanks the configured seed key in the instance's
`server_properties_path` so the game picks a new one at world creation.
Without it, the existing seed is left untouched. Either way, the old world
directory is renamed to `<world_path>_prev_<timestamp>` rather than deleted.
Configured `extra_reset_files` (e.g. a mod's separate save-data file) are
backed up the same way and reset to `{}`. Configured `seed_template_files`
are copied byte-for-byte into the fresh world directory before first boot —
useful for pre-seeding mod state that would otherwise reset itself
destructively on first load of a new world.

### Config

```toml
[instances.world_regen_config]
world_path = "C:\\mc-forge\\world"
server_properties_path = "C:\\mc-forge\\server.properties"
seed_key = "level-seed"
extra_reset_files = ["C:\\mc-forge\\limitedlives_data.json"]
seed_template_files = [
  { src = "C:\\mc-forge\\scripts\\betterzombieai_mapvars_template.dat", dest = "data/betterzombieai_mapvars.dat" },
]
```

Also inheritable via `[games.<name>].world_regen_config`, same as every
other per-instance config (see Game defaults above).

Run: `gameops.exe world regen -instance minecraft -config gameops.toml`
(add `-new-seed` for a brand-new random seed instead of reusing the current one)

## License

MIT
